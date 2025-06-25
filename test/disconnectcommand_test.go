/*
 * Copyright 2025 InfAI (CC SES)
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
	"bytes"
	"context"
	"encoding/json"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/webhooks/vernemqtt"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	"io"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestDisconnectCommand(t *testing.T) {
	wg := &sync.WaitGroup{}
	defer t.Log("wg done")
	defer wg.Wait()
	defer t.Log("wait for wg")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := createConf("password")
	if err != nil {
		t.Error(err)
		return
	}

	config.DisconnectCommandDelay = "1s"

	var brokerUrlForClients string
	config, brokerUrlForClients, err = server.New(ctx, wg, config, client.MQTT4)
	if err != nil {
		t.Error(err)
		return
	}

	c, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{}, "password", client.MQTT4, client.OwnerInTopicDefault)
	if err != nil {
		t.Error(err)
		return
	}

	c2, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{}, "password", client.MQTT4, client.OwnerInTopicDefault)
	if err != nil {
		t.Error(err)
		return
	}

	c3, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "admin", "admin", "", "testname", []client.DeviceRepresentation{}, "password", client.MQTT4, client.OwnerInTopicDefault)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(2 * time.Second)

	b := new(bytes.Buffer)
	err = json.NewEncoder(b).Encode(vernemqtt.DisconnectCommand{Username: "sepl"})
	if err != nil {
		return
	}
	resp, err := http.Post("http://localhost:"+config.WebhookPort+"/disconnect-command", "application/json", b)
	if err != nil {
		t.Error(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Error(resp.StatusCode)
		return
	}
	respPayload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
		return
	}
	if string(respPayload) != `{"result": "ok"}` {
		t.Error(string(respPayload))
		return
	}

	time.Sleep(2 * time.Second)

	if !reflect.DeepEqual(c.ConnLog, []string{"mqtt (re)connected", "mqtt connection lost", "mqtt (re)connected"}) {
		t.Error(c.ConnLog)
		return
	}
	if !reflect.DeepEqual(c2.ConnLog, []string{"mqtt (re)connected", "mqtt connection lost", "mqtt (re)connected"}) {
		t.Error(c.ConnLog)
		return
	}
	if !reflect.DeepEqual(c3.ConnLog, []string{"mqtt (re)connected"}) {
		t.Error(c.ConnLog)
		return
	}

}
