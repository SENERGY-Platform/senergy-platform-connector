/*
 * Copyright 2021 InfAI (CC SES)
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

package process

import (
	"errors"
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"strings"
)

func New(connector *platform_connector_lib.Connector) *Handler {
	return &Handler{connector: connector}
}

type Handler struct {
	connector *platform_connector_lib.Connector
}

func (this *Handler) Subscribe(clientId string, user string, topic string) (result handler.Result, err error) {
	return this.checkTopicAccess(clientId, user, topic)
}

func (this *Handler) Publish(clientId string, user string, topic string, payload []byte, qos int, size float64) (result handler.Result, err error) {
	return this.checkTopicAccess(clientId, user, topic)
}

func (this *Handler) checkTopicAccess(clientId string, user string, topic string) (result handler.Result, err error) {
	if !strings.HasPrefix(topic, "processes/") {
		return handler.Unhandled, nil
	}
	token, err := this.connector.Security().GetCachedUserToken(user, model.RemoteInfo{})
	if err != nil {
		return handler.Error, err
	}
	admin, err := isAdmin(token)
	if err != nil {
		return handler.Error, err
	}
	if admin {
		return handler.Accepted, nil
	}
	hubId, err := parseTopic(topic)
	if err != nil {
		return handler.Rejected, err
	}

	if hubId == "+" || hubId == "#" {
		return handler.Rejected, NonAdminRequestToPlaceholderTopicError //admins may access these topic but those are already accepted for everything
	}
	if clientId != hubId {
		exists, err := this.connector.Iot().ExistsHub(hubId, token) //existence check implies access check
		if err != nil {
			return handler.Error, err
		}
		if !exists {
			return handler.Rejected, HubAccessDeniedError
		}
	}

	return handler.Accepted, nil
}

var MissingHubIdInTopicError = errors.New("missing hub id in topic")
var NonAdminRequestToPlaceholderTopicError = errors.New("non admin request to placeholder topic of processes")
var HubAccessDeniedError = errors.New("access to hub denied")

func parseTopic(topic string) (hubId string, err error) {
	parts := strings.Split(topic, "/")
	if len(parts) < 2 {
		return "", MissingHubIdInTopicError
	}
	return parts[1], nil
}

func isAdmin(token security.JwtToken) (bool, error) {
	jwt, err := token.GetPayload()
	if err != nil {
		return false, err
	}
	for _, role := range jwt.RealmAccess.Roles {
		if role == "admin" {
			return true, nil
		}
	}
	return false, nil
}
