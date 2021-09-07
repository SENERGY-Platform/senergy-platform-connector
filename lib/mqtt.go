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
	"context"
	"errors"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	uuid "github.com/satori/go.uuid"
	"log"

	paho "github.com/eclipse/paho.mqtt.golang"
)

type Mqtt struct {
	client paho.Client
	Debug  bool
}

func MqttStart(ctx context.Context, config configuration.Config) (mqtt *Mqtt, err error) {
	mqtt = &Mqtt{Debug: config.MqttDebug}
	options := paho.NewClientOptions().
		SetPassword(config.AuthClientSecret).
		SetUsername(config.AuthClientId).
		SetAutoReconnect(true).
		SetCleanSession(true).
		SetClientID(config.AuthClientId + "_" + uuid.NewV4().String()).
		AddBroker(config.MqttBroker)

	mqtt.client = paho.NewClient(options)
	if token := mqtt.client.Connect(); token.Wait() && token.Error() != nil {
		log.Println("Error on MqttStart.Connect(): ", token.Error())
		return mqtt, token.Error()
	}

	go func() {
		<-ctx.Done()
		mqtt.client.Disconnect(0)
	}()

	return mqtt, nil
}

func (this *Mqtt) Publish(topic, msg string) (err error) {
	if !this.client.IsConnected() {
		log.Println("WARNING: mqtt client not connected")
		return errors.New("mqtt client not connected")
	}
	if this.Debug {
		log.Println("DEBUG: publish ", topic, msg)
	}
	token := this.client.Publish(topic, 2, false, msg)
	if token.Wait() && token.Error() != nil {
		log.Println("Error on Client.Publish(): ", token.Error())
		return token.Error()
	}
	return err
}
