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
	"log/slog"
	"net/url"
	"time"

	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/google/uuid"

	paho4 "github.com/eclipse/paho.mqtt.golang"
)

type Mqtt interface {
	Publish(topic, msg string) (err error)
}

func MqttStart(ctx context.Context, config configuration.Config) (mqtt Mqtt, err error) {
	if config.MqttVersion == "5" {
		return Mqtt5Start(ctx, config)
	} else {
		return Mqtt4Start(ctx, config)
	}
}

type Mqtt4 struct {
	client paho4.Client
	Debug  bool
	config configuration.Config
}

func Mqtt4Start(ctx context.Context, config configuration.Config) (mqtt *Mqtt4, err error) {
	mqtt = &Mqtt4{Debug: config.MqttDebug, config: config}
	options := paho4.NewClientOptions().
		SetPassword(config.AuthClientSecret).
		SetUsername(config.AuthClientId).
		SetAutoReconnect(true).
		SetCleanSession(true).
		SetClientID(config.AuthClientId + "_" + uuid.NewString()).
		AddBroker(config.MqttBroker)

	mqtt.client = paho4.NewClient(options)
	if token := mqtt.client.Connect(); token.Wait() && token.Error() != nil {
		config.GetLogger().Error("unable to start mqtt connection", "error", token.Error())
		return mqtt, token.Error()
	}

	go func() {
		<-ctx.Done()
		mqtt.client.Disconnect(0)
	}()

	return mqtt, nil
}

func (this *Mqtt4) Publish(topic, msg string) (err error) {
	if !this.client.IsConnected() {
		this.config.GetLogger().Warn("mqtt client not connected")
		return errors.New("mqtt client not connected")
	}
	this.config.GetLogger().Debug("publish to mqtt", "topic", topic, "msg", msg)
	token := this.client.Publish(topic, 2, false, msg)
	if token.Wait() && token.Error() != nil {
		this.config.GetLogger().Error("unable to publish to mqtt", "topic", topic, "msg", msg, "error", token.Error())
		return token.Error()
	}
	return err
}

type Mqtt5 struct {
	client *autopaho.ConnectionManager
	Debug  bool
	config configuration.Config
}

func Mqtt5Start(ctx context.Context, config configuration.Config) (mqtt *Mqtt5, err error) {
	mqtt = &Mqtt5{Debug: config.MqttDebug, config: config}

	broker, err := url.Parse(config.MqttBroker)
	if err != nil {
		return mqtt, err
	}

	mqttLogger := slog.NewLogLogger(config.GetLogger().Handler(), slog.LevelError)
	mqttLogger.SetPrefix("[PAHO-ERR] ")

	c := autopaho.ClientConfig{
		BrokerUrls: []*url.URL{broker},
		OnConnectionUp: func(manager *autopaho.ConnectionManager, connack *paho.Connack) {
			config.GetLogger().Info("mqtt (re)connected")
		},
		OnConnectError: func(err error) {
			config.GetLogger().Error("mqtt connection error", "error", err)
		},
		PahoErrors: mqttLogger,
		KeepAlive:  30,
		ClientConfig: paho.ClientConfig{
			ClientID: config.AuthClientId + "_" + uuid.NewString(),
			OnServerDisconnect: func(disconnect *paho.Disconnect) {
				config.GetLogger().Info("mqtt server disconnect")
			},
			OnClientError: func(err error) {
				config.GetLogger().Error("mqtt client error", "error", err)
			},
		},
	}

	c.SetUsernamePassword(config.AuthClientId, []byte(config.AuthClientSecret))

	mqtt.client, err = autopaho.NewConnection(ctx, c)
	if err != nil {
		return mqtt, err
	}
	go func() {
		<-ctx.Done()
		disconnecttimeout, _ := context.WithTimeout(context.Background(), 10*time.Second)
		config.GetLogger().Info("disconnecting mqtt client", "result", mqtt.client.Disconnect(disconnecttimeout))
	}()
	timeout, _ := context.WithTimeout(ctx, time.Minute)
	return mqtt, mqtt.client.AwaitConnection(timeout)
}

func (this *Mqtt5) Publish(topic, msg string) (err error) {
	this.config.GetLogger().Debug("publish to mqtt", "topic", topic, "msg", msg)
	timeout, _ := context.WithTimeout(context.Background(), time.Minute)
	_, err = this.client.Publish(timeout, &paho.Publish{
		QoS:     2,
		Retain:  false,
		Topic:   topic,
		Payload: []byte(msg),
	})
	if err != nil {
		this.config.GetLogger().Error("unable to publish to mqtt", "topic", topic, "error", err)
		return err
	}
	return err
}
