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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/platform-connector-lib/correlation"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"time"
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

func InitWebhooks(config Config, connector *platform_connector_lib.Connector, logger connectionlog.Logger, correlation *correlation.CorrelationService) *http.Server {
	router := http.NewServeMux()
	router.HandleFunc("/health", func(writer http.ResponseWriter, request *http.Request) {
		msg, err := ioutil.ReadAll(request.Body)
		log.Println("INFO: /health", err, string(msg))
		writer.WriteHeader(http.StatusOK)
	})

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
			sendError(writer, err.Error(), true)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /publish", msg)
		}
		if msg.Username != config.AuthClientId {
			prefix, deviceUri, serviceUri, err := parseTopic(msg.Topic)
			if err != nil {
				sendError(writer, err.Error(), config.Debug)
				return
			}

			token, err := connector.Security().GetCachedUserToken(msg.Username)
			if err != nil {
				sendError(writer, err.Error(), true)
				return
			}

			if config.CheckHub {
				err := checkHub(connector, token, msg.ClientId, deviceUri)
				if err != nil {
					sendError(writer, err.Error(), config.Debug)
					return
				}
			}

			payload, err := base64.StdEncoding.DecodeString(msg.Payload)
			if err != nil {
				sendError(writer, err.Error(), true)
				return
			}

			switch prefix {
			case "event":
				event := platform_connector_lib.EventMsg{}
				err = json.Unmarshal(payload, &event)
				if err != nil {
					sendError(writer, err.Error(), true)
					return
				}
				if !config.CheckHub {
					if err := checkEvent(connector, token, deviceUri, serviceUri); err != nil {
						if err == ServiceNotFound {
							_, err = fmt.Fprint(writer, `{"result": "ok"}`) //ignore event but allow mqtt-publish
							log.Println("DEBUG: got event for unknown service of known device", deviceUri, serviceUri)
							return
						} else {
							sendError(writer, err.Error(), config.Debug)
							return
						}
					}
				}
				if !config.MqttPublishAuthOnly {
					err = connector.HandleDeviceRefEventWithAuthToken(token, deviceUri, serviceUri, event)
					if err != nil {
						sendError(writer, err.Error(), true)
						return
					}
				}
			case "response":
				if !config.MqttPublishAuthOnly {
					msg := ResponseEnvelope{}
					err = json.Unmarshal(payload, &msg)
					if err != nil {
						sendError(writer, err.Error(), true)
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
						sendError(writer, err.Error(), true)
						return
					}
				}
			default:
				sendError(writer, "unexpected prefix", config.Debug)
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
			sendError(writer, err.Error(), true)
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
				sendError(writer, err.Error(), config.Debug)
				return
			}
			for _, topic := range msg.Topics {
				prefix, deviceUri, _, err := parseTopic(topic.Topic)
				if err != nil {
					if config.Debug {
						log.Println("ERROR: InitWebhooks::subscribe::parseTopic", err)
					}
					rejected = append(rejected, topic)
					continue
				}
				if config.CheckHub {
					err := checkHub(connector, token, msg.ClientId, deviceUri)
					if err != nil {
						if config.Debug {
							log.Println("ERROR: InitWebhooks::subscribe::checkHub", err)
						}
						rejected = append(rejected, topic)
						continue
					}
				}
				if prefix != "command" {
					if config.Debug {
						log.Println("ERROR: InitWebhooks::subscribe prefix != 'cmd'", prefix)
					}
					rejected = append(rejected, topic)
					continue
				}
				device, err := connector.Iot().GetDeviceByLocalId(deviceUri, token)
				if err != nil {
					if config.Debug {
						log.Println("WARNING: InitWebhooks::subscribe::DeviceUrlToIotDevice", err)
					}
					rejected = append(rejected, topic)
					continue
				}
				err = logger.LogDeviceConnect(device.Id)
				if err != nil {
					if config.Debug {
						log.Println("ERROR: InitWebhooks::subscribe::CheckEndpointAuth", err)
					}
				}
				ok = append(ok, topic)
			}
			sendSubscriptionResult(writer, ok, rejected)
		} else {
			sendError(writer, "connector does not subscribe", true)
		}
	})

	router.HandleFunc("/login", func(writer http.ResponseWriter, request *http.Request) {
		msg := LoginWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			sendError(writer, err.Error(), true)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /login", msg)
		}
		//log.Println("DEBUG: /login", msg)
		if msg.Username != config.AuthClientId {
			if !msg.CleanSession {
				sendError(writer, "expect clean session", config.Debug)
				return
			}
			token, err := connector.Security().GetUserToken(msg.Username, msg.Password)
			if err != nil {
				sendError(writer, err.Error(), true)
				return
			}
			if token == "" {
				sendError(writer, "access denied", config.Debug)
				return
			}
			exists, err := connector.Iot().ExistsHub(msg.ClientId, token)
			if err != nil {
				sendError(writer, err.Error(), true)
				return
			}
			if config.CheckHub && !exists {
				sendError(writer, "client id is unknown as hub id", config.Debug)
				return
			}

			if exists {
				err = logger.LogHubConnect(msg.ClientId)
				if err != nil {
					sendError(writer, err.Error(), true)
					return
				}
			}
		} else if msg.Password != config.AuthClientSecret {
			sendError(writer, "access denied", config.Debug)
			return
		}
		_, err = fmt.Fprint(writer, `{"result": "ok"}`)
		if err != nil && config.Debug {
			log.Println("ERROR: InitWebhooks::login unable to fprint:", err)
		}
	})

	//https://vernemq.com/docs/plugindevelopment/sessionlifecycle.html
	router.HandleFunc("/disconnect", func(writer http.ResponseWriter, request *http.Request) {
		defer fmt.Fprintf(writer, "{}")
		msg := DisconnectWebhookMsg{}
		err := json.NewDecoder(request.Body).Decode(&msg)
		if err != nil {
			log.Println("ERROR: InitWebhooks::disconnect::jsondecoding", err)
			return
		}
		if config.Debug {
			log.Println("DEBUG: /disconnect", msg)
		}
		err = logger.LogHubDisconnect(msg.ClientId)
		if err != nil {
			log.Println("ERROR: InitWebhooks::disconnect::LogGatewayDisconnect", err)
			return
		}
		token, err := connector.Security().Access()
		if err != nil {
			log.Println("ERROR: InitWebhooks::disconnect::connector.Security().Access", err)
			return
		}
		hub, err := connector.Iot().GetHub(msg.ClientId, token)
		if err != nil {
			if config.Debug {
				log.Println("DEBUG: InitWebhooks::disconnect::connector.Iot().GetHubDevicesAsId", err)
			}
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
			token, err := connector.Security().GenerateUserToken(msg.Username)
			if err != nil {
				log.Println("ERROR: InitWebhooks::unsubscribe::GenerateUserToken", err)
				return
			}
			for _, topic := range msg.Topics {
				prefix, deviceUri, _, err := parseTopic(topic)
				if err != nil {
					log.Println("ERROR: InitWebhooks::unsubscribe::parseTopic", err)
					return
				}
				if prefix != "command" {
					log.Println("WARNING: InitWebhooks::unsubscribe prefix != 'command'", prefix)
					return
				}
				device, err := connector.Iot().GetDeviceByLocalId(deviceUri, token)
				if err != nil {
					log.Println("ERROR: InitWebhooks::unsubscribe::DeviceUrlToIotDevice", err)
					return
				}
				err = logger.LogDeviceDisconnect(device.Id)
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

		runtime.SetBlockProfileRate(int(time.Second.Nanoseconds())) //one sample per second
		runtime.SetMutexProfileFraction(1)
		router.Handle("/debug/pprof/block", pprof.Handler("block"))
		router.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
		router.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		router.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		router.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	}

	server := &http.Server{Addr: ":" + config.WebhookPort, Handler: router, WriteTimeout: 10 * time.Second, ReadTimeout: 2 * time.Second, ReadHeaderTimeout: 2 * time.Second}
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

	if config.SelfCheck {
		go func() {
			defer log.Fatal("ERROR: self check terminated")
			ticker := time.NewTicker(1 * time.Minute)
			for t := range ticker.C {
				go selfCheck(config, t)
			}
		}()
	}

	return server
}

func selfCheck(config Config, t time.Time) {
	//ensure exit in deadlock
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		<-ctx.Done()
		if ctx.Err() != context.Canceled {
			go func() {
				log.Fatal("FATAL: connectivity test by context:", ctx.Err())
			}()
			//ensure os.Exit if logging is blocked
			time.Sleep(200 * time.Millisecond)
			os.Exit(1)
		}
	}()

	log.Println("INFO: connectivity test start:", t.String())
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Post("http://localhost:"+config.WebhookPort+"/health", "application/json", bytes.NewBuffer([]byte("local connection test: "+t.String())))
	if err != nil {
		if config.SelfCheckFatal {
			log.Fatal("FATAL: connectivity test:", err)
		} else {
			log.Println("ERROR: connectivity test:", err)
		}
	} else {
		log.Println("INFO: connectivity test ok:", t.String())
		ioutil.ReadAll(resp.Body)
	}
	resp.Body.Close()
}
