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

package client

import (
	"github.com/SENERGY-Platform/iot-device-repository/lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/iot"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"hash/fnv"
	"log"
	"sort"
	"strconv"
	"strings"
)

func getHash(representations []DeviceRepresentation) string {
	deviceUris := []string{}
	for _, device := range representations {
		deviceUris = append(deviceUris, device.Uri)
	}
	sort.Strings(deviceUris)
	h := fnv.New32a()
	h.Write([]byte(strings.Join(deviceUris, ";")))
	return strconv.FormatUint(uint64(h.Sum32()), 10)
}

func (this *Client) provisionHub(token security.JwtToken) (isNew bool, err error) {
	iotClient := iot.New(this.semanticRepoUrl, this.deviceRepoUrl, "")
	hash := getHash(this.devices)
	exists := false
	deviceUris := []string{}
	for _, device := range this.devices {
		deviceUris = append(deviceUris, device.Uri)
	}
	if this.HubId != "" {
		exists, err = iotClient.ExistsHub(this.HubId, token)
		if err != nil {
			log.Println("ERROR: iotClient.ExistsHub()", err)
			return isNew, err
		}
	}
	isNew = !exists
	if exists {
		oldHash, err := iotClient.GetHubHash(this.HubId, token)
		if err != nil {
			log.Println("ERROR: iotClient.GetHubHash()", err)
			return isNew, err
		}
		if oldHash != hash {
			_, err = iotClient.UpdateHub(this.HubId, model.Hub{Hash: hash, Name: this.hubName, Devices: deviceUris}, token)
		}
		if err != nil {
			log.Println("ERROR: iotClient.UpdateHub()", err)
		}
		return isNew, err
	} else {
		result, err := iotClient.CreateHub(model.Hub{Hash: hash, Name: this.hubName, Devices: deviceUris}, token)
		if err != nil {
			log.Println("ERROR: iotClient.CreateHub()", err)
			return isNew, err
		}
		this.HubId = result.Id
		return isNew, nil
	}
}

func (this *Client) provisionDevices(token security.JwtToken) (newDevices bool, err error) {
	iotClient := iot.New(this.semanticRepoUrl, this.deviceRepoUrl, "")
	for _, device := range this.devices {
		_, err := iotClient.DeviceUrlToIotDevice(device.Uri, token)
		if err != nil && err != security.ErrorNotFound {
			log.Println("ERROR: iotClient.DeviceUrlToIotDevice()", err)
			return false, err
		}
		if err == security.ErrorNotFound {
			_, err = iotClient.CreateIotDevice(device, token)
			if err != nil {
				log.Println("ERROR: iotClient.CreateIotDevice()", err)
				return false, err
			}
			newDevices = true
		}
	}
	return newDevices, nil
}
