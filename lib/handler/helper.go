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

package handler

import (
	"errors"
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"log"
	"strings"
)

func ParseTopic(topic string) (prefix string, deviceUri string, serviceUri string, err error) {
	parts := strings.Split(topic, "/")
	if len(parts) != 3 {
		err = errors.New("expect 3-part topic with prefix/device-uri/service-uri")
		return
	}
	return parts[0], parts[1], parts[2], nil
}

func CheckHub(connector *platform_connector_lib.Connector, token security.JwtToken, hubId string, deviceUri string) (err error) {
	hub, err := connector.Iot().GetHub(hubId, token)
	if err != nil {
		return err
	}
	for _, device := range hub.DeviceLocalIds {
		if device == deviceUri {
			return nil
		}
	}
	return errors.New("device is not assigned to hub")
}

func CheckEvent(connector *platform_connector_lib.Connector, token security.JwtToken, deviceUri string, serviceUri string) (err error) {
	device, err := connector.IotCache.WithToken(token).GetDeviceByLocalId(deviceUri)
	if err != nil {
		return err
	}
	dt, err := connector.IotCache.WithToken(token).GetDeviceType(device.DeviceTypeId)
	if err != nil {
		return err
	}
	for _, service := range dt.Services {
		if service.LocalId == serviceUri {
			return nil
		}
	}
	return ServiceNotFound
}

var ServiceNotFound = errors.New("service not found")

func HandleTopicSubscribe(clientId string, username string, topic string, handlers []Handler) (Result, error) {
	for _, h := range handlers {
		handlerResult, err := h.Subscribe(clientId, username, topic)
		if err != nil {
			return Error, err
		}
		switch handlerResult {
		case Accepted, Rejected, Error:
			return handlerResult, err
		case Unhandled:
			continue
		default:
			log.Println("WARNING: unknown handler result", handlerResult)
			continue
		}
	}
	log.Println("WARNING: no matching topic handler found", topic)
	return Rejected, errors.New("no matching topic handler found")
}
