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
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlimit"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/service-commons/pkg/jwt"
)

// login godoc
// @Summary      login webhook
// @Description  checks auth; logins on the token login mountpoint authenticate with a keycloak access token as password and are answered with TokenLoginResponse; all responses are with code=200, differences in swagger doc are because of technical incompatibilities of the documentation format
// @Accept       json
// @Produce      json
// @Param        message body LoginWebhookMsg true "login infos"
// @Success      200 {object}  OkResponse
// @Success      201 {object}  TokenLoginResponse
// @Failure      400 {object}  ErrorResponse
// @Router       /login [POST]
func login(writer http.ResponseWriter, request *http.Request, config configuration.Config, connector *platform_connector_lib.Connector, connectionLimit *connectionlimit.ConnectionLimitHandler, tokenCerts *jwt.KeycloakCertProvider, logger *slog.Logger) {
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

	if config.TokenLoginMountpoint != "" && msg.Mountpoint == config.TokenLoginMountpoint {
		logger.Info("login", "action", "login", "peerAddr", msg.PeerAddr, "loginType", "token", "mountpoint", msg.Mountpoint, "username", msg.Username, "clientId", msg.ClientId, "cleanStart", msg.CleanStart, "cleanSession", msg.CleanSession)
		tokenLogin(writer, msg, config, connectionLimit, tokenCerts, logger)
		return
	}

	if authenticationMethod == "certificate" {
		logger.Info("login", "action", "login", "peerAddr", msg.PeerAddr, "loginType", "cert", "clientId", msg.ClientId, "cleanStart", msg.CleanStart, "cleanSession", msg.CleanSession)
	} else {
		logger.Info("login", "action", "login", "peerAddr", msg.PeerAddr, "loginType", "pw", "username", msg.Username, "clientId", msg.ClientId, "cleanStart", msg.CleanStart, "cleanSession", msg.CleanSession)
	}

	// every mountpoint other than the token login mountpoint is unknown; without this a login on an additional
	// listener would be treated as a certificate login and trust the username
	if msg.Mountpoint != "" {
		logger.Warn("refuse login with unknown mountpoint", "action", "login", "peerAddr", msg.PeerAddr, "mountpoint", msg.Mountpoint, "clientId", msg.ClientId)
		sendError(writer, "access denied", config.Debug)
		return
	}
	// the client id prefix of token sessions is exclusive to them, publish and subscribe rely on it
	if isTokenSessionClientId(msg.ClientId) {
		logger.Warn("refuse login with reserved client id prefix", "action", "login", "peerAddr", msg.PeerAddr, "clientId", msg.ClientId)
		sendError(writer, "access denied", config.Debug)
		return
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
	if err != nil {
		config.GetLogger().Debug("ERROR: InitWebhooks::login unable to fprint", "error", err)
	}
}

// tokenLogin handles the login on the token login mountpoint: the password is a Keycloak access token of the user
// named by the username. The session is moved to the default mountpoint, so it sees the messages of the
// devices, and its client id is renamed to web:<exp>:<original client id>, which the publish and subscribe
// webhooks restrict and the token session sweeper uses to end the session when the token expires.
func tokenLogin(writer http.ResponseWriter, msg LoginWebhookMsg, config configuration.Config, connectionLimit *connectionlimit.ConnectionLimitHandler, tokenCerts *jwt.KeycloakCertProvider, logger *slog.Logger) {
	deny := func(reason string, err error) {
		// neither token nor password are logged
		logger.Warn("refuse token login", "action", "login", "peerAddr", msg.PeerAddr, "username", msg.Username, "clientId", msg.ClientId, "reason", reason, "error", err)
		sendError(writer, "access denied", config.Debug)
	}
	if tokenCerts == nil || tokenCerts.CertUrl == "" {
		deny("token login is not configured: missing KeycloakCertUrl", nil)
		return
	}
	if msg.Password == "" {
		deny("missing password", nil)
		return
	}
	if msg.Username == "" {
		deny("missing username", nil)
		return
	}
	// always, regardless of ForceCleanSession: a persistent token session would keep its subscriptions and queue
	// events while offline, past the expiry of its token
	if !msg.CleanSession && !msg.CleanStart {
		sendError(writer, "expect clean session", config.Debug)
		return
	}
	rawToken := msg.Password
	if len(rawToken) > 7 && strings.EqualFold(rawToken[:7], "bearer ") {
		rawToken = rawToken[7:]
	}
	claims, err := jwt.ParseWithValidation(tokenCerts, rawToken)
	if err != nil {
		deny("invalid token", err)
		return
	}
	if claims.Sub == "" || claims.Sub != msg.Username {
		deny("token subject does not match username", nil)
		return
	}
	exp, err := accessTokenExpiry(rawToken)
	if err != nil {
		deny("invalid access token", err)
		return
	}
	if !exp.After(time.Now()) {
		deny("token is expired", nil)
		return
	}
	err = connectionLimit.Check(msg.ClientId)
	if err != nil {
		sendError(writer, err.Error(), config.Debug)
		return
	}
	// a browser is no hub: no CheckHub
	err = json.NewEncoder(writer).Encode(TokenLoginResponse{
		Result: "ok",
		Modifiers: TokenLoginModifiers{
			Mountpoint: "",
			ClientId:   tokenSessionClientId(exp, msg.ClientId),
		},
	})
	if err != nil {
		logger.Debug("ERROR: InitWebhooks::login unable to encode token login response", "error", err)
	}
}
