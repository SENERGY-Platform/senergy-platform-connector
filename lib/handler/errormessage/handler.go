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
	"errors"
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/correlation"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/metrics"
	"github.com/SENERGY-Platform/service-commons/pkg/jwt"
	"log"
	"strings"
)

func New(config configuration.Config, connector *platform_connector_lib.Connector, correlation *correlation.CorrelationService, metrics *metrics.Metrics) *Handler {
	return &Handler{
		config:      config,
		connector:   connector,
		correlation: correlation,
		metrics:     metrics,
	}
}

type Handler struct {
	connector   *platform_connector_lib.Connector
	correlation *correlation.CorrelationService
	metrics     *metrics.Metrics
	config      configuration.Config
}

func (this *Handler) Subscribe(clientId string, user string, topic string) (result handler.Result, err error) {
	return handler.Unhandled, nil
}

func (this *Handler) Publish(clientId string, user string, topic string, payload []byte, qos int, size float64) (result handler.Result, err error) {
	if !strings.HasPrefix(topic, "error") {
		return handler.Unhandled, nil
	}
	topicParts := strings.Split(topic, "/")
	if topicParts[0] != "error" {
		return handler.Unhandled, nil
	}

	this.metrics.ClientErrorMessages.Inc()

	token, err := this.connector.Security().GetCachedUserToken(user, model.RemoteInfo{})
	if err != nil {
		log.Println("ERROR: unable to get user token", err)
		return
	}

	parsedToken, err := jwt.Parse(string(token))
	if err != nil {
		return handler.Error, err
	}
	userId := parsedToken.GetUserId()

	if len(topicParts) >= 4 && userId != topicParts[2] {
		return handler.Rejected, errors.New("mismatch between client user and owner in topic")
	}
	switch {
	case len(topicParts) == 1:
		this.handleGeneralError(userId, clientId, payload)
		return handler.Accepted, nil
	case len(topicParts) == 3 && topicParts[1] == "device":
		//error/device/{local_device_id}
		if this.config.ForceTopicsWithOwner {
			return handler.Rejected, errors.New("expect owner in topic")
		}
		this.handleDeviceError(token, userId, topicParts[2], payload)
		return handler.Accepted, nil
	case len(topicParts) == 3 && topicParts[1] == "command":
		//error/command/{correlation_id}
		if this.config.ForceTopicsWithOwner {
			return handler.Rejected, errors.New("expect owner in topic")
		}
		this.handleCommandError(userId, topicParts[2], payload)
		return handler.Accepted, nil
	case len(topicParts) == 4 && topicParts[1] == "device":
		//error/device/{owner_id}/{local_device_id}
		this.handleDeviceError(token, userId, topicParts[3], payload)
		return handler.Accepted, nil
	case len(topicParts) == 4 && topicParts[1] == "command":
		//error/command/{owner_id}/{correlation_id}
		this.handleCommandError(userId, topicParts[3], payload)
		return handler.Accepted, nil
	}
	return handler.Rejected, errors.New("unknown error topic")
}

func (this *Handler) handleGeneralError(user string, clientId string, payload []byte) {
	userId, err := this.connector.Security().GetUserId(user)
	if err != nil {
		log.Println("ERROR: unable to get user id", err)
		return
	}
	this.connector.HandleClientError(userId, clientId, string(payload))
}

func (this *Handler) handleDeviceError(token security.JwtToken, userId string, deviceId string, payload []byte) {
	device, err := this.connector.IotCache.WithToken(token).GetDeviceByLocalId(deviceId)
	if err != nil {
		log.Println("ERROR: unable to get user device", err)
		return
	}
	this.connector.HandleDeviceError(userId, device, string(payload))
}

func (this *Handler) handleCommandError(userId string, correlationId string, payload []byte) {
	protocolMsg, err := this.correlation.Get(correlationId)
	if err != nil {
		log.Println("ERROR: unable to use correlationId", err)
		return
	}
	this.connector.HandleCommandError(userId, protocolMsg, string(payload))
}
