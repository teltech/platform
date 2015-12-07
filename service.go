package platform

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	logger                 = GetLogger("platform")
	serviceToken           = os.Getenv("SERVICE_TOKEN")
	stillConsuming         bool
	consumedWorkCount      int
	consumedWorkCountMutex *sync.Mutex
)

type Courier struct {
	responses chan *Request
}

func (c *Courier) Send(response *Request) {
	if response.GetCompleted() {
		logger.Printf("[Courier] %s sending FINAL %s", response.GetUuid(), response.Routing.RouteTo[0].GetUri())
	} else {
		logger.Printf("[Courier] %s sending INTERMEDIARY %s", response.GetUuid(), response.Routing.RouteTo[0].GetUri())
	}

	c.responses <- response
}

func NewCourier(publisher Publisher) *Courier {
	responses := make(chan *Request, 10)

	go func() {
		for response := range responses {
			logger.Printf("[Service.Subscriber] publishing response: %s", response)

			destinationRouteIndex := len(response.Routing.RouteTo) - 1
			destinationRoute := response.Routing.RouteTo[destinationRouteIndex]
			response.Routing.RouteTo = response.Routing.RouteTo[:destinationRouteIndex]

			body, err := Marshal(response)
			if err != nil {
				logger.Printf("[Service.Subscriber] failed to marshal response: %s", err)
				continue
			}

			// URI may not be valid here, we may need to parse it first for good practice. - Bryan
			publisher.Publish(destinationRoute.GetUri(), body)

			logger.Println("[Service.Subscriber] published response successfully")
		}
	}()

	return &Courier{
		responses: responses,
	}
}

type RequestHeartbeatCourier struct {
	parent ResponseSender
	quit   chan bool
}

func (rhc *RequestHeartbeatCourier) Send(response *Request) {
	logger.Printf("[RequestHeartbeatCourier.Send] %s attempting to send response", response.GetUuid())
	if response.GetCompleted() {
		rhc.quit <- true
	}

	rhc.parent.Send(response)

	logger.Printf("[RequestHeartbeatCourier.Send] %s sent response", response.GetUuid())
}

func NewRequestHeartbeatCourier(parent ResponseSender, request *Request) *RequestHeartbeatCourier {
	quit := make(chan bool, 1)

	logger.Println("[NewRequestHeartbeatCourier] creating a new heartbeat courier")

	go func() {
		for {
			select {
			case <-time.After(500 * time.Millisecond):
				parent.Send(GenerateResponse(request, &Request{
					Routing: RouteToUri("resource:///heartbeat"),
				}))

			case <-quit:
				return

			}
		}
	}()

	return &RequestHeartbeatCourier{
		parent: parent,
		quit:   quit,
	}
}

func identifyPanic() string {
	var name, file string
	var line int
	var pc [16]uintptr

	n := runtime.Callers(3, pc[:])
	for _, pc := range pc[:n] {
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}
		file, line = fn.FileLine(pc)
		name = fn.Name()
		if !strings.HasPrefix(name, "runtime.") {
			break
		}
	}

	switch {
	case name != "":
		return fmt.Sprintf("%v:%v", name, line)
	case file != "":
		return fmt.Sprintf("%v:%v", file, line)
	}

	return fmt.Sprintf("pc:%x", pc)
}

type Service struct {
	publisher  Publisher
	subscriber Subscriber
	courier    *Courier
	name       string
}

func (s *Service) AddHandler(path string, handler Handler) {
	logger.Println("[Service.AddHandler] adding handler", path)

	s.subscriber.Subscribe("microservice-"+path, ConsumerHandlerFunc(func(p []byte) error {
		logger.Printf("[Service.Subscriber] handling %s request", path)

		request := &Request{}
		if err := Unmarshal(p, request); err != nil {
			logger.Println("[Service.Subscriber] failed to decode request")

			return nil
		}

		requestHeartbeatCourier := NewRequestHeartbeatCourier(s.courier, request)

		if Getenv("PLATFORM_PREVENT_PANICS", "1") == "1" {
			defer func() {
				if r := recover(); r != nil {
					panicErrorBytes, _ := Marshal(&Error{
						Message: String(fmt.Sprintf("A fatal error has occurred. %s: %s %s", path, identifyPanic(), r)),
					})

					requestHeartbeatCourier.Send(GenerateResponse(request, &Request{
						Routing:   RouteToUri("resource:///platform/reply/error"),
						Payload:   panicErrorBytes,
						Completed: Bool(true),
					}))

					s.publisher.Publish("panic."+path, p)
				}
			}()
		}

		handler.Handle(requestHeartbeatCourier, request)

		return nil
	}))
}

func (s *Service) AddListener(topic string, handler ConsumerHandler) {
	s.subscriber.Subscribe(topic, handler)
}

func (s *Service) Run() {
	stillConsuming = true
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	consumedWorkCountMutex = &sync.Mutex{}

	// Emit a signal if we catch an interrupt
	go func() {
		select {
		case <-sigc:
			logger.Println("Recieved exit signal, waiting for work queue to empty..")
			stillConsuming = false

			for {
				if getConsumerWorkCount() < 1 {
					time.Sleep(time.Millisecond * 500)
					break
				}
			}
			logger.Println("Exiting.")
			os.Exit(0)
		}
	}()

	s.subscriber.Run()

	logger.Println("Subscriptions have stopped")
}

func NewService(serviceName string, publisher Publisher, subscriber Subscriber) (*Service, error) {
	return &Service{
		subscriber: subscriber,
		publisher:  publisher,
		courier:    NewCourier(publisher),
		name:       serviceName,
	}, nil
}

func NewBasicService(serviceName string) (*Service, error) {
	rabbitUser := Getenv("RABBITMQ_USER", "admin")
	rabbitPass := Getenv("RABBITMQ_PASS", "admin")
	rabbitAddr := Getenv("RABBITMQ_PORT_5672_TCP_ADDR", "127.0.0.1")
	rabbitPort := Getenv("RABBITMQ_PORT_5672_TCP_PORT", "5672")

	connectionManager := NewAmqpConnectionManager(rabbitUser, rabbitPass, rabbitAddr+":"+rabbitPort, "")

	publisher, err := NewAmqpPublisher(connectionManager)
	if err != nil {
		return nil, err
	}

	subscriber, err := NewAmqpSubscriber(connectionManager, serviceName)
	if err != nil {
		return nil, err
	}

	return NewService(serviceName, publisher, subscriber)
}
