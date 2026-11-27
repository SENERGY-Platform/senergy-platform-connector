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
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server/docker"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server/mock/jwks"
	paho "github.com/eclipse/paho.mqtt.golang"
)

// vernemqWithWebsocketImage is the broker version the token login was verified against. The shared helper
// stays on its own version, so the rest of the suite is not affected by this test.
const vernemqWithWebsocketImage = "ghcr.io/senergy-platform/vernemq:v2.2.1"

// TestTokenLogin is the gate of the token login design: a browser client logs in with username = user id and
// password = access token on a WebSocket listener with its own mountpoint, and has to receive the events that a
// certificate client publishes in the default mountpoint.
func TestTokenLogin(t *testing.T) {
	const authenticationMethod = "certificate"
	const mqttVersion = client.MQTT4

	wg := &sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keys, err := jwks.New("test-kid")
	if err != nil {
		t.Fatal(err)
	}
	foreignKeys, err := jwks.New("test-kid")
	if err != nil {
		t.Fatal(err)
	}
	jwksServer := keys.Serve()
	defer jwksServer.Close()

	config, err := createConf(authenticationMethod)
	if err != nil {
		t.Fatal(err)
	}
	config.KeycloakCertUrl = jwksServer.URL
	config.TokenLoginMountpoint = "web"

	var brokerUrlForClients, websocketUrl string
	config, brokerUrlForClients, websocketUrl, err = server.NewWithBrokerOptions(ctx, wg, config, mqttVersion, docker.VernemqttOptions{
		Image:               vernemqWithWebsocketImage,
		WebsocketMountpoint: "web",
	})
	if err != nil {
		t.Fatal(err)
	}

	characteristicId, err := createCharacteristic("test1", config)
	if err != nil {
		t.Fatal(err)
	}
	deviceTypeId, _, _, _, _, err := createDeviceType(config, config.DeviceManagerUrl, characteristicId, nil)
	if err != nil {
		t.Fatal(err)
	}

	// the certificate of the test client is issued for "sepl"
	hub, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname-sepl", []client.DeviceRepresentation{{Name: "test1", Uri: "test1", IotType: deviceTypeId}}, authenticationMethod, mqttVersion, client.OwnerInTopicDefault, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Stop()

	certReceived := newStringsByKey()
	certSub, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname-sepl-sub", nil, authenticationMethod, mqttVersion, client.OwnerInTopicDefault, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer certSub.Stop()

	topic := "event/sepl/test1/sepl_get"
	if code, err := subscribeWithResult(certSub, topic, 1, certReceived); err != nil || code != 1 {
		t.Fatal("certificate subscriber:", code, err)
	}

	webClient := func(username string, password string, clientId string) (paho.Client, error) {
		options := paho.NewClientOptions().
			AddBroker(websocketUrl).
			SetClientID(clientId).
			SetCleanSession(true).
			SetAutoReconnect(false).
			SetConnectTimeout(10 * time.Second).
			SetUsername(username)
		if password != "" {
			options.SetPassword(password)
		}
		c := paho.NewClient(options)
		token := c.Connect()
		token.Wait()
		return c, token.Error()
	}
	sign := func(keys *jwks.Keys, sub string, exp time.Time) string {
		token, err := keys.Sign(map[string]interface{}{"sub": sub, "typ": "Bearer", "exp": exp.Unix()})
		if err != nil {
			t.Fatal(err)
		}
		return token
	}

	t.Run("refused logins", func(t *testing.T) {
		cases := map[string]struct{ username, password string }{
			"no password":    {"sepl", ""},
			"wrong sub":      {"someone-else", sign(keys, "sepl", time.Now().Add(time.Hour))},
			"expired":        {"sepl", sign(keys, "sepl", time.Now().Add(-time.Minute))},
			"bad signature":  {"sepl", sign(foreignKeys, "sepl", time.Now().Add(time.Hour))},
			"not a token":    {"sepl", "sepl"},
			"bearer garbage": {"sepl", "Bearer x.y.z"},
		}
		for name, c := range cases {
			conn, err := webClient(c.username, c.password, "refused")
			if err == nil {
				conn.Disconnect(0)
				t.Errorf("%v: expected the login to be refused", name)
			}
		}
	})

	// the sweeper check below needs a token that expires while the test runs
	shortLived, err := webClient("sepl", sign(keys, "sepl", time.Now().Add(10*time.Second)), "wshort")
	if err != nil {
		t.Fatal("login with a short lived token:", err)
	}
	defer shortLived.Disconnect(0)

	webReceived := newStringsByKey()
	web, err := webClient("sepl", "Bearer "+sign(keys, "sepl", time.Now().Add(time.Hour)), "wclient")
	if err != nil {
		t.Fatal("login with a valid token:", err)
	}
	defer web.Disconnect(0)
	if !web.IsConnected() {
		t.Fatal("token client is not connected")
	}

	webSubscribe := func(c paho.Client, topic string) (byte, error) {
		token := c.Subscribe(topic, 1, func(_ paho.Client, message paho.Message) {
			t.Logf("token client got %v qos=%v duplicate=%v retained=%v", message.Topic(), message.Qos(), message.Duplicate(), message.Retained())
			webReceived.Add(message.Topic(), string(message.Payload()))
		})
		token.Wait()
		if token.Error() != nil {
			return 0, token.Error()
		}
		return token.(*paho.SubscribeToken).Result()[topic], nil
	}

	t.Run("subscribe restrictions", func(t *testing.T) {
		// paho keeps the local route of a subscription the broker rejected, so a message matching both "#" and
		// the event topic would run the callback twice. The rejected attempts therefore use a client of their own.
		rejected, err := webClient("sepl", sign(keys, "sepl", time.Now().Add(time.Hour)), "wrejected")
		if err != nil {
			t.Fatal(err)
		}
		defer rejected.Disconnect(0)
		expect := func(c paho.Client, topic string, expected byte) {
			code, err := webSubscribe(c, topic)
			if err != nil {
				t.Error(topic, err)
				return
			}
			if code != expected {
				t.Errorf("subscribe %v: expected suback code %#x, got %#x", topic, expected, code)
			}
		}
		expect(rejected, "command/sepl/test1/sepl_get", 0x80)
		expect(rejected, "#", 0x80)
		expect(rejected, "$SYS/#", 0x80)
		expect(rejected, "response/sepl", 0x80)
		expect(rejected, "event/someone-else/test1/sepl_get", 0x80) // not the owner, no read access
		expect(web, topic, 1)
		// a rejected SUBACK is the client's view; the broker must hold the accepted topic only
		subscriptions, err := sessionTopics(config.VmqAdminApiUrl, ":wclient")
		if err != nil {
			t.Fatal(err)
		}
		if len(subscriptions) != 1 || subscriptions[0] != topic {
			t.Errorf("broker subscriptions of the token client: expected only %v, got %#v", topic, subscriptions)
		}
		subscriptions, err = sessionTopics(config.VmqAdminApiUrl, ":wrejected")
		if err != nil {
			t.Fatal(err)
		}
		for _, subscription := range subscriptions {
			if subscription != "" {
				t.Errorf("broker subscriptions of the rejected client: expected none, got %#v", subscriptions)
				break
			}
		}
	})

	t.Run("publish is refused", func(t *testing.T) {
		// MQTT 3.1.1 closes the connection after a publish that is not authorized, so the client that
		// publishes is not the one that subscribes. The proof that nothing is delivered is checked below,
		// after a control message of the hub has passed.
		publisher, err := webClient("sepl", sign(keys, "sepl", time.Now().Add(time.Hour)), "wpublisher")
		if err != nil {
			t.Fatal(err)
		}
		defer publisher.Disconnect(0)
		publisher.Publish(topic, 0, false, `{"from":"web-client"}`).Wait()
		if !waitFor(10*time.Second, func() bool { return !publisher.IsConnected() }) {
			t.Error("expected the broker to close the connection of the refused publisher")
		}
	})

	err = hub.SendEvent("test1", "sepl_get", platform_connector_lib.EventMsg{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})
	if err != nil {
		t.Fatal(err)
	}
	if !waitFor(60*time.Second, func() bool { return len(webReceived.Get(topic)) >= 1 && len(certReceived.Get(topic)) >= 1 }) {
		t.Fatalf("token client did not receive the event: token client %#v, certificate client %#v", webReceived.All(), certReceived.All())
	}
	for _, msg := range webReceived.Get(topic) {
		t.Log("token client received:", topic, msg)
	}
	time.Sleep(settleTime)
	for _, msg := range append(webReceived.Get(topic), certReceived.Get(topic)...) {
		if strings.Contains(msg, "web-client") {
			t.Errorf("a publish of the token client was delivered: %v", msg)
		}
	}
	if len(webReceived.All()) != 1 {
		t.Errorf("token client: unexpected messages %#v", webReceived.All())
	}
	// All groups by topic, so a second delivery of the same event shows only here
	if got := webReceived.Get(topic); len(got) != 1 {
		t.Errorf("token client: expected the event once, got %v deliveries: %#v", len(got), got)
	}

	t.Run("token sessions are renamed and listed", func(t *testing.T) {
		sessions, err := listSessions(config.VmqAdminApiUrl)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sessions, "web:") || !strings.Contains(sessions, ":wclient") {
			t.Errorf("expected the renamed token session in the session list, got %v", sessions)
		}
	})

	// the sweeper runs every 60s; the short lived token has expired by then and its session has to be disconnected
	// while the other one stays
	t.Run("sweeper disconnects expired sessions", func(t *testing.T) {
		// the broker's own view is the proof; whether paho notices the closed connection depends on the
		// keep alive and is only logged
		sessions := ""
		if !waitFor(100*time.Second, func() bool {
			sessions, err = listSessions(config.VmqAdminApiUrl)
			return err == nil && !strings.Contains(sessions, ":wshort")
		}) {
			t.Errorf("the session of the expired token was not disconnected, sessions: %v (%v)", sessions, err)
		}
		if !strings.Contains(sessions, ":wclient") {
			t.Errorf("the session of the valid token is gone, sessions: %v", sessions)
		}
		t.Log("client side of the swept session: connected =", shortLived.IsConnected())
	})
}

// sessionTopics lists the topics the broker holds for the sessions whose client id ends with clientIdSuffix,
// one entry per subscription row.
func sessionTopics(apiUrl string, clientIdSuffix string) (result []string, err error) {
	resp, err := http.Get(apiUrl + "/api/v1/session/show?--client_id&--topic&--mountpoint&--limit=1000000")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	table := struct {
		Table []struct {
			ClientId   string `json:"client_id"`
			Topic      string `json:"topic"`
			Mountpoint string `json:"mountpoint"`
		} `json:"table"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&table)
	if err != nil {
		return nil, err
	}
	for _, row := range table.Table {
		if strings.HasSuffix(row.ClientId, clientIdSuffix) {
			result = append(result, row.Topic)
		}
	}
	return result, nil
}

func listSessions(apiUrl string) (string, error) {
	resp, err := http.Get(apiUrl + "/api/v1/session/show?--client_id&--limit=1000000")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	buf, err := io.ReadAll(resp.Body)
	return string(buf), err
}
