/*
 * Copyright 2023 InfAI (CC SES)
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
	"encoding/json"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

func (this *Client) startMqtt5() (err error) {
	broker, err := url.Parse(this.mqttUrl)
	if err != nil {
		return err
	}
	config := autopaho.ClientConfig{
		BrokerUrls:                    []*url.URL{broker},
		CleanStartOnInitialConnection: true,
		OnConnectionUp: func(manager *autopaho.ConnectionManager, connack *paho.Connack) {
			log.Println("mqtt (re)connected")
			err := this.loadOldSubscriptions()
			if err != nil {
				debug.PrintStack()
				log.Fatal("FATAL: ", err)
			}
		},
		OnConnectError: func(err error) {
			log.Println("mqtt connection error:", err)
		},
		KeepAlive:  30,
		PahoErrors: log.New(os.Stderr, "[PAHO-ERR] ", log.LstdFlags),
		ClientConfig: paho.ClientConfig{
			ClientID: this.HubId,
			OnServerDisconnect: func(disconnect *paho.Disconnect) {
				log.Println("mqtt servicer disconnect")
			},
			OnClientError: func(err error) {
				log.Println("mqtt client error:", err)
			},
			Router: paho.NewStandardRouter(),
		},
	}

	if this.authenticationMethod == "certificate" {
		dir, err := os.Getwd()
		ClientCertificatePath := filepath.Join(dir, "mqtt_certs", "mock_client", "client.crt")
		PrivateKeyPath := filepath.Join(dir, "mqtt_certs", "mock_client", "private.key")
		RootCACertificatePath := filepath.Join(dir, "mqtt_certs", "ca", "ca.crt")

		tlsConfig, err := lib.CreateTLSConfig(ClientCertificatePath, PrivateKeyPath, RootCACertificatePath)
		if err != nil {
			log.Println("Error on MQTT TLS config", err)
			return err
		}
		config.TlsCfg = tlsConfig
	} else {
		config.ConnectUsername = this.username
		config.ConnectPassword = []byte(this.password)
	}

	this.mqtt5, err = autopaho.NewConnection(context.Background(), config)
	if err != nil {
		return err
	}
	this.mqtt5router = config.Router
	timeout, _ := context.WithTimeout(context.Background(), time.Minute)
	return this.mqtt5.AwaitConnection(timeout)
}

func (this *Client) SubscribeMqtt5(topic string, qos byte, callback func(topic string, pl []byte)) error {
	this.mqtt5router.RegisterHandler(topic, func(publish *paho.Publish) {
		callback(publish.Topic, publish.Payload)
	})
	this.registerSubscription(topic, qos, nil)

	timeout, _ := context.WithTimeout(context.Background(), time.Minute)
	_, err := this.mqtt5.Subscribe(timeout, &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{{Topic: topic, QoS: qos}},
	})
	if err != nil {
		log.Println("Error on Client.Subscribe(): ", err)
		return err
	}
	return nil
}

func (this *Client) UnsubscribeMqtt5(deviceUri string, serviceUri string) (err error) {
	timeout, _ := context.WithTimeout(context.Background(), time.Minute)
	if this.ownerInTopic {
		_, err = this.mqtt5.Unsubscribe(timeout, &paho.Unsubscribe{
			Topics: []string{"command/" + this.userid + "/" + deviceUri + "/" + serviceUri},
		})
	} else {
		_, err = this.mqtt5.Unsubscribe(timeout, &paho.Unsubscribe{
			Topics: []string{"command/" + deviceUri + "/" + serviceUri},
		})
	}

	if err != nil {
		log.Println("Error on Client.Unsubscribe(): ", err)
		return err
	}
	return nil
}

func (this *Client) PublishMqtt5(topic string, msg interface{}, qos byte) (err error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	timeout, _ := context.WithTimeout(context.Background(), time.Minute)
	_, err = this.mqtt5.Publish(timeout, &paho.Publish{
		QoS:     qos,
		Retain:  false,
		Topic:   topic,
		Payload: payload,
	})
	if err != nil {
		log.Println("Error on Client.PublishMqtt5(): ", err)
		return err
	}
	return err
}

func (this *Client) loadOldSubscriptionsMqtt5() (err error) {
	subs := this.getSubscriptions()
	mqtt5Subs := []paho.SubscribeOptions{}
	for _, sub := range subs {
		log.Println("resubscribe to", sub.Topic)
		mqtt5Subs = append(mqtt5Subs, paho.SubscribeOptions{Topic: sub.Topic, QoS: sub.Qos})
	}
	if len(mqtt5Subs) > 0 {
		timeout, _ := context.WithTimeout(context.Background(), time.Minute)
		_, err = this.mqtt5.Subscribe(timeout, &paho.Subscribe{
			Subscriptions: mqtt5Subs,
		})
	}
	return err
}
