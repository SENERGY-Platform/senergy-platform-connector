/*
 * Copyright 2019 InfAI (CC SES)
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

package lib

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/platform-connector-lib/iot/options"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
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

func InitWebhooks(config configuration.Config, connector *platform_connector_lib.Connector, logger connectionlog.Logger, handlers []handler.Handler) *http.Server {
	router := http.NewServeMux()
	router.HandleFunc("/health", func(writer http.ResponseWriter, request *http.Request) {
		log.Println("INFO: /health received")
		msg, err := io.ReadAll(request.Body)
		log.Println("INFO: /health body =", err, string(msg))
		writer.WriteHeader(http.StatusOK)
	})

	router.HandleFunc("/publish", func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				debug.PrintStack()
				sendError(writer, fmt.Sprint(p), true)
				return
			}
		}()
		if config.Debug {
			now := time.Now()
			defer func(start time.Time) {
				log.Println("DEBUG: /publish in ", time.Now().Sub(start))
			}(now)
		}
		msg := PublishWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			sendError(writer, err.Error(), true)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /publish", msg)
		}
		if msg.Username == config.AuthClientId {
			_, err = fmt.Fprint(writer, `{"result": "ok"}`)
			if err != nil {
				log.Println("ERROR: InitWebhooks::publish unable to fprint:", err)
			}
			return
		} else {
			payload, err := base64.StdEncoding.DecodeString(msg.Payload)
			if err != nil {
				sendError(writer, err.Error(), true)
				return
			}

			for _, h := range handlers {
				handlerResult, err := h.Publish(msg.ClientId, msg.Username, msg.Topic, payload, msg.Qos)
				if err != nil && config.Debug {
					log.Println("DEBUG:", err)
				}
				switch handlerResult {
				case handler.Accepted:
					_, err = fmt.Fprint(writer, `{"result": "ok"}`)
					if err != nil {
						log.Println("ERROR: InitWebhooks::publish unable to fprint:", err)
					}
					return
				case handler.Rejected, handler.Error:
					sendError(writer, err.Error(), config.Debug)
					return
				case handler.Unhandled:
					continue
				default:
					log.Println("WARNING: unknown handler result", handlerResult)
					continue
				}
			}
			log.Println("WARNING: no matching topic handler found", msg.Topic)
			sendError(writer, "no matching topic handler found", config.Debug)
			return
		}

	})

	router.HandleFunc("/subscribe", func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				debug.PrintStack()
				sendError(writer, fmt.Sprint(p), true)
				return
			}
		}()
		msg := SubscribeWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			sendError(writer, err.Error(), true)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /subscribe", msg)
		}
		ok := []WebhookmsgTopic{}
		rejected := []WebhookmsgTopic{}
		if msg.Username == config.AuthClientId {
			fmt.Fprint(writer, `{"result": "ok"}`) //debug access
			return
		} else {
			for _, topic := range msg.Topics {
				handlerResult, err := handleTopicSubscribe(msg.ClientId, msg.Username, prepareTopic(topic.Topic), handlers)
				if err != nil && config.Debug {
					log.Println("DEBUG:", err)
				}
				switch handlerResult {
				case handler.Accepted:
					ok = append(ok, topic)
				case handler.Unhandled, handler.Rejected:
					rejected = append(rejected, topic)
				case handler.Error:
					sendError(writer, err.Error(), config.Debug)
					return
				default:
					log.Println("WARNING: unknown handler result", handlerResult)
					rejected = append(rejected, topic)
				}
			}
			sendSubscriptionResult(writer, ok, rejected)
			return
		}
	})

	router.HandleFunc("/login", func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				debug.PrintStack()
				sendError(writer, fmt.Sprint(p), true)
				return
			}
		}()
		msg := LoginWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			sendError(writer, err.Error(), true)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /login", msg)
		}

		authenticationMethod := config.MqttAuthMethod

		//log.Println("DEBUG: /login", msg)
		if msg.Username != config.AuthClientId {
			if !msg.CleanSession && config.ForceCleanSession && !contains(config.CleanSessionAllowUserList, msg.Username) {
				sendError(writer, "expect clean session", config.Debug)
				return
			}

			var token security.JwtToken
			var err error

			if authenticationMethod == "password" {
				token, err = connector.Security().GetUserToken(msg.Username, msg.Password)
			} else if authenticationMethod == "certificate" {
				// The user is already authenticated by the TLS client certificate validation in the broker
				token, err = connector.Security().ExchangeUserToken(msg.Username)
			}

			if err != nil {
				sendError(writer, err.Error(), config.Debug)
				return
			}
			if token == "" {
				sendError(writer, "access denied", config.Debug)
				return
			}
			if config.CheckHub {
				exists, err := connector.Iot().ExistsHub(msg.ClientId, token)
				if err != nil {
					sendError(writer, err.Error(), true)
					return
				}
				if !exists {
					sendError(writer, "client id is unknown as hub id", config.Debug)
				}
				return
			}
		} else if msg.Password != config.AuthClientSecret && authenticationMethod == "password" {
			sendError(writer, "access denied", config.Debug)
			return
		}

		_, err = fmt.Fprint(writer, `{"result": "ok"}`)
		if err != nil && config.Debug {
			log.Println("ERROR: InitWebhooks::login unable to fprint:", err)
		}
	})

	router.HandleFunc("/online", func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				debug.PrintStack()
				sendError(writer, fmt.Sprint(p), true)
				return
			} else {
				fmt.Fprint(writer, `{}`)
			}
		}()
		msg := OnlineWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			sendError(writer, err.Error(), true)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /online", msg)
		}

		token, err := connector.Security().Access()
		if err != nil {
			log.Println("WARNING: /online", err)
			return
		}

		exists, err := connector.Iot().ExistsHub(msg.ClientId, token)
		if err != nil {
			log.Println("WARNING: /online", err)
			return
		}

		if exists {
			err = logger.LogHubConnect(msg.ClientId)
			if err != nil {
				log.Println("WARNING: /online", err)
				return
			}
		}
	})

	//https://vernemq.com/docs/plugindevelopment/sessionlifecycle.html
	router.HandleFunc("/disconnect", func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				debug.PrintStack()
				sendError(writer, fmt.Sprint(p), true)
				return
			} else {
				fmt.Fprintf(writer, "{}")
			}
		}()
		msg := DisconnectWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			log.Println("ERROR: InitWebhooks::disconnect::jsondecoding", err)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /disconnect", msg)
		}
		token, err := connector.Security().Access()
		if err != nil {
			log.Println("ERROR: InitWebhooks::disconnect::connector.Security().Access", err)
			return
		}
		hub, err := connector.Iot().GetHub(msg.ClientId, token, options.Silent)
		if err != nil {
			if err == security.ErrorNotFound {
				return
			}
			if config.Debug {
				log.Println("DEBUG: InitWebhooks::disconnect::connector.Iot().GetHubDevicesAsId", err)
			}
			return
		}
		err = logger.LogHubDisconnect(msg.ClientId)
		if err != nil {
			log.Println("ERROR: InitWebhooks::disconnect::LogGatewayDisconnect", err)
			return
		}
		for _, localId := range hub.DeviceLocalIds {
			device, err := connector.IotCache.WithToken(token).GetDeviceByLocalId(localId)
			if err != nil {
				log.Println("ERROR: InitWebhooks::disconnect::GetDeviceByLocalId", err)
				continue
			}
			err = logger.LogDeviceDisconnect(device.Id)
			if err != nil {
				log.Println("ERROR: InitWebhooks::disconnect::LogDeviceDisconnect", err)
			}
		}
	})

	router.HandleFunc("/unsubscribe", func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				debug.PrintStack()
				sendError(writer, fmt.Sprint(p), true)
				return
			}
		}()
		msg := UnsubscribeWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			sendError(writer, err.Error(), true)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /unsubscribe", msg)
		}
		//defer json.NewEncoder(writer).Encode(map[string]interface{}{"result": "ok", "topics": msg.Topics})
		defer json.NewEncoder(writer).Encode(map[string]interface{}{"result": "ok", "topics": msg.Topics})
		if msg.Username != config.AuthClientId {
			token, err := connector.Security().GetCachedUserToken(msg.Username)
			if err != nil {
				log.Println("ERROR: InitWebhooks::unsubscribe::GenerateUserToken", err)
				return
			}
			for _, topic := range msg.Topics {
				if !strings.HasPrefix(topic, "command") {
					continue
				}
				prefix, deviceUri, _, err := handler.ParseTopic(topic)
				if err != nil {
					log.Println("ERROR: InitWebhooks::unsubscribe::parseTopic", err)
					return
				}
				if prefix != "command" {
					continue
				}
				device, err := connector.Iot().GetDeviceByLocalId(deviceUri, token)
				if err != nil {
					log.Println("ERROR: InitWebhooks::unsubscribe::DeviceUrlToIotDevice", err)
					continue
				}
				err = logger.LogDeviceDisconnect(device.Id)
				if err != nil {
					log.Println("ERROR: InitWebhooks::unsubscribe::CheckEndpointAuth", err)
					continue
				}
			}
		}
	})

	var httpHandler http.Handler

	if config.WebhookTimeout > 0 {
		httpHandler = &HttpTimeoutHandler{
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

func handleTopicSubscribe(clientId string, username string, topic string, handlers []handler.Handler) (handler.Result, error) {
	for _, h := range handlers {
		handlerResult, err := h.Subscribe(clientId, username, topic)
		if err != nil {
			return handler.Error, err
		}
		switch handlerResult {
		case handler.Accepted, handler.Rejected, handler.Error:
			return handlerResult, err
		case handler.Unhandled:
			continue
		default:
			log.Println("WARNING: unknown handler result", handlerResult)
			continue
		}
	}
	log.Println("WARNING: no matching topic handler found", topic)
	return handler.Rejected, errors.New("no matching topic handler found")
}
