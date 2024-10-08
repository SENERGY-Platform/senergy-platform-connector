/*
 * Copyright 2023 InfAI (CC SES)
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package vernemqtt

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlimit"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/webhooks/helper"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
)

func sendError(writer http.ResponseWriter, msg string, logging bool) {
	if logging {
		log.Println("DEBUG: send error:", msg)
	}
	err := json.NewEncoder(writer).Encode(map[string]map[string]string{"result": {"error": msg}})
	if err != nil {
		log.Println("ERROR: unable to send error msg:", err, msg)
	}
}

func sendIgnoreRedirect(writer http.ResponseWriter, topic string, msg string) {
	log.Println("WARNING: send ignore redirect:", topic, msg)
	err := json.NewEncoder(writer).Encode(map[string]interface{}{
		"result": "ok",
		"modifiers": map[string]interface{}{
			"topic":   "ignored/" + topic,
			"payload": base64.StdEncoding.EncodeToString([]byte(msg)),
			"retain":  false,
			"qos":     0,
		}})
	if err != nil {
		log.Println("ERROR: unable to send ignore redirect:", err, msg)
	}
}

func sendIgnoreRedirectAndNotification(writer http.ResponseWriter, connector *platform_connector_lib.Connector, user, clientId, topic, msg string) {
	msg = removeSecretsFromString(connector.Config, msg)
	sendIgnoreRedirect(writer, topic, msg)
	userId, err := connector.Security().GetUserId(user)
	if err != nil {
		log.Println("ERROR: unable to get user id", err)
		return
	}
	connector.HandleClientError(userId, clientId, "ignore message to "+topic+": "+msg)
}

func removeSecretsFromString(config platform_connector_lib.Config, input string) string {
	output := input
	secrets := map[string]string{
		config.DeviceRepoUrl:            "device-repository",
		config.DeviceManagerUrl:         "device-manager",
		config.PermQueryUrl:             "permission-search",
		config.KafkaUrl:                 "kafka",
		config.AuthClientSecret:         "***",
		config.AuthEndpoint:             "auth",
		config.DeveloperNotificationUrl: "dev-notify",
		config.JwtPrivateKey:            "***",
		config.PostgresPw:               "***",
	}
	for secret, replace := range secrets {
		if secret != "" && secret != "-" {
			output = strings.ReplaceAll(output, secret, replace)
		}
	}
	return output
}

func sendSubscriptionResult(writer http.ResponseWriter, ok []WebhookmsgTopic, rejected []WebhookmsgTopic) {
	topics := []interface{}{}
	for _, topic := range ok {
		topics = append(topics, topic)
	}
	for _, topic := range rejected {
		topics = append(topics, map[string]interface{}{
			"topic": topic.Topic,
			"qos":   128,
		})
	}
	msg := map[string]interface{}{
		"result": "ok",
		"topics": topics,
	}
	err := json.NewEncoder(writer).Encode(msg)
	if err != nil {
		log.Println("ERROR: unable to send sendSubscriptionResult msg:", err)
	}
}

func InitWebhooks(config configuration.Config, connector *platform_connector_lib.Connector, connectionLogger connectionlog.Logger, handlers []handler.Handler, connectionLimit *connectionlimit.ConnectionLimitHandler) *http.Server {
	router := http.NewServeMux()

	logger := GetLogger()

	router.HandleFunc("/health", func(writer http.ResponseWriter, request *http.Request) {
		log.Println("INFO: /health received")
		msg, err := io.ReadAll(request.Body)
		log.Println("INFO: /health body =", err, string(msg))
		writer.WriteHeader(http.StatusOK)
	})

	router.HandleFunc("/publish", func(writer http.ResponseWriter, request *http.Request) {
		publish(writer, request, config, handlers, connector)
	})

	router.HandleFunc("/subscribe", func(writer http.ResponseWriter, request *http.Request) {
		subscribe(writer, request, config, handlers)
	})

	router.HandleFunc("/login", func(writer http.ResponseWriter, request *http.Request) {
		login(writer, request, config, connector, connectionLimit, logger)
	})

	router.HandleFunc("/online", func(writer http.ResponseWriter, request *http.Request) {
		online(writer, request, config, connector, connectionLogger)
	})

	//https://vernemq.com/docs/plugindevelopment/sessionlifecycle.html
	router.HandleFunc("/disconnect", func(writer http.ResponseWriter, request *http.Request) {
		disconnect(writer, request, config, connector, connectionLogger, logger)
	})

	router.HandleFunc("/unsubscribe", func(writer http.ResponseWriter, request *http.Request) {
		unsubscribe(writer, request, config, connector, connectionLogger)
	})

	var httpHandler http.Handler

	if config.WebhookTimeout > 0 {
		httpHandler = &helper.HttpTimeoutHandler{
			Timeout:        time.Duration(config.WebhookTimeout) * time.Second,
			RequestHandler: router,
			TimeoutHandler: func() {
				f, err := os.OpenFile("timeouts.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
				if err != nil {
					go log.Println("ERROR:", err)
					time.Sleep(1 * time.Second)
					os.Exit(1)
					return
				}
				go log.Println("ERROR: webhook response timeout")
				fmt.Fprintln(f, time.Now().String())
				f.Sync()
				time.Sleep(1 * time.Second)
				os.Exit(1)
			},
		}
	} else {
		httpHandler = router
	}

	server := &http.Server{Addr: ":" + config.WebhookPort, Handler: httpHandler, WriteTimeout: 10 * time.Second, ReadTimeout: 2 * time.Second, ReadHeaderTimeout: 2 * time.Second}
	server.RegisterOnShutdown(func() {
		log.Println("DEBUG: server shutdown")
	})
	go func() {
		log.Println("Listening on ", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			log.Println("ERROR: unable to start server", err)
			log.Fatal(err)
		}
	}()

	return server
}

func contains(list []string, element string) bool {
	for _, e := range list {
		if e == element {
			return true
		}
	}
	return false
}

func prepareTopic(topic string) string {
	if strings.HasPrefix(topic, "$share/") {
		parts := strings.Split(topic, "/")
		if len(parts) < 3 {
			return topic
		} else {
			return strings.Join(parts[2:], "/")
		}
	} else {
		return topic
	}
}
