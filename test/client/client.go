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
	paho "github.com/eclipse/paho.mqtt.golang"
	"log"
	"time"
)

//values are matching to test server produced by /senergy-platform-connector/test/server/server.go
var Id = "connector"
var Secret = "d61daec4-40d6-4d3e-98c9-f3b515696fc6"

func New(mqttUrl string, provisioningUrl string, authUrl string, userName string, password string, hubId string, hubName string, devices []DeviceRepresentation) (client *Client, err error) {
	client = &Client{
		authUrl:         authUrl,
		mqttUrl:         mqttUrl,
		provisioningUrl: provisioningUrl,
		HubId:           hubId,
		hubName:         hubName,
		username:        userName,
		password:        password,
		clientId:        Id,
		clientSecret:    Secret,
		devices:         devices,
	}
	token, err := client.login()
	if err != nil {
		log.Println("ERROR: unable to login", err)
		return client, err
	}
	newDevices, err := client.provisionDevices(token.JwtToken())
	if err != nil {
		log.Println("ERROR: unable to provision device", err)
		return client, err
	}
	if newDevices {
		time.Sleep(2 * time.Second) //wait for device creation
	}
	newHub, err := client.provisionHub(token.JwtToken())
	if err != nil {
		log.Println("ERROR: unable to provisionHub", err)
		return client, err
	}
	if newHub {
		time.Sleep(2 * time.Second) //wait for hub creation
	}
	err = client.startMqtt()
	return
}

type Client struct {
	mqttUrl         string
	provisioningUrl string
	authUrl         string
	HubId           string

	username string
	password string

	mqtt         paho.Client
	clientId     string
	clientSecret string
	devices      []DeviceRepresentation
	hubName      string
}

func (this *Client) Stop() {
	this.mqtt.Disconnect(0)
}

func (this *Client) Mqtt() paho.Client {
	return this.mqtt
}

type DeviceRepresentation = model.ProvisioningDevice
