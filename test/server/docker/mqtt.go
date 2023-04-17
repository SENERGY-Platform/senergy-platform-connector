package docker

import (
	"context"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func Vernemqtt(ctx context.Context, wg *sync.WaitGroup, connecorUrl string, config configuration.Config) (brokerUrlForConnector string, brokerUrlForClients string, err error) {
	log.Println("start mqtt")
	ports := []string{"1883/tcp"}
	env := map[string]string{}
	var mounts testcontainers.ContainerMounts
	if config.MqttAuthMethod == "certificate" {
		ports = append(ports, "8883/tcp")
		caCertificateFileName := "ca.crt"
		serverCertificateFileName := "server.crt"
		privateKeyFileName := "private.key"
		dir, err := os.Getwd()
		if err != nil {
			log.Println(err)
		}
		mounts = testcontainers.ContainerMounts{
			{Source: testcontainers.GenericBindMountSource{HostPath: filepath.Join(dir, "mqtt_certs", "broker")}, Target: "/etc/certs/server"},
			{Source: testcontainers.GenericBindMountSource{HostPath: filepath.Join(dir, "mqtt_certs", "ca")}, Target: "/etc/certs/ca"},
		}
		env = map[string]string{
			"DOCKER_VERNEMQ_ACCEPT_EULA":                             "yes",
			"DOCKER_VERNEMQ_LOG__CONSOLE__LEVEL":                     "debug",
			"DOCKER_VERNEMQ_SHARED_SUBSCRIPTION_POLICY":              "random",
			"DOCKER_VERNEMQ_PLUGINS__VMQ_WEBHOOKS":                   "on",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE__HOOK":       "auth_on_subscribe",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE__ENDPOINT":   "http://" + connecorUrl + "/subscribe",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH__HOOK":         "auth_on_publish",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH__ENDPOINT":     "http://" + connecorUrl + "/publish",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG__HOOK":             "auth_on_register",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG__ENDPOINT":         "http://" + connecorUrl + "/login",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLOFF__HOOK":             "on_client_offline",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLOFF__ENDPOINT":         "http://" + connecorUrl + "/disconnect",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR__HOOK":        "on_unsubscribe",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR__ENDPOINT":    "http://" + connecorUrl + "/unsubscribe",
			"DOCKER_VERNEMQ_PLUGINS__VMQ_PASSWD":                     "off",
			"DOCKER_VERNEMQ_PLUGINS__VMQ_ACL":                        "off",
			"DOCKER_VERNEMQ_LISTENER__SSL__REQUIRE_CERTIFICATE":      "on",
			"DOCKER_VERNEMQ_LISTENER__SSL__USE_IDENTITY_AS_USERNAME": "on",
			"DOCKER_VERNEMQ_LISTENER__SSL__CAFILE":                   "/etc/certs/ca/" + caCertificateFileName,
			"DOCKER_VERNEMQ_LISTENER__SSL__CERTFILE":                 "/etc/certs/server/" + serverCertificateFileName,
			"DOCKER_VERNEMQ_LISTENER__SSL__KEYFILE":                  "/etc/certs/server/" + privateKeyFileName,
			"DOCKER_VERNEMQ_LISTENER__SSL__DEFAULT":                  "0.0.0.0:8883",
		}
	} else {
		env = map[string]string{
			"DOCKER_VERNEMQ_ACCEPT_EULA":                           "yes",
			"DOCKER_VERNEMQ_LOG__CONSOLE__LEVEL":                   "debug",
			"DOCKER_VERNEMQ_SHARED_SUBSCRIPTION_POLICY":            "random",
			"DOCKER_VERNEMQ_PLUGINS__VMQ_WEBHOOKS":                 "on",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE__HOOK":     "auth_on_subscribe",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE__ENDPOINT": "http://" + connecorUrl + "/subscribe",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH__HOOK":       "auth_on_publish",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH__ENDPOINT":   "http://" + connecorUrl + "/publish",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG__HOOK":           "auth_on_register",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG__ENDPOINT":       "http://" + connecorUrl + "/login",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLOFF__HOOK":           "on_client_offline",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLOFF__ENDPOINT":       "http://" + connecorUrl + "/disconnect",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR__HOOK":      "on_unsubscribe",
			"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR__ENDPOINT":  "http://" + connecorUrl + "/unsubscribe",

			// These plugins need to be deactivated so that the webhooks for auth_on_publish/subscribe are called
			"DOCKER_VERNEMQ_PLUGINS__VMQ_PASSWD": "off",
			"DOCKER_VERNEMQ_PLUGINS__VMQ_ACL":    "off",
		}
	}

	if config.MqttVersion != "" && config.MqttVersion != "3,4" {
		env["DOCKER_VERNEMQ_LISTENER__TCP__ALLOWED_PROTOCOL_VERSIONS"] = config.MqttVersion
		env["DOCKER_VERNEMQ_LISTENER.tcp.allowed_protocol_versions"] = config.MqttVersion
		env["DOCKER_VERNEMQ_LISTENER__TCP__ALLOWED_protocol_versions"] = config.MqttVersion

		env["DOCKER_VERNEMQ_LISTENER__SSL__ALLOWED_PROTOCOL_VERSIONS"] = config.MqttVersion
		env["DOCKER_VERNEMQ_LISTENER.ssl.allowed_protocol_versions"] = config.MqttVersion
		env["DOCKER_VERNEMQ_LISTENER__SSL__ALLOWED_protocol_versions"] = config.MqttVersion
	}
	if strings.Contains(config.MqttVersion, "5") {
		env["DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE5__HOOK"] = "auth_on_subscribe_m5"
		env["DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE5__ENDPOINT"] = "http://" + connecorUrl + "/subscribe"
		env["DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH5__HOOK"] = "auth_on_publish_m5"
		env["DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH5__ENDPOINT"] = "http://" + connecorUrl + "/publish"
		env["DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG5__HOOK"] = "auth_on_register_m5"
		env["DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG5__ENDPOINT"] = "http://" + connecorUrl + "/login"
		env["DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR5__HOOK"] = "on_unsubscribe_m5"
		env["DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR5__ENDPOINT"] = "http://" + connecorUrl + "/unsubscribe"
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:           "erlio/docker-vernemq:latest",
			Tmpfs:           map[string]string{},
			WaitingFor:      wait.ForListeningPort("1883/tcp"),
			AlwaysPullImage: true,
			Env:             env,
			Mounts:          mounts,
		},
		Started: true,
	})
	if err != nil {
		return "", "", err
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		log.Println("DEBUG: remove container mqtt", c.Terminate(context.Background()))
	}()

	err = Dockerlog(ctx, c, "MQTT")
	if err != nil {
		return "", "", err
	}

	ipAddress, err := c.ContainerIP(ctx)
	if err != nil {
		return "", "", err
	}

	if config.MqttAuthMethod == "certificate" {
		brokerUrlForClients = "ssl://" + ipAddress + ":8883"
	} else {
		brokerUrlForClients = "tcp://" + ipAddress + ":1883"
	}
	brokerUrlForConnector = "tcp://" + ipAddress + ":1883"

	return brokerUrlForConnector, brokerUrlForClients, err
}
