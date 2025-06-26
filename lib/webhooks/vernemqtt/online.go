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
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"log"
	"net/http"
	"runtime/debug"
)

// online godoc
// @Summary      online webhook
// @Description  logs hub as connected; all responses are with code=200, differences in swagger doc are because of technical incompatibilities of the documentation format
// @Accept       json
// @Produce      json
// @Param message body OnlineWebhookMsg true "client infos"
// @Success      200 {object} EmptyResponse
// @Failure      400 {object} ErrorResponse
// @Router       /online [POST]
func online(writer http.ResponseWriter, request *http.Request, config configuration.Config, connector *platform_connector_lib.Connector, logger connectionlog.Logger) {
	defer func() {
		if p := recover(); p != nil {
			if config.Debug {
				debug.PrintStack()
			}
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
	log.Println("/online", msg.ClientId)

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
}
