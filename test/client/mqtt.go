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
	"encoding/json"
	"errors"
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler/response"
	paho "github.com/eclipse/paho.mqtt.golang"
	"log"
	"time"
)

type MqttVersion int

const (
	MQTT4 MqttVersion = iota
	MQTT5
)

func (this *Client) startMqtt() error {
	if this.mqttVersion == MQTT4 {
		return this.startMqtt4()
	}
	if this.mqttVersion == MQTT5 {
		return this.startMqtt5()
	}
	return errors.New("unknown mqtt version")
}

func (this *Client) SendDeviceError(deviceUri string, message string) error {
	return this.Publish("error/device/"+deviceUri, message, 2)
}

func (this *Client) SendClientError(message string) error {
	return this.Publish("error", message, 2)
}

func (this *Client) ListenCommand(deviceUri string, serviceUri string, handler func(msg platform_connector_lib.CommandRequestMsg) (platform_connector_lib.CommandResponseMsg, error)) error {
	return this.ListenCommandWithQos(deviceUri, serviceUri, 2, handler)
}

func (this *Client) ListenCommandWithQos(deviceUri string, serviceUri string, qos byte, handler func(msg platform_connector_lib.CommandRequestMsg) (platform_connector_lib.CommandResponseMsg, error)) error {
	topic := "command/" + deviceUri + "/" + serviceUri
	callback := func(topic string, pl []byte) {
		request := lib.RequestEnvelope{}
		err := json.Unmarshal(pl, &request)
		if err != nil {
			log.Println("ERROR: unable to decode request envalope", err)
			return
		}
		if time.Since(time.Unix(request.Time, 0)) > 40*time.Second {
			log.Println("WARNING: received old command; do nothing")
			return
		}
		//log.Println("DEBUG: client handle command", request)
		respMsg, err := handler(request.Payload)
		if err != nil {
			log.Println("ERROR: while processing command:", err)
			log.Println(this.Publish("error/command/"+request.CorrelationId, err.Error(), 2))
			return
		}
		go func() {
			err = this.Publish("response/"+deviceUri+"/"+serviceUri, response.ResponseEnvelope{CorrelationId: request.CorrelationId, Payload: respMsg}, qos)
			if err != nil {
				log.Println("ERROR: unable to Publish response", err)
			}
		}()
	}
	return this.Subscribe(topic, qos, callback)
}

func (this *Client) Unsubscribe(deviceUri string, serviceUri string) (err error) {
	if this.mqttVersion == MQTT4 {
		return this.UnsubscribeMqtt4(deviceUri, serviceUri)
	}
	if this.mqttVersion == MQTT5 {
		return this.UnsubscribeMqtt5(deviceUri, serviceUri)
	}
	return errors.New("unknown mqtt version")
}

func (this *Client) SendEvent(deviceUri string, serviceUri string, msg platform_connector_lib.EventMsg) (err error) {
	return this.Publish("event/"+deviceUri+"/"+serviceUri, msg, 2)
}

func (this *Client) SendEventWithQos(deviceUri string, serviceUri string, msg platform_connector_lib.EventMsg, qos byte) (err error) {
	return this.Publish("event/"+deviceUri+"/"+serviceUri, msg, qos)
}

func (this *Client) Publish(topic string, msg interface{}, qos byte) (err error) {
	log.Println("DEBUG: publish", topic, msg)
	if this.mqttVersion == MQTT4 {
		return this.PublishMqtt4(topic, msg, qos)
	}
	if this.mqttVersion == MQTT5 {
		return this.PublishMqtt5(topic, msg, qos)
	}
	return errors.New("unknown mqtt version")
}

type Subscription struct {
	Topic   string
	Handler paho.MessageHandler
	Qos     byte
}

func (this *Client) registerSubscription(topic string, qos byte, handler paho.MessageHandler) {
	this.subscriptionsMux.Lock()
	defer this.subscriptionsMux.Unlock()
	this.subscriptions[topic] = Subscription{
		Topic:   topic,
		Handler: handler,
		Qos:     qos,
	}
}

func (this *Client) unregisterSubscriptions(topic string) {
	this.subscriptionsMux.Lock()
	defer this.subscriptionsMux.Unlock()
	delete(this.subscriptions, topic)
}

func (this *Client) getSubscriptions() (result []Subscription) {
	this.subscriptionsMux.Lock()
	defer this.subscriptionsMux.Unlock()
	for _, sub := range this.subscriptions {
		result = append(result, sub)
	}
	return
}

func (this *Client) loadOldSubscriptions() error {
	if this.mqttVersion == MQTT4 {
		return this.loadOldSubscriptionsMqtt4()
	}
	if this.mqttVersion == MQTT5 {
		return this.loadOldSubscriptionsMqtt5()
	}
	return errors.New("unknown mqtt version")
}
