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

package test

import (
	"context"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	paho "github.com/eclipse/paho.mqtt.golang"
	"log"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestProcessHandler(t *testing.T) {
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := configuration.LoadConfig("../config.json")
	if err != nil {
		t.Error(err)
		return
	}
	config.Debug = true
	config.FatalKafkaError = false
	config.Validate = true
	config.ValidateAllowUnknownField = true
	config.ValidateAllowMissingField = true
	config.Log = "stdout"
	config.MqttAuthMethod = "password"

	var brokerUrlForClients string
	config, brokerUrlForClients, err = server.New(ctx, wg, config, client.MQTT4)
	if err != nil {
		t.Error(err)
		return
	}

	c, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "user", "user", "", "testname", []client.DeviceRepresentation{}, config.MqttAuthMethod, client.MQTT4, config.TopicsWithOwner)
	if err != nil {
		t.Error(err)
		return
	}

	mux := sync.Mutex{}

	//process-sync

	adminSync := paho.NewClient(paho.NewClientOptions().
		SetUsername("admin").
		SetPassword("admin").
		SetClientID("admin").
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetOnConnectHandler(func(_ paho.Client) {
			log.Println("TEST: adminSync connected")
		}).
		SetConnectionLostHandler(func(c paho.Client, err error) {
			log.Println("TEST: adminSync disconnected", err)
		}).
		AddBroker(config.MqttBroker))

	token := adminSync.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		t.Error("timeout")
		return
	}

	adminSyncAll := map[string][]string{}
	token = adminSync.Subscribe("processes/#", 2, func(c paho.Client, message paho.Message) {
		log.Println("TEST: adminSync processes/# receive", message.Topic(), string(message.Payload()))
		mux.Lock()
		defer mux.Unlock()
		adminSyncAll[message.Topic()] = append(adminSyncAll[message.Topic()], string(message.Payload()))
	})
	if !token.WaitTimeout(10 * time.Second) {
		t.Error("timeout")
		return
	}

	sharedAdminSync := paho.NewClient(paho.NewClientOptions().
		SetUsername("admin").
		SetPassword("admin").
		SetClientID("shared-admin").
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetOnConnectHandler(func(_ paho.Client) {
			log.Println("TEST: adminSync connected")
		}).
		SetConnectionLostHandler(func(c paho.Client, err error) {
			log.Println("TEST: adminSync disconnected", err)
		}).
		AddBroker(config.MqttBroker))

	token = sharedAdminSync.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		t.Error("timeout")
		return
	}

	sharedAdminAll := map[string][]string{}
	token = sharedAdminSync.Subscribe("$share/group/processes/+/deployment", 2, func(c paho.Client, message paho.Message) {
		log.Println("TEST: sharedAdminAll $share/group/processes/+/deployment receive", message.Topic(), string(message.Payload()))
		mux.Lock()
		defer mux.Unlock()
		sharedAdminAll[message.Topic()] = append(sharedAdminAll[message.Topic()], string(message.Payload()))
	})
	if !token.WaitTimeout(10 * time.Second) {
		t.Error("timeout")
		return
	}

	// mgw-process-sync-client

	clientSync := paho.NewClient(paho.NewClientOptions().
		SetUsername("user").
		SetPassword("user").
		SetClientID("client").
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetOnConnectHandler(func(_ paho.Client) {
			log.Println("TEST: clientSync connected")
		}).
		SetConnectionLostHandler(func(c paho.Client, err error) {
			log.Println("TEST: clientSync disconnected", err)
		}).
		AddBroker(config.MqttBroker))

	token = clientSync.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		t.Error("timeout")
		return
	}

	clientSyncAll := map[string][]string{}
	token = clientSync.Subscribe("processes/"+c.HubId+"/#", 2, func(_ paho.Client, message paho.Message) {
		log.Println("TEST: clientSync processes/"+c.HubId+"/# receive", message.Topic(), string(message.Payload()))
		mux.Lock()
		defer mux.Unlock()
		clientSyncAll[message.Topic()] = append(clientSyncAll[message.Topic()], string(message.Payload()))
	})
	if !token.WaitTimeout(10 * time.Second) {
		t.Error("timeout")
		return
	}

	token = clientSync.Publish("processes/"+c.HubId+"/deployment", 2, false, "client publish")
	if !token.WaitTimeout(10 * time.Second) {
		t.Error("timeout")
		return
	}

	token = adminSync.Publish("processes/"+c.HubId+"/deployment", 2, false, "admin publish")
	if !token.WaitTimeout(10 * time.Second) {
		t.Error("timeout")
		return
	}

	time.Sleep(10 * time.Second)
	mux.Lock()
	defer mux.Unlock()

	if !reflect.DeepEqual(adminSyncAll, map[string][]string{"processes/" + c.HubId + "/deployment": {"client publish", "admin publish"}}) {
		t.Error(adminSyncAll)
	}
	if !reflect.DeepEqual(sharedAdminAll, map[string][]string{"processes/" + c.HubId + "/deployment": {"client publish", "admin publish"}}) {
		t.Error(sharedAdminAll)
	}
	if !reflect.DeepEqual(clientSyncAll, map[string][]string{"processes/" + c.HubId + "/deployment": {"client publish", "admin publish"}}) {
		t.Error(clientSyncAll)
	}
}
