package docker

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/ory/dockertest/v3"
	uuid "github.com/satori/go.uuid"
)

func Vernemqtt(pool *dockertest.Pool, ctx context.Context, connecorUrl string, config configuration.Config) (brokerUrlForConnector string, brokerUrlForClients string, err error) {
	log.Println("start mqtt")
	var container *dockertest.Resource
	if config.MqttAuthMethod == "password" {
		container, err = pool.Run("erlio/docker-vernemq", "latest", []string{
			"DOCKER_VERNEMQ_ACCEPT_EULA=yes",
			"DOCKER_VERNEMQ_LOG__CONSOLE__LEVEL=debug",
			"DOCKER_VERNEMQ_SHARED_SUBSCRIPTION_POLICY=random",
			"DOCKER_VERNEMQ_PLUGINS__VMQ_WEBHOOKS=on",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE__HOOK=auth_on_subscribe",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE__ENDPOINT=http://" + connecorUrl + "/subscribe",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH__HOOK=auth_on_publish",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH__ENDPOINT=http://" + connecorUrl + "/publish",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG__HOOK=auth_on_register",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG__ENDPOINT=http://" + connecorUrl + "/login",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLOFF__HOOK=on_client_offline",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLOFF__ENDPOINT=http://" + connecorUrl + "/disconnect",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR__HOOK=on_unsubscribe",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR__ENDPOINT=http://" + connecorUrl + "/unsubscribe",

			// These plugins need to be deactivated so that the webhooks for auth_on_publish/subscribe are called
			"DOCKER_VERNEMQ_PLUGINS__VMQ_PASSWD=off",
			"DOCKER_VERNEMQ_PLUGINS__VMQ_ACL=off",
		})
		brokerUrlForClients = "tcp://" + container.Container.NetworkSettings.IPAddress + ":1883"

	} else if config.MqttAuthMethod == "certificate" {
		caCertificateFileName := "ca.crt"
		serverCertificateFileName := "server.crt"
		privateKeyFileName := "private.key"
		dir, err := os.Getwd()
		if err != nil {
			log.Println(err)
		}
		options := dockertest.RunOptions{
			Repository: "erlio/docker-vernemq",
			Tag:        "latest",
			Env: []string{
				"DOCKER_VERNEMQ_ACCEPT_EULA=yes",
				"DOCKER_VERNEMQ_LOG__CONSOLE__LEVEL=debug",
				"DOCKER_VERNEMQ_SHARED_SUBSCRIPTION_POLICY=random",
				"DOCKER_VERNEMQ_PLUGINS__VMQ_WEBHOOKS=on",
				"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE__HOOK=auth_on_subscribe",
				"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE__ENDPOINT=http://" + connecorUrl + "/subscribe",
				"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH__HOOK=auth_on_publish",
				"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH__ENDPOINT=http://" + connecorUrl + "/publish",
				"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG__HOOK=auth_on_register",
				"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG__ENDPOINT=http://" + connecorUrl + "/login",
				"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLOFF__HOOK=on_client_offline",
				"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLOFF__ENDPOINT=http://" + connecorUrl + "/disconnect",
				"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR__HOOK=on_unsubscribe",
				"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR__ENDPOINT=http://" + connecorUrl + "/unsubscribe",
				"DOCKER_VERNEMQ_PLUGINS__VMQ_PASSWD=off",
				"DOCKER_VERNEMQ_PLUGINS__VMQ_ACL=off",
				"DOCKER_VERNEMQ_LISTENER__SSL__REQUIRE_CERTIFICATE=on",
				"DOCKER_VERNEMQ_LISTENER__SSL__USE_IDENTITY_AS_USERNAME=on",
				"DOCKER_VERNEMQ_LISTENER__SSL__CAFILE=/etc/certs/ca/" + caCertificateFileName,
				"DOCKER_VERNEMQ_LISTENER__SSL__CERTFILE=/etc/certs/server/" + serverCertificateFileName,
				"DOCKER_VERNEMQ_LISTENER__SSL__KEYFILE=/etc/certs/server/" + privateKeyFileName,
				"DOCKER_VERNEMQ_LISTENER__SSL__DEFAULT=0.0.0.0:8883",
			},
			Mounts: []string{
				// TODO: depends on where the tests are run / or /test
				filepath.Join(dir, "mqtt_certs", "broker") + ":/etc/certs/server",
				filepath.Join(dir, "mqtt_certs", "ca") + ":/etc/certs/ca",
			},
		}
		container, err = pool.RunWithOptions(&options)
		brokerUrlForClients = "ssl://" + container.Container.NetworkSettings.IPAddress + ":8883"
	}
	brokerUrlForConnector = "tcp://" + container.Container.NetworkSettings.IPAddress + ":1883"

	if err != nil {
		return "", "", err
	}
	go Dockerlog(pool, ctx, container, "VERNEMQ")
	go func() {
		<-ctx.Done()
		log.Println("DEBUG: remove container " + container.Container.Name)
		container.Close()
	}()

	//mock auth webhooks for initial connection check
	server := &http.Server{
		Addr:              ":" + config.WebhookPort,
		WriteTimeout:      10 * time.Second,
		ReadTimeout:       2 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			fmt.Fprint(writer, `{"result": "ok"}`)
		})}
	go func() {
		log.Println("Mock Listening on ", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Println("ERROR: unable to start server", err)
		}
	}()
	defer func() {
		server.Shutdown(context.Background())
	}()

	err = pool.Retry(func() error {
		log.Println("DEBUG: try to connection to broker")
		options := paho.NewClientOptions().
			SetAutoReconnect(true).
			SetCleanSession(true).
			SetClientID(uuid.NewV4().String()).
			AddBroker(brokerUrlForConnector)

		client := paho.NewClient(options)
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			log.Println("Error on Mqtt.Connect(): ", token.Error())
			return token.Error()
		}
		defer client.Disconnect(0)
		return nil
	})
	return brokerUrlForConnector, brokerUrlForClients, err
}

func VernemqWithManagementApi(pool *dockertest.Pool, ctx context.Context, wg *sync.WaitGroup) (brokerUrl string, managementUrl string, err error) {
	log.Println("start mqtt")
	container, err := pool.Run("erlio/docker-vernemq", "1.9.1-alpine", []string{
		"DOCKER_VERNEMQ_ALLOW_ANONYMOUS=on",
		"DOCKER_VERNEMQ_LOG__CONSOLE__LEVEL=debug",
		"DOCKER_VERNEMQ_SHARED_SUBSCRIPTION_POLICY=random",
	})
	if err != nil {
		return "", "", err
	}
	go Dockerlog(pool, ctx, container, "VERNEMQ")
	wg.Add(1)
	go func() {
		<-ctx.Done()
		log.Println("DEBUG: remove container " + container.Container.Name)
		container.Close()
		wg.Done()
	}()
	err = pool.Retry(func() error {
		log.Println("DEBUG: try to connection to broker")
		options := paho.NewClientOptions().
			SetAutoReconnect(true).
			SetCleanSession(true).
			SetClientID(uuid.NewV4().String()).
			AddBroker("tcp://" + container.Container.NetworkSettings.IPAddress + ":1883")

		client := paho.NewClient(options)
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			log.Println("Error on Mqtt.Connect(): ", token.Error())
			return token.Error()
		}
		defer client.Disconnect(0)
		return nil
	})
	execLog := &LogWriter{Logger: log.New(os.Stdout, "[VERNEMQ-EXEC]", 0)}
	code, err := container.Exec([]string{"vmq-admin", "api-key", "add", "key=testkey"}, dockertest.ExecOptions{StdErr: execLog, StdOut: execLog})
	if err != nil {
		log.Panic("ERROR", code, err)
		return "", "", err
	}
	return "tcp://" + container.Container.NetworkSettings.IPAddress + ":1883", "http://testkey@" + container.Container.NetworkSettings.IPAddress + ":8888", err
}
