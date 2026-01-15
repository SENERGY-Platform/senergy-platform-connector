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
	"net/http"
	"runtime/debug"
	"strings"

	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
)

// unsubscribe godoc
// @Summary      unsubscribe webhook
// @Description  logs device as disconnected; all responses are with code=200, differences in swagger doc are because of technical incompatibilities of the documentation format
// @Accept       json
// @Produce      json
// @Param message body UnsubscribeWebhookMsg true "topic infos"
// @Success      200 {object} UnsubResponse
// @Failure      400 {object} ErrorResponse
// @Router       /unsubscribe [POST]
func unsubscribe(writer http.ResponseWriter, request *http.Request, config configuration.Config, connector *platform_connector_lib.Connector, logger connectionlog.Logger) {
	defer func() {
		if p := recover(); p != nil {
			if config.Debug {
				debug.PrintStack()
			}
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
	config.GetLogger().Debug("/unsubscribe", "msg", msg)
	//defer json.NewEncoder(writer).Encode(map[string]interface{}{"result": "ok", "topics": msg.Topics})
	defer json.NewEncoder(writer).Encode(UnsubResponse{Result: "ok", Topics: msg.Topics})
	if msg.Username != config.AuthClientId {
		token, err := connector.Security().GetCachedUserToken(msg.Username, model.RemoteInfo{})
		if err != nil {
			config.GetLogger().Error("InitWebhooks::unsubscribe::GenerateUserToken", "error", err, "username", msg.Username)
			return
		}
		for _, topic := range msg.Topics {
			if !strings.HasPrefix(topic, "command") {
				continue
			}
			prefix, _, deviceUri, _, err := handler.ParseTopic(topic)
			if err != nil {
				config.GetLogger().Error("InitWebhooks::unsubscribe::parseTopic", "error", err, "topic", topic)
				return
			}
			if prefix != "command" {
				continue
			}
			device, err := connector.IotCache.WithToken(token).GetDeviceByLocalId(deviceUri)
			if err != nil {
				config.GetLogger().Error("InitWebhooks::unsubscribe::DeviceUrlToIotDevice", "error", err, "deviceLocalId", deviceUri)
				continue
			}
			err = logger.LogDeviceDisconnect(device.Id)
			if err != nil {
				config.GetLogger().Error("InitWebhooks::unsubscribe::LogDeviceDisconnect", "error", err, "deviceId", device.Id)
				continue
			}
		}
	}
}
