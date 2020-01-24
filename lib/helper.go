/*
 * Copyright 2019 InfAI (CC SES)
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

package lib

import (
	"errors"
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"strings"
)

func parseTopic(topic string) (prefix string, deviceUri string, serviceUri string, err error) {
	parts := strings.Split(topic, "/")
	if len(parts) != 3 {
		err = errors.New("expect 3-part topic with prefix/device-uri/service-uri")
		return
	}
	return parts[0], parts[1], parts[2], nil
}

func checkHub(connector *platform_connector_lib.Connector, token security.JwtToken, hubId string, deviceUri string) (err error) {
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

var ServiceNotFound = errors.New("service not found")

func checkEvent(connector *platform_connector_lib.Connector, token security.JwtToken, deviceUri string, serviceUri string) (err error) {
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

func StringToList(str string) []string {
	temp := strings.Split(str, ",")
	result := []string{}
	for _, e := range temp {
		trimmed := strings.TrimSpace(e)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
