package config

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestArrayConfigManager(t *testing.T) {
	Convey("Geting a service without any environment variables set should return an error", t, func() {
		configManager, err := NewArrayConfigManager([]string{})
		So(err, ShouldBeNil)
		So(configManager, ShouldNotBeNil)

		serviceConfigs, err := configManager.GetServiceConfigs("RABBITMQ", "5672")
		So(err, ShouldEqual, NoServiceConfigs)
		So(len(serviceConfigs), ShouldEqual, 0)
	})

	Convey("Geting a service config that has values set should return a service config", t, func() {
		configManager, err := NewArrayConfigManager([]string{
			"RABBITMQ_1_PORT_5672_TCP_ADDR=127.0.0.1",
			"RABBITMQ_1_PORT_5672_TCP_PORT=5672",
		})
		So(err, ShouldBeNil)
		So(configManager, ShouldNotBeNil)

		serviceConfigs, err := configManager.GetServiceConfigs("RABBITMQ", "5672")
		So(err, ShouldBeNil)
		So(len(serviceConfigs), ShouldEqual, 1)
		So(serviceConfigs, ShouldResemble, []*ServiceConfig{
			&ServiceConfig{
				User:  "",
				Pass:  "",
				Index: "1",
				Addr:  "127.0.0.1",
				Port:  "5672",
			},
		})
	})

	Convey("A service at an unspecified index should assume index 1, and not return duplicates", t, func() {
		configManager, err := NewArrayConfigManager([]string{
			"RABBITMQ_PORT_5672_TCP_ADDR=127.0.0.1",
			"RABBITMQ_PORT_5672_TCP_PORT=5672",
			"RABBITMQ_1_PORT_5672_TCP_ADDR=127.0.0.1",
			"RABBITMQ_1_PORT_5672_TCP_PORT=5672",
		})
		So(err, ShouldBeNil)
		So(configManager, ShouldNotBeNil)

		serviceConfigs, err := configManager.GetServiceConfigs("RABBITMQ", "5672")
		So(err, ShouldBeNil)
		So(serviceConfigs, ShouldResemble, []*ServiceConfig{
			&ServiceConfig{
				User:  "",
				Pass:  "",
				Index: "1",
				Addr:  "127.0.0.1",
				Port:  "5672",
			},
		})
	})

	Convey("When a user and pass is set at the service index level, it should be included on the service", t, func() {
		configManager, err := NewArrayConfigManager([]string{
			"RABBITMQ_1_PORT_5672_USER=user",
			"RABBITMQ_1_PORT_5672_PASS=pass",
			"RABBITMQ_1_PORT_5672_TCP_ADDR=127.0.0.1",
			"RABBITMQ_1_PORT_5672_TCP_PORT=5672",
		})
		So(err, ShouldBeNil)
		So(configManager, ShouldNotBeNil)

		serviceConfigs, err := configManager.GetServiceConfigs("RABBITMQ", "5672")
		So(err, ShouldBeNil)
		So(serviceConfigs, ShouldResemble, []*ServiceConfig{
			&ServiceConfig{
				User:  "user",
				Pass:  "pass",
				Index: "1",
				Addr:  "127.0.0.1",
				Port:  "5672",
			},
		})
	})

	Convey("When a user and pass is set at the root level, it should be included on every service", t, func() {
		configManager, err := NewArrayConfigManager([]string{
			"RABBITMQ_USER=user",
			"RABBITMQ_PASS=pass",
			"RABBITMQ_1_PORT_5672_TCP_ADDR=127.0.0.1",
			"RABBITMQ_1_PORT_5672_TCP_PORT=5672",
			"RABBITMQ_2_PORT_5672_TCP_ADDR=127.0.0.1",
			"RABBITMQ_2_PORT_5672_TCP_PORT=5672",
		})
		So(err, ShouldBeNil)
		So(configManager, ShouldNotBeNil)

		serviceConfigs, err := configManager.GetServiceConfigs("RABBITMQ", "5672")
		So(err, ShouldBeNil)
		So(len(serviceConfigs), ShouldEqual, 2)
		So(serviceConfigs, ShouldResemble, []*ServiceConfig{
			&ServiceConfig{
				User:  "user",
				Pass:  "pass",
				Index: "1",
				Addr:  "127.0.0.1",
				Port:  "5672",
			},
			&ServiceConfig{
				User:  "user",
				Pass:  "pass",
				Index: "2",
				Addr:  "127.0.0.1",
				Port:  "5672",
			},
		})
	})

	Convey("A service at an unspecified index should assume index 1, and should include additional service indices", t, func() {
		configManager, err := NewArrayConfigManager([]string{
			"RABBITMQ_PORT_5672_TCP_ADDR=127.0.0.1",
			"RABBITMQ_PORT_5672_TCP_PORT=5672",
			"RABBITMQ_2_PORT_5672_TCP_ADDR=127.0.0.2",
			"RABBITMQ_2_PORT_5672_TCP_PORT=5672",
		})
		So(err, ShouldBeNil)
		So(configManager, ShouldNotBeNil)

		serviceConfigs, err := configManager.GetServiceConfigs("RABBITMQ", "5672")
		So(err, ShouldBeNil)
		So(len(serviceConfigs), ShouldEqual, 2)
		So(serviceConfigs, ShouldResemble, []*ServiceConfig{
			&ServiceConfig{
				User:  "",
				Pass:  "",
				Index: "1",
				Addr:  "127.0.0.1",
				Port:  "5672",
			},
			&ServiceConfig{
				User:  "",
				Pass:  "",
				Index: "2",
				Addr:  "127.0.0.2",
				Port:  "5672",
			},
		})
	})
}

func TestParseServiceVariableString(t *testing.T) {
	Convey("Parsing an service variable key should return an error", t, func() {
		serviceVariableKey, err := parseServiceVariableKeyString("")
		So(serviceVariableKey, ShouldBeNil)
		So(err, ShouldEqual, InvalidServiceVariableFormat)
	})

	Convey("Parsing a valid indexed service should produce a valid service variable key", t, func() {
		serviceVariableKey, err := parseServiceVariableKeyString("RABBITMQ_1_PORT_5672_TCP_ADDR")
		So(err, ShouldBeNil)
		So(serviceVariableKey, ShouldNotBeNil)
		So(serviceVariableKey, ShouldResemble, &ServiceVariableKey{
			Name:  "RABBITMQ",
			Index: "1",
			Port:  "5672",
			Key:   "TCP_ADDR",
		})
	})

	Convey("Parsing a valid indexed service should produce a valid service variable key", t, func() {
		serviceVariableKey, err := parseServiceVariableKeyString("RABBITMQ_1_PORT_5672_USER")
		So(err, ShouldBeNil)
		So(serviceVariableKey, ShouldNotBeNil)
		So(serviceVariableKey, ShouldResemble, &ServiceVariableKey{
			Name:  "RABBITMQ",
			Index: "1",
			Port:  "5672",
			Key:   "USER",
		})
	})
}

func TestNewEtcdConfigManager(t *testing.T) {
	Convey("Providing a nil service config return an error", t, func() {
		etcdConfigManager, err := NewEtcdConfigManager(nil)
		So(etcdConfigManager, ShouldBeNil)
		So(err, ShouldNotBeNil)
	})

	Convey("Using the etcd config manager with established endpoints should return a valid service config when queried", t, func() {
		mux := http.NewServeMux()
		mux.Handle("/version", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "etcd 0.4.6")
		}))
		mux.Handle("/v2/keys/RABBITMQ/5672", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"action":"get","node":{"key":"/RABBITMQ/5672","dir":true,"nodes":[{"key":"/RABBITMQ/5672/123","dir":true,"nodes":[{"key":"/RABBITMQ/5672/123/tcp_addr","value":"127.0.0.1","modifiedIndex":3,"createdIndex":3},{"key":"/RABBITMQ/5672/123/tcp_port","value":"5672","modifiedIndex":4,"createdIndex":4}],"modifiedIndex":3,"createdIndex":3}],"modifiedIndex":3,"createdIndex":3}}`)
		}))

		server := httptest.NewServer(mux)
		defer server.Close()

		u, err := url.Parse(server.URL)
		So(err, ShouldBeNil)

		etcdConfigManager, err := NewEtcdConfigManager(&ServiceConfig{
			Addr: strings.Split(u.Host, ":")[0],
			Port: strings.Split(u.Host, ":")[1],
		})
		So(err, ShouldBeNil)
		So(etcdConfigManager, ShouldNotBeNil)
		So(etcdConfigManager.client, ShouldNotBeNil)

		serviceConfigs, err := etcdConfigManager.GetServiceConfigs("RABBITMQ", "5672")
		So(err, ShouldBeNil)
		So(serviceConfigs, ShouldResemble, []*ServiceConfig{
			&ServiceConfig{
				User:  "",
				Pass:  "",
				Index: "123",
				Addr:  "127.0.0.1",
				Port:  "5672",
			},
		})
	})
}

func TestEtcdConfigManagerFromArrayConfigManager(t *testing.T) {
	Convey("Just testing our use case where the etcd config would come from the env", t, func() {
		mux := http.NewServeMux()
		mux.Handle("/version", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "etcd 0.4.6")
		}))
		mux.Handle("/v2/keys/RABBITMQ/5672", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"action":"get","node":{"key":"/RABBITMQ/5672","dir":true,"nodes":[{"key":"/RABBITMQ/5672/123","dir":true,"nodes":[{"key":"/RABBITMQ/5672/123/tcp_addr","value":"127.0.0.1","modifiedIndex":3,"createdIndex":3},{"key":"/RABBITMQ/5672/123/tcp_port","value":"5672","modifiedIndex":4,"createdIndex":4}],"modifiedIndex":3,"createdIndex":3}],"modifiedIndex":3,"createdIndex":3}}`)
		}))

		server := httptest.NewServer(mux)
		defer server.Close()

		u, err := url.Parse(server.URL)
		So(err, ShouldBeNil)

		etcdAddr := strings.Split(u.Host, ":")[0]
		etcdPort := strings.Split(u.Host, ":")[1]

		arrayConfigManager, err := NewArrayConfigManager([]string{
			"ETCD_1_PORT_4001_TCP_ADDR=" + etcdAddr,
			"ETCD_1_PORT_4001_TCP_PORT=" + etcdPort,
		})

		etcdServiceConfigs, err := arrayConfigManager.GetServiceConfigs("ETCD", "4001")
		So(err, ShouldBeNil)
		So(etcdServiceConfigs, ShouldResemble, []*ServiceConfig{
			&ServiceConfig{
				Index: "1",
				Addr:  etcdAddr,
				Port:  etcdPort,
			},
		})

		etcdConfigManager, err := NewEtcdConfigManager(etcdServiceConfigs[0])
		So(err, ShouldBeNil)
		So(etcdConfigManager, ShouldNotBeNil)

		rabbitmqServiceConfigs, err := etcdConfigManager.GetServiceConfigs("RABBITMQ", "5672")
		So(err, ShouldBeNil)
		So(rabbitmqServiceConfigs, ShouldResemble, []*ServiceConfig{
			&ServiceConfig{
				User:  "",
				Pass:  "",
				Index: "123",
				Addr:  "127.0.0.1",
				Port:  "5672",
			},
		})
	})
}
