/*
 * Copyright 2026 InfAI (CC SES)
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
	"net/http"
	"sync"
	"testing"
	"time"

	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	paho "github.com/eclipse/paho.mqtt.golang"
	_ "github.com/lib/pq"
)

// subscribeWithResult subscribes with the raw mqtt 3.1.1 client to get the SUBACK return code:
// the owner of a rejected subscription gets 0x80, an accepted one the granted qos.
func subscribeWithResult(c *client.Client, topic string, qos byte, received *stringsByKey) (byte, error) {
	token := c.Mqtt().Subscribe(topic, qos, func(_ paho.Client, message paho.Message) {
		received.Add(message.Topic(), string(message.Payload()))
	})
	token.Wait()
	if token.Error() != nil {
		return 0, token.Error()
	}
	return token.(*paho.SubscribeToken).Result()[topic], nil
}

func grantLocalDeviceAccess(url string, userId string, ownerId string, localId string) error {
	body, err := json.Marshal(map[string]string{"user_id": userId, "owner_id": ownerId, "local_id": localId})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, url+"/mock/local-device-access", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &http.ProtocolError{ErrorString: "unexpected status " + resp.Status}
	}
	return nil
}

func TestEventSubscribe4(t *testing.T) {
	const mqttVersion = client.MQTT4
	const authenticationMethod = "password"

	wg := &sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := createConf(authenticationMethod)
	if err != nil {
		t.Error(err)
		return
	}
	var brokerUrlForClients string
	config, brokerUrlForClients, err = server.New(ctx, wg, config, mqttVersion)
	if err != nil {
		t.Error(err)
		return
	}

	characteristicId, err := createCharacteristic("test1", config)
	if err != nil {
		t.Error(err)
		return
	}
	deviceTypeId, _, _, _, _, err := createDeviceType(config, config.DeviceManagerUrl, characteristicId, nil)
	if err != nil {
		t.Error(err)
		return
	}

	newClient := func(user string, devices []client.DeviceRepresentation) *client.Client {
		c, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, user, user, "", "testname-"+user, devices, authenticationMethod, mqttVersion, client.OwnerInTopicDefault, nil)
		if err != nil {
			t.Error(err)
			return nil
		}
		return c
	}

	//the hub of the owner "sepl" publishes the events of its device "test1"
	hub := newClient("sepl", []client.DeviceRepresentation{{Name: "test1", Uri: "test1", IotType: deviceTypeId}})
	if hub == nil {
		return
	}
	defer hub.Stop()
	ownerSub := newClient("sepl", nil)
	if ownerSub == nil {
		return
	}
	defer ownerSub.Stop()
	//"user" gets read access, "stranger" (token subject "unknown") does not
	granted := newClient("user", nil)
	if granted == nil {
		return
	}
	defer granted.Stop()
	stranger := newClient("stranger", nil)
	if stranger == nil {
		return
	}
	defer stranger.Stop()

	err = grantLocalDeviceAccess(config.DeviceRepoUrl, "user", "sepl", "test1")
	if err != nil {
		t.Error(err)
		return
	}

	topic := "event/sepl/test1/sepl_get"
	ownerMsgs, grantedMsgs, strangerMsgs := newStringsByKey(), newStringsByKey(), newStringsByKey()

	expectCode := func(name string, c *client.Client, topic string, received *stringsByKey, expected byte) {
		code, err := subscribeWithResult(c, topic, 1, received)
		if err != nil {
			t.Error(name, err)
			return
		}
		if code != expected {
			t.Errorf("%v: subscribe %v: expected suback code %#x, got %#x", name, topic, expected, code)
		}
	}
	expectCode("owner", ownerSub, topic, ownerMsgs, 1)
	expectCode("granted user", granted, topic, grantedMsgs, 1)
	expectCode("granted user, all services", granted, "event/sepl/test1/+", grantedMsgs, 1)
	expectCode("stranger", stranger, topic, strangerMsgs, 0x80)
	expectCode("stranger, all services", stranger, "event/sepl/test1/+", strangerMsgs, 0x80)
	expectCode("granted user, other device", granted, "event/sepl/other/sepl_get", grantedMsgs, 0x80)
	expectCode("granted user, all devices", granted, "event/sepl/+/sepl_get", grantedMsgs, 0x80)
	expectCode("granted user, wildcard owner", granted, "event/+/test1/sepl_get", grantedMsgs, 0x80)
	expectCode("granted user, hash", granted, "event/sepl/test1/#", grantedMsgs, 0x80)
	expectCode("owner, no owner level", ownerSub, "event/test1/sepl_get", ownerMsgs, 0x80)

	err = hub.SendEvent("test1", "sepl_get", platform_connector_lib.EventMsg{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})
	if err != nil {
		t.Error(err)
		return
	}

	waitFor(60*time.Second, func() bool {
		return len(ownerMsgs.Get(topic)) >= 1 && len(grantedMsgs.Get(topic)) >= 1
	})
	//settle, so a message that is wrongly delivered to the stranger is still caught below
	time.Sleep(settleTime)

	if n := len(ownerMsgs.Get(topic)); n == 0 {
		t.Error("owner did not receive the event")
	}
	if len(ownerMsgs.All()) != 1 {
		t.Errorf("owner: unexpected messages: %#v", ownerMsgs.All())
	}
	if len(grantedMsgs.Get(topic)) == 0 {
		t.Error("user with access did not receive the event")
	}
	//the exact and the "+" subscription both match, so the broker may deliver once per subscription;
	//All() is keyed by topic, so a second delivery on the same topic does not count here
	if len(grantedMsgs.All()) != 1 {
		t.Errorf("user with access: unexpected messages: %#v", grantedMsgs.All())
	}
	if len(strangerMsgs.All()) != 0 {
		t.Errorf("user without access received messages: %#v", strangerMsgs.All())
	}
}
