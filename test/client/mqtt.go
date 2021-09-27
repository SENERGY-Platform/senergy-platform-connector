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
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler/response"
	paho "github.com/eclipse/paho.mqtt.golang"
	"log"
	"time"
)

func (this *Client) startMqtt() error {
	options := paho.NewClientOptions().
		SetPassword(this.password).
		SetUsername(this.username).
		SetClientID(this.HubId).
		SetAutoReconnect(true).
		SetCleanSession(true).
		AddBroker(this.mqttUrl).
		SetOnConnectHandler(func(client paho.Client) {
			err := this.loadOldSubscriptions()
			if err != nil {
				log.Fatal("FATAL: ", err)
			}
		})
	this.mqtt = paho.NewClient(options)
	if token := this.mqtt.Connect(); token.Wait() && token.Error() != nil {
		log.Println("Error on Client.Connect(): ", token.Error())
		return token.Error()
	}
	return nil
}

func (this *Client) ListenCommand(deviceUri string, serviceUri string, handler func(msg platform_connector_lib.CommandRequestMsg) (platform_connector_lib.CommandResponseMsg, error)) error {
	if !this.mqtt.IsConnected() {
		log.Println("WARNING: mqtt client not connected")
		return errors.New("mqtt client not connected")
	}
	token := this.mqtt.Subscribe("command/"+deviceUri+"/"+serviceUri, 1, func(client paho.Client, message paho.Message) {
		request := lib.RequestEnvelope{}
		err := json.Unmarshal(message.Payload(), &request)
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
			log.Println("ERROR: while processing command", err)
			return
		}
		response := response.ResponseEnvelope{CorrelationId: request.CorrelationId, Payload: respMsg}
		err = this.Publish("response/"+deviceUri+"/"+serviceUri, response, 2)
		if err != nil {
			log.Println("ERROR: unable to Publish response", err)
		}
	})
	if token.Wait() && token.Error() != nil {
		log.Println("Error on Client.Subscribe(): ", token.Error())
		return token.Error()
	}
	return nil
}

func (this *Client) ListenCommandWithQos(deviceUri string, serviceUri string, qos byte, handler func(msg platform_connector_lib.CommandRequestMsg) (platform_connector_lib.CommandResponseMsg, error)) error {
	if !this.mqtt.IsConnected() {
		log.Println("WARNING: mqtt client not connected")
		return errors.New("mqtt client not connected")
	}
	token := this.mqtt.Subscribe("command/"+deviceUri+"/"+serviceUri, qos, func(client paho.Client, message paho.Message) {
		request := lib.RequestEnvelope{}
		err := json.Unmarshal(message.Payload(), &request)
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
			log.Println("ERROR: while processing command", err)
			return
		}
		response := response.ResponseEnvelope{CorrelationId: request.CorrelationId, Payload: respMsg}
		err = this.Publish("response/"+deviceUri+"/"+serviceUri, response, qos)
		if err != nil {
			log.Println("ERROR: unable to Publish response", err)
		}
	})
	if token.Wait() && token.Error() != nil {
		log.Println("Error on Client.Subscribe(): ", token.Error())
		return token.Error()
	}
	return nil
}

func (this *Client) Unsubscribe(deviceUri string, serviceUri string) (err error) {
	if !this.mqtt.IsConnected() {
		log.Println("WARNING: mqtt client not connected")
		return errors.New("mqtt client not connected")
	}
	token := this.mqtt.Unsubscribe("command/" + deviceUri + "/" + serviceUri)
	if token.Wait() && token.Error() != nil {
		log.Println("Error on Client.Unsubscribe(): ", token.Error())
		return token.Error()
	}
	return nil
}

func (this *Client) SendEvent(deviceUri string, serviceUri string, msg platform_connector_lib.EventMsg) (err error) {
	return this.Publish("event/"+deviceUri+"/"+serviceUri, msg, 2)
}

func (this *Client) SendEventWithQos(deviceUri string, serviceUri string, msg platform_connector_lib.EventMsg, qos byte) (err error) {
	return this.Publish("event/"+deviceUri+"/"+serviceUri, msg, qos)
}

func (this *Client) Publish(topic string, msg interface{}, qos byte) (err error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if !this.mqtt.IsConnected() {
		log.Println("WARNING: mqtt client not connected")
		return errors.New("mqtt client not connected")
	}
	token := this.mqtt.Publish(topic, 2, false, string(payload))
	if token.Wait() && token.Error() != nil {
		log.Println("Error on Client.Publish(): ", token.Error())
		return token.Error()
	}
	return err
}

type Subscription struct {
	Topic   string
	Handler paho.MessageHandler
}

func (this *Client) registerSubscription(topic string, handler paho.MessageHandler) {
	this.subscriptionsMux.Lock()
	defer this.subscriptionsMux.Unlock()
	this.subscriptions[topic] = handler
}

func (this *Client) unregisterSubscriptions(topic string) {
	this.subscriptionsMux.Lock()
	defer this.subscriptionsMux.Unlock()
	delete(this.subscriptions, topic)
}

func (this *Client) getSubscriptions() (result []Subscription) {
	this.subscriptionsMux.Lock()
	defer this.subscriptionsMux.Unlock()
	for topic, handler := range this.subscriptions {
		result = append(result, Subscription{Topic: topic, Handler: handler})
	}
	return
}

func (this *Client) loadOldSubscriptions() error {
	if !this.mqtt.IsConnected() {
		log.Println("WARNING: mqtt client not connected")
		return errors.New("mqtt client not connected")
	}
	subs := this.getSubscriptions()
	for _, sub := range subs {
		log.Println("resubscribe to", sub.Topic)
		token := this.mqtt.Subscribe(sub.Topic, 2, sub.Handler)
		if token.Wait() && token.Error() != nil {
			log.Println("Error on Subscribe: ", sub.Topic, token.Error())
			return token.Error()
		}
	}
	return nil
}
