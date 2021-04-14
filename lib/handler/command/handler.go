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

package command

import (
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"log"
	"strings"
)

func New(config configuration.Config, connector *platform_connector_lib.Connector, logger connectionlog.Logger) *Handler {
	return &Handler{
		config:    config,
		connector: connector,
		logger:    logger,
	}
}

type Handler struct {
	config    configuration.Config
	connector *platform_connector_lib.Connector
	logger    connectionlog.Logger
}

func (this *Handler) Subscribe(clientId string, user string, topic string) (result handler.Result, err error) {
	if !strings.HasPrefix(topic, "command") {
		return handler.Unhandled, nil
	}
	prefix, deviceUri, _, err := handler.ParseTopic(topic)
	if err != nil {
		return handler.Rejected, err
	}
	if prefix != "command" {
		//may happen if topic is something like "commandhandling/foo/bar"
		log.Println("WARNING: handler.ParseTopic() returned '"+prefix+"' while the topic string prefix is command:", topic)
		return handler.Unhandled, nil
	}
	token, err := this.connector.Security().GetCachedUserToken(user)
	if err != nil {
		return handler.Error, err
	}
	if this.config.CheckHub {
		err := handler.CheckHub(this.connector, token, clientId, deviceUri)
		if err != nil {
			return handler.Rejected, err
		}
	}
	device, err := this.connector.IotCache.WithToken(token).GetDeviceByLocalId(deviceUri)
	if err != nil {
		if this.config.Debug {
			log.Println("WARNING: InitWebhooks::subscribe::DeviceUrlToIotDevice", err)
		}
		return handler.Rejected, err
	}
	err = this.logger.LogDeviceConnect(device.Id)
	if err != nil {
		if this.config.Debug {
			log.Println("ERROR: InitWebhooks::subscribe::CheckEndpointAuth", err)
		}
	}
	return handler.Accepted, nil
}

func (this *Handler) Publish(clientId string, user string, topic string, payload []byte) (result handler.Result, err error) {
	return handler.Unhandled, nil
}
