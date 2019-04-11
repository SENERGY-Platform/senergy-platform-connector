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
	"gilhub.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/SENERGY-Platform/platform-connector-lib"
	paho "github.com/eclipse/paho.mqtt.golang"
	"log"
)

func (this *Client) startMqtt() error {
	options := paho.NewClientOptions().
		SetPassword(this.password).
		SetUsername(this.username).
		SetClientID(this.HubId).
		SetAutoReconnect(true).
		SetCleanSession(true).
		AddBroker(this.mqttUrl)
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
		respMsg, err := handler(request.Payload)
		if err != nil {
			log.Println("ERROR: while processing command", err)
			return
		}
		response := lib.ResponseEnvelope{CorrelationId: request.CorrelationId, Payload: respMsg}
		err = this.publish("response/"+deviceUri+"/"+serviceUri, response)
		if err != nil {
			log.Println("ERROR: unable to publish response", err)
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
	return this.publish("event/"+deviceUri+"/"+serviceUri, msg)
}

func (this *Client) publish(topic string, msg interface{}) (err error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if !this.mqtt.IsConnected() {
		log.Println("WARNING: mqtt client not connected")
		return errors.New("mqtt client not connected")
	}
	token := this.mqtt.Publish(topic, 1, false, string(payload))
	if token.Wait() && token.Error() != nil {
		log.Println("Error on Client.Publish(): ", token.Error())
		return token.Error()
	}
	return err
}
