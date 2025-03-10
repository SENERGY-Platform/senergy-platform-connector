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
	"encoding/json"
	"fmt"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"log"
	"net/http"
	"runtime/debug"
)

func subscribe(writer http.ResponseWriter, request *http.Request, config configuration.Config, handlers []handler.Handler) {
	defer func() {
		if p := recover(); p != nil {
			if config.Debug {
				debug.PrintStack()
			}
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
			handlerResult, err := handler.HandleTopicSubscribe(msg.ClientId, msg.Username, prepareTopic(topic.Topic), handlers)
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
}
