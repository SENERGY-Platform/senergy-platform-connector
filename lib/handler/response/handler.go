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

package response

import (
	"encoding/json"
	"errors"
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/correlation"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"github.com/SENERGY-Platform/service-commons/pkg/jwt"
	"log"
	"strings"
)

func New(config configuration.Config, connector *platform_connector_lib.Connector, correlation *correlation.CorrelationService) *Handler {
	return &Handler{
		config:      config,
		connector:   connector,
		correlation: correlation,
	}
}

type Handler struct {
	config      configuration.Config
	connector   *platform_connector_lib.Connector
	correlation *correlation.CorrelationService
}

func (this *Handler) Subscribe(clientId string, user string, topic string) (result handler.Result, err error) {
	return handler.Unhandled, nil
}

func (this *Handler) Publish(clientId string, user string, topic string, payload []byte, qos int, size float64) (result handler.Result, err error) {
	if !strings.HasPrefix(topic, "response") {
		return handler.Unhandled, nil
	}
	var prefix, owner, deviceUri string
	if this.config.TopicsWithOwner {
		prefix, owner, deviceUri, _, err = handler.ParseTopicWithOwner(topic)
	} else {
		prefix, deviceUri, _, err = handler.ParseTopic(topic)
	}
	if err != nil {
		return handler.Error, err
	}
	if prefix != "response" {
		//may happen if topic is something like "responsehandling/foo/bar"
		log.Println("WARNING: handler.ParseTopic() returned '"+prefix+"' while the topic string prefix is response:", topic)
		return handler.Unhandled, nil
	}

	token, err := this.connector.Security().GetCachedUserToken(user, model.RemoteInfo{})
	if err != nil {
		return handler.Error, err
	}

	if this.config.TopicsWithOwner {
		parsedToken, err := jwt.Parse(string(token))
		if err != nil {
			return handler.Error, err
		}
		if !parsedToken.IsAdmin() && parsedToken.GetUserId() != owner {
			return handler.Rejected, errors.New("mismatch between client user and owner in topic")
		}
	}

	if this.config.CheckHub {
		err := handler.CheckHub(this.connector, token, clientId, deviceUri)
		if err != nil {
			return handler.Error, err
		}
	}
	if !this.config.MqttPublishAuthOnly {
		msg := ResponseEnvelope{}
		err = json.Unmarshal(payload, &msg)
		if err != nil {
			return handler.Error, err
		}
		request, err := this.correlation.Get(msg.CorrelationId)
		if err != nil {
			log.Println("ERROR: InitWebhooks::publish::response::correlation.Get", err)
			return handler.Accepted, nil //potentially old message; may be ignored; but dont cut connection
		}
		request.Trace = append(request.Trace, msg.Trace...) // merge traces
		err = this.connector.HandleCommandResponse(request, msg.Payload, platform_connector_lib.Qos(qos))
		if err != nil {
			return handler.Error, err
		}
	}
	return handler.Accepted, nil
}
