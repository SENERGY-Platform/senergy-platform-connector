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
	"github.com/SENERGY-Platform/platform-connector-lib/iot"
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

func userMayAccessDevice(iot *iot.Iot, token security.JwtToken, deviceUri string, serviceUri string) (access bool, deviceId string, err error) {
	entities, err := iot.DeviceUrlToIotDevice(deviceUri, token)
	if err != nil {
		return false, deviceId, err
	}
	for _, entity := range entities {
		//single level mqtt-topic wildcard
		if serviceUri == "+" {
			return true, entity.Device.Id, nil
		}
		for _, service := range entity.Services {
			if service.Url == serviceUri {
				return true, entity.Device.Id, nil
			}
		}
	}
	return false, "", nil
}

func checkHub(connector *platform_connector_lib.Connector, token security.JwtToken, hubId string, deviceUri string) (err error) {
	devices, err := connector.Iot().GetHubDevices(hubId, token)
	if err != nil {
		return err
	}
	for _, device := range devices {
		if device == deviceUri {
			return nil
		}
	}
	return errors.New("device is not assigned to hub")
}

func checkEvent(connector *platform_connector_lib.Connector, token security.JwtToken, deviceUri string, serviceUri string) (err error) {
	devices, err := connector.IotCache.WithToken(token).DeviceUrlToIotDevice(deviceUri)
	if err != nil {
		return err
	}
	for _, device := range devices {
		for _, service := range device.Services {
			if service.Url == serviceUri {
				return nil
			}
		}
	}
	return errors.New("not found")
}
