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

package errormessage

import (
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/correlation"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"log"
	"strings"
)

func New(connector *platform_connector_lib.Connector, correlation *correlation.CorrelationService) *Handler {
	return &Handler{
		connector:   connector,
		correlation: correlation,
	}
}

type Handler struct {
	connector   *platform_connector_lib.Connector
	correlation *correlation.CorrelationService
}

func (this *Handler) Subscribe(clientId string, user string, topic string) (result handler.Result, err error) {
	return handler.Unhandled, nil
}

func (this *Handler) Publish(clientId string, user string, topic string, payload []byte, qos int) (result handler.Result, err error) {
	if !strings.HasPrefix(topic, "error") {
		return handler.Unhandled, nil
	}
	topicParts := strings.Split(topic, "/")
	if topicParts[0] != "error" {
		return handler.Unhandled, nil
	}
	switch {
	case len(topicParts) == 1:
		this.handleGeneralError(user, clientId, payload)
	case len(topicParts) == 3 && topicParts[1] == "device":
		this.handleDeviceError(user, topicParts[2], payload)
	case len(topicParts) == 3 && topicParts[1] == "command":
		this.handleCommandError(user, topicParts[2], payload)
	}
	return handler.Accepted, nil
}

func (this *Handler) handleGeneralError(user string, clientId string, payload []byte) {
	userId, err := this.connector.Security().GetUserId(user)
	if err != nil {
		log.Println("ERROR: unable to get user id", err)
		return
	}
	this.connector.HandleClientError(userId, clientId, string(payload))
}

func (this *Handler) handleDeviceError(user string, deviceId string, payload []byte) {
	userId, err := this.connector.Security().GetUserId(user)
	if err != nil {
		log.Println("ERROR: unable to get user id", err)
		return
	}
	token, err := this.connector.Security().GetCachedUserToken(user)
	if err != nil {
		log.Println("ERROR: unable to get user token", err)
		return
	}
	device, err := this.connector.IotCache.WithToken(token).GetDeviceByLocalId(deviceId)
	if err != nil {
		log.Println("ERROR: unable to get user device", err)
		return
	}
	this.connector.HandleDeviceError(userId, device, string(payload))
}

func (this *Handler) handleCommandError(user string, correlationId string, payload []byte) {
	userId, err := this.connector.Security().GetUserId(user)
	if err != nil {
		log.Println("ERROR: unable to get user id", err)
		return
	}
	protocolMsg, err := this.correlation.Get(correlationId)
	if err != nil {
		log.Println("ERROR: unable to use correlationId", err)
		return
	}
	this.connector.HandleCommandError(userId, protocolMsg, string(payload))
}
