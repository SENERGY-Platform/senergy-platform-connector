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
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SENERGY-Platform/platform-connector-lib/connectionlimit"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/webhooks/helper"
	"github.com/swaggo/swag"

	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
)

//go:generate go tool swag init -o ../../../docs --parseDependency -d .. -g vernemqtt/webhooks.go

func sendError(writer http.ResponseWriter, msg string, logging bool) {
	if logging {
		log.Println("DEBUG: send error:", msg)
	}
	err := json.NewEncoder(writer).Encode(ErrorResponse{Result: ErrorResponseResult{Error: msg}})
	if err != nil {
		log.Println("ERROR: unable to send error msg:", err, msg)
	}
}

func sendIgnoreRedirect(writer http.ResponseWriter, topic string, msg string) {
	log.Println("WARNING: send ignore redirect:", topic, msg)
	err := json.NewEncoder(writer).Encode(RedirectResponse{
		Result: "ok",
		Modifiers: RedirectModifiers{
			Topic:   "ignored/" + topic,
			Payload: base64.StdEncoding.EncodeToString([]byte(msg)),
			Retain:  false,
			Qos:     0,
		},
	})
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
		config.PermissionsV2Url:         "permissions-v2",
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
	topics := []WebhookmsgTopic{}
	for _, topic := range ok {
		topics = append(topics, topic)
	}
	for _, topic := range rejected {
		topics = append(topics, WebhookmsgTopic{
			Topic: topic.Topic,
			Qos:   128,
		})
	}
	err := json.NewEncoder(writer).Encode(SubscriptionResponse{
		Result: "ok",
		Topics: topics,
	})
	if err != nil {
		log.Println("ERROR: unable to send sendSubscriptionResult msg:", err)
	}
}

// InitWebhooks doc
// @title         Senergy-Connector-Webhooks
// @description   webhooks for vernemqtt; all responses are with code=200, differences in swagger doc are because of technical incompatibilities of the documentation format
// @version       0.1
// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html
// @BasePath  /
func InitWebhooks(config configuration.Config, connector *platform_connector_lib.Connector, connectionLogger connectionlog.Logger, handlers []handler.Handler, connectionLimit *connectionlimit.ConnectionLimitHandler) *http.Server {
	router := http.NewServeMux()

	logger := config.GetLogger().With("snrgy-log-type", "connector-webhook")

	router.HandleFunc("GET /doc", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		doc, err := swag.ReadDoc()
		if err != nil {
			http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		//remove empty host to enable developer-swagger-api service to replace it; can not use cleaner delete on json object, because developer-swagger-api is sensible to formatting; better alternative is refactoring of developer-swagger-api/apis/db/db.py
		doc = strings.Replace(doc, `"host": "",`, "", 1)
		_, _ = writer.Write([]byte(doc))
	})

	router.HandleFunc("/disconnect-command", func(writer http.ResponseWriter, request *http.Request) {
		disconnectCommand(writer, request, config, logger)
	})

	router.HandleFunc("/health", func(writer http.ResponseWriter, request *http.Request) {
		config.GetLogger().Info("/health received")
		msg, err := io.ReadAll(request.Body)
		config.GetLogger().Info("/health body", "error", err, "msg", string(msg))
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
					go config.GetLogger().Error("ERROR", "error", err)
					time.Sleep(1 * time.Second)
					os.Exit(1)
					return
				}
				go config.GetLogger().Error("webhook response timeout")
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
		config.GetLogger().Debug("server shutdown")
	})
	go func() {
		config.GetLogger().Info("start server", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			config.GetLogger().Error("unable to start server", "error", err)
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
