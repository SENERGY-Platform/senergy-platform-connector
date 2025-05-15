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

func TestIgnore4(t *testing.T) {
	testIgnore(t, client.MQTT4)
}

func TestIgnore5(t *testing.T) {
	testIgnore(t, client.MQTT5)
}

func testIgnore(t *testing.T, mqttVersion client.MqttVersion) {
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
	config.AuthClientId = "connector"
	config.AuthClientSecret = "secret"
	config.MutedUserNotificationTitles = nil

	clientId := ""

	notifyCalls := map[string][]string{}
	notifyServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		temp, _ := io.ReadAll(request.Body)
		notifyCalls[request.URL.Path] = append(notifyCalls[request.URL.Path], strings.ReplaceAll(string(temp), clientId, "client-id-placeholder"))
	}))
	defer notifyServer.Close()
	config.NotificationUrl = notifyServer.URL

	var brokerUrlForClients string
	config, brokerUrlForClients, err = server.New(ctx, wg, config, mqttVersion)
	if err != nil {
		t.Error(err)
		return
	}

	c, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{}, config.MqttAuthMethod, mqttVersion, client.OwnerInTopicDefault)
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Stop()

	clientId = c.HubId

	adminClient, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, config.AuthClientId, config.AuthClientSecret, "", "testname", []client.DeviceRepresentation{}, config.MqttAuthMethod, mqttVersion, client.OwnerInTopicDefault)
	if err != nil {
		t.Error(err)
		return
	}

	defer adminClient.Stop()

	ignoredMsg := map[string][]string{}
	ignoredMux := sync.Mutex{}
	err = adminClient.Subscribe("ignored/#", 2, func(topic string, payload []byte) {
		ignoredMux.Lock()
		defer ignoredMux.Unlock()
		ignoredMsg[topic] = append(ignoredMsg[topic], string(payload))
	})
	if err != nil {
		t.Error(err)
		return
	}

	err = adminClient.Subscribe("foo/bar", 2, func(topic string, payload []byte) {
		t.Error("unexpected message")
		return
	})
	if err != nil {
		t.Error(err)
		return
	}
	err = adminClient.Subscribe("event/#", 2, func(topic string, payload []byte) {
		t.Error("unexpected message")
		return
	})
	if err != nil {
		t.Error(err)
		return
	}

	err = c.Publish("foo/bar", "my message", 2)
	if err != nil {
		t.Error(err)
		return
	}
	eventprefix := "event/"
	if config.ForceTopicsWithOwner {
		eventprefix = "event/ownerid/"
	}
	err = c.Publish(eventprefix+"not/msgformat", "my message", 2)
	if err != nil {
		t.Error(err)
		return
	}

	err = c.Publish(eventprefix+"not/foryou", map[string]string{}, 2)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(10 * time.Second) //wait for command to finish

	expectedNotifications := map[string][]string{
		"/notifications": {
			"{\"userId\":\"sepl\",\"title\":\"Client-Error\",\"message\":\"Error: ignore message to foo/bar: no matching topic handler found\\n\\nClient: client-id-placeholder\",\"topic\":\"mgw\"}\n",
			"{\"userId\":\"sepl\",\"title\":\"Client-Error\",\"message\":\"Error: ignore message to " + eventprefix + "not/msgformat: json: cannot unmarshal string into Go value of type map[string]string\\n\\nClient: client-id-placeholder\",\"topic\":\"mgw\"}\n",
			"{\"userId\":\"sepl\",\"title\":\"Client-Error\",\"message\":\"Error: ignore message to " + eventprefix + "not/foryou: not found: device-manager/local-devices/not 404\\n\\nClient: client-id-placeholder\",\"topic\":\"mgw\"}\n",
		},
	}

	if !reflect.DeepEqual(notifyCalls, expectedNotifications) {
		t.Errorf("\n%#v\n%#v\n", expectedNotifications, notifyCalls)
		return
	}

	expectedIgnores := map[string][]string{
		"ignored/" + eventprefix + "not/foryou":    {"not found: device-manager/local-devices/not 404"},
		"ignored/" + eventprefix + "not/msgformat": {"json: cannot unmarshal string into Go value of type map[string]string"},
		"ignored/foo/bar":                          {"no matching topic handler found"},
	}
	if !reflect.DeepEqual(ignoredMsg, expectedIgnores) {
		t.Errorf("\n%#v\n%#v\n", ignoredMsg, expectedIgnores)
		return
	}
}
