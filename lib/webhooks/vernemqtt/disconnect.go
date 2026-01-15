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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/platform-connector-lib/iot/options"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
)

// disconnect godoc
// @Summary      disconnected webhook
// @Description  logs user hubs and devices as disconnected; all responses are with code=200, differences in swagger doc are because of technical incompatibilities of the documentation format
// @Accept       json
// @Produce      json
// @Param        message body DisconnectWebhookMsg true "disconnect info"
// @Success      200 {object}  EmptyResponse
// @Failure      400 {object}  ErrorResponse
// @Router       /disconnect [POST]
func disconnect(writer http.ResponseWriter, request *http.Request, config configuration.Config, connector *platform_connector_lib.Connector, connectionLogger connectionlog.Logger, logger *slog.Logger) {
	defer func() {
		if p := recover(); p != nil {
			if config.Debug {
				debug.PrintStack()
			}
			sendError(writer, fmt.Sprint(p), true)
			return
		} else {
			fmt.Fprintf(writer, "{}")
		}
	}()
	msg := DisconnectWebhookMsg{}
	err := json.NewDecoder(request.Body).Decode(&msg)
	if err != nil {
		config.GetLogger().Error("InitWebhooks::disconnect::jsondecoding", "error", err)
		return
	}

	logger.Info("disconnect", "action", "disconnect", "clientId", msg.ClientId)
	token, err := connector.Security().Access()
	if err != nil {
		config.GetLogger().Error("InitWebhooks::disconnect::connector.Security().Access", "error", err)
		return
	}
	hub, err := connector.Iot().GetHub(msg.ClientId, token, options.Silent)
	if err != nil {
		if errors.Is(err, security.ErrorNotFound) {
			config.GetLogger().Warn("no hub found", "error", err, "clientId", msg.ClientId)
			return
		}
		config.GetLogger().Error("InitWebhooks::disconnect::connector.Iot().GetHub", "error", err, "clientId", msg.ClientId)
		return
	}
	userToken, err := connector.Security().GetCachedUserToken(hub.OwnerId, model.RemoteInfo{})
	if err != nil {
		config.GetLogger().Error("InitWebhooks::disconnect::connector.Security().GetCachedUserToken", "error", err, "clientId", msg.ClientId)
		return
	}
	err = connectionLogger.LogHubDisconnect(msg.ClientId)
	if err != nil {
		config.GetLogger().Error("InitWebhooks::disconnect::LogHubDisconnect", "error", err, "clientId", msg.ClientId)
		return
	}
	for _, localId := range hub.DeviceLocalIds {
		device, err := connector.IotCache.WithToken(userToken).GetDeviceByLocalId(localId)
		if err != nil {
			config.GetLogger().Error("InitWebhooks::disconnect::GetDeviceByLocalId", "error", err, "localId", localId)
			continue
		}
		err = connectionLogger.LogDeviceDisconnect(device.Id)
		if err != nil {
			config.GetLogger().Error("InitWebhooks::disconnect::LogDeviceDisconnect", "error", err, "deviceId", device.Id)
		}
	}
}
