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
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	paho5 "github.com/eclipse/paho.golang/paho"
	paho "github.com/eclipse/paho.mqtt.golang"
)

// values are matching to test server produced by /senergy-platform-connector/test/server/server.go
var Id = "connector"
var Secret = "d61daec4-40d6-4d3e-98c9-f3b515696fc6"

func New(mqttUrl string, deviceManagerUrl string, deviceRepoUrl string, authUrl string, userName string, password string, hubId string, hubName string, devices []DeviceRepresentation, authenticationMethod string, mqttVersion MqttVersion, ownerInTopic bool) (client *Client, err error) {
	client = &Client{
		ownerInTopic:         ownerInTopic,
		authUrl:              authUrl,
		mqttUrl:              mqttUrl,
		deviceManagerUrl:     deviceManagerUrl,
		deviceRepoUrl:        deviceRepoUrl,
		HubId:                hubId,
		hubName:              hubName,
		userid:               userName,
		username:             userName,
		password:             password,
		clientId:             Id,
		clientSecret:         Secret,
		devices:              devices,
		subscriptions:        map[string]Subscription{},
		authenticationMethod: authenticationMethod,
		mqttVersion:          mqttVersion,
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

func NewWithoutProvisioning(mqttUrl string, deviceManagerUrl string, deviceRepoUrl string, authUrl string, userName string, password string, hubId string, hubName string, devices []DeviceRepresentation, mqttVersion MqttVersion, ownerInTopic bool) (client *Client, err error) {
	client = &Client{
		ownerInTopic:     ownerInTopic,
		authUrl:          authUrl,
		mqttUrl:          mqttUrl,
		deviceManagerUrl: deviceManagerUrl,
		deviceRepoUrl:    deviceRepoUrl,
		HubId:            hubId,
		hubName:          hubName,
		username:         userName,
		password:         password,
		clientId:         Id,
		clientSecret:     Secret,
		devices:          devices,
		subscriptions:    map[string]Subscription{},
		mqttVersion:      mqttVersion,
	}
	err = client.startMqtt()
	return
}

type Client struct {
	ownerInTopic bool

	mqttVersion      MqttVersion
	mqttUrl          string
	deviceRepoUrl    string
	deviceManagerUrl string
	authUrl          string
	HubId            string

	userid   string
	username string
	password string

	mqtt         paho.Client
	mqtt5        *autopaho.ConnectionManager
	mqtt5router  paho5.Router
	clientId     string
	clientSecret string
	devices      []DeviceRepresentation
	hubName      string

	subscriptionsMux sync.Mutex
	subscriptions    map[string]Subscription

	authenticationMethod string
}

func (this *Client) Stop() {
	if this.mqttVersion == MQTT4 {
		this.mqtt.Disconnect(0)
	}
	if this.mqttVersion == MQTT5 {
		disconnecttimeout, _ := context.WithTimeout(context.Background(), 10*time.Second)
		this.mqtt5.Disconnect(disconnecttimeout)
	}
}

func (this *Client) Subscribe(topic string, qos byte, callback func(topic string, pl []byte)) error {
	if this.mqttVersion == MQTT4 {
		return this.SubscribeMqtt4(topic, qos, callback)
	}
	if this.mqttVersion == MQTT5 {
		return this.SubscribeMqtt5(topic, qos, callback)
	}
	return errors.New("unknown mqtt version")
}

func (this *Client) Mqtt() paho.Client {
	return this.mqtt
}

type DeviceRepresentation struct {
	IotType string `json:"iot_type"`
	Uri     string `json:"uri"`
	Name    string `json:"name"`
}
