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
	"fmt"
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/platform-connector-lib/correlation"
	"log"
	"net/http"
	"net/http/pprof"
	"time"
)

func sendError(writer http.ResponseWriter, msg string, additionalInfo ...int) {
	//http.Error(writer, fmt.Sprintf(`{"result": { "error": "%s" }}`, msg), statusCode)
	//_, err := fmt.Fprintf(writer, `{"result": { "error": "%s" }}`, msg)
	_, err := fmt.Fprintf(writer, `{"result": "next"}`)
	//_, err := fmt.Fprintf(writer, `{"result": "ok"}`)
	if err != nil {
		log.Println("ERROR: unable to send error msg:", err, msg, additionalInfo)
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

func InitWebhooks(config Config, connector *platform_connector_lib.Connector, logger *connectionlog.Logger, correlation *correlation.CorrelationService) *http.Server {
	router := http.NewServeMux()
	router.HandleFunc("/publish", func(writer http.ResponseWriter, request *http.Request) {
		if config.Debug {
			now := time.Now()
			defer func(start time.Time) {
				log.Println("DEBUG: /publish in ", time.Now().Sub(start))
			}(now)
		}
		msg := PublishWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			log.Println("ERROR: InitWebhooks::publish::jsondecoding", err)
			sendError(writer, err.Error(), http.StatusUnauthorized)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /publish", msg)
		}
		if msg.Username != config.AuthClientId {
			prefix, deviceUri, serviceUri, err := parseTopic(msg.Topic)
			if err != nil {
				log.Println("ERROR: InitWebhooks::publish::parseTopic", err)
				sendError(writer, err.Error(), http.StatusUnauthorized)
				return
			}

			token, err := connector.Security().GetCachedUserToken(msg.Username)
			if err != nil {
				log.Println("ERROR: InitWebhooks::publish::GenerateUserToken", err)
				sendError(writer, err.Error(), http.StatusUnauthorized)
				return
			}

			if config.CheckHub {
				err := checkHub(connector, token, msg.ClientId, deviceUri)
				if err != nil {
					log.Println("ERROR: InitWebhooks::publish::checkHub", err)
					sendError(writer, err.Error())
					return
				}
			}

			payload, err := base64.StdEncoding.DecodeString(msg.Payload)
			if err != nil {
				log.Println("ERROR: InitWebhooks::publish::base64decoding", err)
				sendError(writer, err.Error(), http.StatusBadRequest)
				return
			}

			switch prefix {
			case "event":
				event := platform_connector_lib.EventMsg{}
				err = json.Unmarshal(payload, &event)
				if err != nil {
					log.Println("ERROR: InitWebhooks::publish::event::json", err)
					sendError(writer, err.Error(), http.StatusBadRequest)
					return
				}
				if !config.CheckHub {
					if err := checkEvent(connector, token, deviceUri, serviceUri); err != nil {
						log.Println("ERROR: InitWebhooks::publish::event::checkEvent", err)
						sendError(writer, err.Error(), http.StatusInternalServerError)
						return
					}
				}
				if !config.MqttPublishAuthOnly {
					err = connector.HandleDeviceRefEventWithAuthToken(token, deviceUri, serviceUri, event)
					if err != nil {
						log.Println("ERROR: InitWebhooks::publish::event::HandleDeviceRefEventWithAuthToken", err)
						sendError(writer, err.Error(), http.StatusInternalServerError)
						return
					}
				}
			case "response":
				if !config.MqttPublishAuthOnly {
					msg := ResponseEnvelope{}
					err = json.Unmarshal(payload, &msg)
					if err != nil {
						log.Println("ERROR: InitWebhooks::publish::response::json", err)
						sendError(writer, err.Error(), http.StatusBadRequest)
						return
					}
					request, err := correlation.Get(msg.CorrelationId)
					if err != nil {
						log.Println("ERROR: InitWebhooks::publish::response::correlation.Get", err)
						//sendError(writer, err.Error(), http.StatusBadRequest)
						_, _ = fmt.Fprint(writer, `{"result": "ok"}`) //potentially old message; may be ignored; but dont cut connection
						return
					}
					err = connector.HandleCommandResponse(request, msg.Payload)
					if err != nil {
						log.Println("ERROR: InitWebhooks::publish::response::HandleCommandResponse", err)
						sendError(writer, err.Error(), http.StatusInternalServerError)
						return
					}
				}
			default:
				log.Println("ERROR: InitWebhooks::publish prefix not allowed", prefix, msg.Username)
				sendError(writer, "unexpected prefix", http.StatusUnauthorized)
				return
			}

		}
		_, err = fmt.Fprint(writer, `{"result": "ok"}`)
		if err != nil {
			log.Println("ERROR: InitWebhooks::publish unable to fprint:", err)
		}
	})

	router.HandleFunc("/subscribe", func(writer http.ResponseWriter, request *http.Request) {
		msg := SubscribeWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			log.Println("ERROR: InitWebhooks::subscribe::jsondecoding", err)
			sendError(writer, err.Error(), http.StatusUnauthorized)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /subscribe", msg)
		}
		ok := []WebhookmsgTopic{}
		rejected := []WebhookmsgTopic{}
		if msg.Username != config.AuthClientId {
			token, err := connector.Security().GenerateUserToken(msg.Username)
			if err != nil {
				log.Println("ERROR: InitWebhooks::subscribe::GenerateUserToken", err)
				sendError(writer, err.Error(), http.StatusUnauthorized)
				return
			}
			for _, topic := range msg.Topics {
				prefix, deviceUri, serviceUri, err := parseTopic(topic.Topic)
				if err != nil {
					log.Println("ERROR: InitWebhooks::subscribe::parseTopic", err)
					//sendError(writer, err.Error(), http.StatusUnauthorized)
					//return
					rejected = append(rejected, topic)
					continue
				}
				if config.CheckHub {
					err := checkHub(connector, token, msg.ClientId, deviceUri)
					if err != nil {
						log.Println("ERROR: InitWebhooks::subscribe::checkHub", err)
						//sendError(writer, err.Error())
						//return
						rejected = append(rejected, topic)
						continue
					}
				}
				if prefix != "command" {
					log.Println("ERROR: InitWebhooks::subscribe prefix != 'cmd'", prefix)
					//sendError(writer, "expect username as topic prefix", http.StatusUnauthorized)
					//return
					rejected = append(rejected, topic)
					continue
				}
				access, deviceId, err := userMayAccessDevice(connector.Iot(), token, deviceUri, serviceUri)
				if err != nil {
					log.Println("ERROR: InitWebhooks::subscribe::CheckEndpointAuth", err)
					sendError(writer, err.Error(), http.StatusUnauthorized)
					return
				}
				if !access {
					log.Println("ERROR: InitWebhooks::subscribe::CheckEndpointAuth", err)
					rejected = append(rejected, topic)
					continue
				}
				err = logger.LogDeviceConnect(deviceId)
				if err != nil {
					log.Println("ERROR: InitWebhooks::subscribe::CheckEndpointAuth", err)
				}
				ok = append(ok, topic)
			}
			sendSubscriptionResult(writer, ok, rejected)
		} else {
			sendError(writer, "connector does not subscribe", http.StatusUnauthorized)
		}
	})

	router.HandleFunc("/login", func(writer http.ResponseWriter, request *http.Request) {
		msg := LoginWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			log.Println("ERROR: InitWebhooks::login::jsondecoding", err)
			sendError(writer, err.Error(), http.StatusUnauthorized)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /login", msg)
		}
		//log.Println("DEBUG: /login", msg)
		if msg.Username != config.AuthClientId {
			if !msg.CleanSession {
				log.Println("ERROR: InitWebhooks::login::CleanSession", msg)
				sendError(writer, "expect clean session", http.StatusBadRequest)
			}
			token, err := connector.Security().GetUserToken(msg.Username, msg.Password)
			if err != nil {
				log.Println("ERROR: InitWebhooks::login::GetOpenidPasswordToken", err)
				sendError(writer, err.Error(), http.StatusUnauthorized)
				return
			}
			if token == "" {
				log.Println("ERROR: InitWebhooks::login::token missing", msg)
				sendError(writer, "access denied", http.StatusUnauthorized)
				return
			}
			exists, err := connector.Iot().ExistsHub(msg.ClientId, token)
			if err != nil {
				log.Println("ERROR: InitWebhooks::login::ExistsHub", err)
				sendError(writer, err.Error())
				return
			}
			if config.CheckHub && !exists {
				log.Println("ERROR: InitWebhooks::login::ExistsHub false")
				sendError(writer, "client id is unknown as hub id")
				return
			}

			connector.EnsureTopics(token, 100, 1, 1)

			if exists {
				err = logger.LogGatewayConnect(msg.ClientId)
				if err != nil {
					log.Println("ERROR: InitWebhooks::login::LogGatewayConnect", err, msg)
					sendError(writer, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		} else if msg.Password != config.AuthClientSecret {
			log.Println("ERROR: msg.Password != config.AuthClientSecret")
			sendError(writer, "access denied")
			return
		}
		_, err = fmt.Fprint(writer, `{"result": "ok"}`)
		if err != nil {
			log.Println("ERROR: InitWebhooks::login unable to fprint:", err)
		}
	})

	//https://vernemq.com/docs/plugindevelopment/sessionlifecycle.html
	router.HandleFunc("/disconnect", func(writer http.ResponseWriter, request *http.Request) {
		defer fmt.Fprintf(writer, "")
		msg := DisconnectWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			log.Println("ERROR: InitWebhooks::disconnect::jsondecoding", err)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /disconnect", msg)
		}
		err = logger.LogGatewayDisconnect(msg.ClientId)
		if err != nil {
			log.Println("ERROR: InitWebhooks::disconnect::LogGatewayDisconnect", err)
			return
		}
		token, err := connector.Security().Access()
		if err != nil {
			log.Println("ERROR: InitWebhooks::disconnect::connector.Security().Access", err)
			return
		}
		devices, err := connector.Iot().GetHubDevicesAsId(msg.ClientId, token)
		if err != nil {
			if config.Debug {
				log.Println("DEBUG: InitWebhooks::disconnect::connector.Iot().GetHubDevicesAsId", err)
			}
			return
		}
		for _, device := range devices {
			err = logger.LogDeviceDisconnect(device)
			if err != nil {
				log.Println("ERROR: InitWebhooks::disconnect::LogDeviceDisconnect", err)
			}
		}
	})

	router.HandleFunc("/unsubscribe", func(writer http.ResponseWriter, request *http.Request) {
		msg := UnsubscribeWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			log.Println("ERROR: InitWebhooks::unsubscribe::jsondecoding", err)
			sendError(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /unsubscribe", msg)
		}
		//defer json.NewEncoder(writer).Encode(map[string]interface{}{"result": "ok", "topics": msg.Topics})
		defer json.NewEncoder(writer).Encode(map[string]interface{}{"result": "ok", "topics": msg.Topics})
		if msg.Username != config.AuthClientId {
			token, err := connector.Security().GenerateUserToken(msg.Username)
			if err != nil {
				log.Println("ERROR: InitWebhooks::unsubscribe::GenerateUserToken", err)
				return
			}
			for _, topic := range msg.Topics {
				prefix, deviceUri, serviceUri, err := parseTopic(topic)
				if err != nil {
					log.Println("ERROR: InitWebhooks::unsubscribe::parseTopic", err)
					return
				}
				if prefix != "command" {
					log.Println("WARNING: InitWebhooks::unsubscribe prefix != 'command'", prefix)
					return
				}
				access, deviceId, err := userMayAccessDevice(connector.Iot(), token, deviceUri, serviceUri)
				if err != nil {
					log.Println("ERROR: InitWebhooks::unsubscribe::CheckEndpointAuth", err)
					return
				}
				if !access {
					log.Println("ERROR: InitWebhooks::unsubscribe::CheckEndpointAuth", err)
					return
				}
				err = logger.LogDeviceDisconnect(deviceId)
				if err != nil {
					log.Println("ERROR: InitWebhooks::unsubscribe::CheckEndpointAuth", err)
					return
				}
			}
		}
	})

	if config.Debug {
		router.HandleFunc("/debug/pprof/", pprof.Index)
		router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		router.HandleFunc("/debug/pprof/profile", pprof.Profile)
		router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		router.HandleFunc("/debug/pprof/trace", pprof.Trace)

		router.Handle("/debug/pprof/block", pprof.Handler("block"))
		router.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		router.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		router.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	}

	server := &http.Server{Addr: ":" + config.WebhookPort, Handler: router}
	go func() {
		log.Println("Listening on ", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			log.Println("ERROR: unable to start server", err)
			log.Fatal(err)
		}
	}()
	return server
}
