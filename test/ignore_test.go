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
	_ "github.com/lib/pq"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIgnore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer time.Sleep(10 * time.Second) //wait for container shutdown
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
	config.PublishToPostgres = true

	clientId := ""

	notifyCalls := map[string][]string{}
	notifyServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		temp, _ := io.ReadAll(request.Body)
		notifyCalls[request.URL.Path] = append(notifyCalls[request.URL.Path], strings.ReplaceAll(string(temp), clientId, "client-id-placeholder"))
	}))
	defer notifyServer.Close()
	config.NotificationUrl = notifyServer.URL

	var brokerUrlForClients string
	config, brokerUrlForClients, err = server.New(ctx, config)
	if err != nil {
		t.Error(err)
		return
	}

	c, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{}, config.MqttAuthMethod)
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Stop()

	clientId = c.HubId

	adminClient, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, config.AuthClientId, config.AuthClientSecret, "", "testname", []client.DeviceRepresentation{}, config.MqttAuthMethod)
	if err != nil {
		t.Error(err)
		return
	}

	defer adminClient.Stop()

	ignoredMsg := map[string][]string{}
	ignoredMux := sync.Mutex{}
	token := adminClient.Mqtt().Subscribe("ignored/#", 2, func(c paho.Client, message paho.Message) {
		ignoredMux.Lock()
		defer ignoredMux.Unlock()
		ignoredMsg[message.Topic()] = append(ignoredMsg[message.Topic()], string(message.Payload()))
	})
	if token.Wait() && token.Error() != nil {
		t.Error(token.Error())
		return
	}

	token = adminClient.Mqtt().Subscribe("foo/bar", 2, func(c paho.Client, message paho.Message) {
		t.Error("unexpected message")
		return
	})
	if token.Wait() && token.Error() != nil {
		t.Error(token.Error())
		return
	}
	token = adminClient.Mqtt().Subscribe("event/#", 2, func(c paho.Client, message paho.Message) {
		t.Error("unexpected message")
		return
	})
	if token.Wait() && token.Error() != nil {
		t.Error(token.Error())
		return
	}

	err = c.Publish("foo/bar", "my message", 2)
	if err != nil {
		t.Error(err)
		return
	}
	err = c.Publish("event/not/msgformat", "my message", 2)
	if err != nil {
		t.Error(err)
		return
	}

	err = c.Publish("event/not/foryou", map[string]string{}, 2)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(10 * time.Second) //wait for command to finish

	expectedNotifications := map[string][]string{
		"/notifications": {
			"{\"userId\":\"sepl\",\"title\":\"Client-Error\",\"message\":\"Error: unable to publish to topic foo/bar: no matching topic handler found\\n\\nClient: client-id-placeholder\"}\n",
			"{\"userId\":\"sepl\",\"title\":\"Client-Error\",\"message\":\"Error: unable to publish to topic event/not/msgformat: json: cannot unmarshal string into Go value of type map[string]string\\n\\nClient: client-id-placeholder\"}\n",
			"{\"userId\":\"sepl\",\"title\":\"Client-Error\",\"message\":\"Error: unable to publish to topic event/not/foryou: not found\\n\\nClient: client-id-placeholder\"}\n",
		},
	}

	if !reflect.DeepEqual(notifyCalls, expectedNotifications) {
		t.Errorf("%#v", notifyCalls)
		return
	}

	expectedIgnores := map[string][]string{
		"ignored/event/not/foryou":    {"not found"},
		"ignored/event/not/msgformat": {"json: cannot unmarshal string into Go value of type map[string]string"},
		"ignored/foo/bar":             {"no matching topic handler found"},
	}
	if !reflect.DeepEqual(ignoredMsg, expectedIgnores) {
		t.Errorf("%#v", ignoredMsg)
		return
	}
}
