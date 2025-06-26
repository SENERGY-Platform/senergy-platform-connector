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
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlimit"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"log"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
)

// login godoc
// @Summary      login webhook
// @Description  checks auth; all responses are with code=200, differences in swagger doc are because of technical incompatibilities of the documentation format
// @Accept       json
// @Produce      json
// @Param        message body LoginWebhookMsg true "login infos"
// @Success      200 {object}  OkResponse
// @Failure      400 {object}  ErrorResponse
// @Router       /login [POST]
func login(writer http.ResponseWriter, request *http.Request, config configuration.Config, connector *platform_connector_lib.Connector, connectionLimit *connectionlimit.ConnectionLimitHandler, logger *slog.Logger) {
	defer func() {
		if p := recover(); p != nil {
			if config.Debug {
				debug.PrintStack()
			}
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

	authenticationMethod := config.MqttAuthMethod

	if authenticationMethod == "certificate" {
		logger.Info("login", "action", "login", "peerAddr", msg.PeerAddr, "loginType", "cert", "clientId", msg.ClientId, "cleanStart", msg.CleanStart, "cleanSession", msg.CleanSession)
	} else {
		logger.Info("login", "action", "login", "peerAddr", msg.PeerAddr, "loginType", "pw", "username", msg.Username, "clientId", msg.ClientId, "cleanStart", msg.CleanStart, "cleanSession", msg.CleanSession)
	}

	//log.Println("DEBUG: /login", msg)
	if msg.Username != config.AuthClientId {
		if (!msg.CleanSession && !msg.CleanStart) && config.ForceCleanSession && !contains(config.CleanSessionAllowUserList, msg.Username) {
			sendError(writer, "expect clean session", config.Debug)
			return
		}

		var token security.JwtToken
		var err error

		if authenticationMethod == "password" {
			token, err = connector.Security().GetUserToken(msg.Username, msg.Password, model.RemoteInfo{
				Ip:       msg.PeerAddr,
				Port:     strconv.Itoa(msg.PeerPort),
				Protocol: config.SecRemoteProtocol,
			})
		} else if authenticationMethod == "certificate" {
			// The user is already authenticated by the TLS client certificate validation in the broker
			token, err = connector.Security().ExchangeUserToken(msg.Username, model.RemoteInfo{
				Ip:       msg.PeerAddr,
				Port:     strconv.Itoa(msg.PeerPort),
				Protocol: config.SecRemoteProtocol,
			})
		}

		if err != nil {
			sendError(writer, err.Error(), config.Debug)
			return
		}
		if token == "" {
			sendError(writer, "access denied", config.Debug)
			return
		}
		err = connectionLimit.Check(msg.ClientId)
		if err != nil {
			sendError(writer, err.Error(), config.Debug)
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
}
