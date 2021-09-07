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

package event

import (
	"encoding/json"
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"log"
	"strings"
)

func New(config configuration.Config, connector *platform_connector_lib.Connector) *Handler {
	return &Handler{
		connector: connector,
		config:    config,
	}
}

type Handler struct {
	connector *platform_connector_lib.Connector
	config    configuration.Config
}

func (this *Handler) Subscribe(clientId string, user string, topic string) (result handler.Result, err error) {
	return handler.Unhandled, nil
}

func (this *Handler) Publish(clientId string, user string, topic string, payload []byte, qos int) (result handler.Result, err error) {
	if !strings.HasPrefix(topic, "event") {
		return handler.Unhandled, nil
	}
	prefix, deviceUri, serviceUri, err := handler.ParseTopic(topic)
	if err != nil {
		return handler.Rejected, err
	}
	if prefix != "event" {
		//may happen if topic is something like "eventhandling/foo/bar"
		log.Println("WARNING: handler.ParseTopic() returned '"+prefix+"' while the topic string prefix is event:", topic)
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
	event := platform_connector_lib.EventMsg{}
	err = json.Unmarshal(payload, &event)
	if err != nil {
		return handler.Error, err
	}
	if !this.config.CheckHub {
		if err := handler.CheckEvent(this.connector, token, deviceUri, serviceUri); err != nil {
			if err == handler.ServiceNotFound {
				if this.config.Debug {
					log.Println("DEBUG: got event for unknown service of known device", deviceUri, serviceUri)
				}
				return handler.Accepted, nil
			} else {
				return handler.Rejected, err
			}
		}
	}
	if !this.config.MqttPublishAuthOnly {
		err = this.connector.HandleDeviceRefEventWithAuthToken(token, deviceUri, serviceUri, event, platform_connector_lib.Qos(qos))
		if err != nil {
			return handler.Error, err
		}
	}
	return handler.Accepted, nil
}
