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
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/statistics"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

func publish(writer http.ResponseWriter, request *http.Request, config configuration.Config, handlers []handler.Handler, connector *platform_connector_lib.Connector) {
	defer func() {
		if p := recover(); p != nil {
			if config.Debug {
				debug.PrintStack()
			}
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
	buf, err := io.ReadAll(request.Body)
	if err != nil {
		sendError(writer, err.Error(), true)
		return
	}
	msgSize := float64(len(buf))
	msg := PublishWebhookMsg{}
	err = json.Unmarshal(buf, &msg)
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
		statistics.SourceReceive(msgSize, msg.Username)
		for _, h := range handlers {
			handlerResult, err := h.Publish(msg.ClientId, msg.Username, msg.Topic, payload, msg.Qos, msgSize)
			if err != nil && config.Debug {
				log.Println("DEBUG:", err)
			}
			switch handlerResult {
			case handler.Accepted:
				_, err = fmt.Fprint(writer, `{"result": "ok"}`)
				if err != nil {
					log.Println("ERROR: InitWebhooks::publish unable to fprint:", err)
				}
				statistics.SourceReceiveHandled(msgSize, msg.Username)
				return
			case handler.Rejected:
				sendIgnoreRedirectAndNotification(writer, connector, msg.Username, msg.ClientId, msg.Topic, err.Error())
				return
			case handler.Error:
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
		sendIgnoreRedirectAndNotification(writer, connector, msg.Username, msg.ClientId, msg.Topic, "no matching topic handler found")
		return
	}
}
