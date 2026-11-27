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

package vernemqtt

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server/mock/jwks"
	"github.com/SENERGY-Platform/service-commons/pkg/jwt"
)

type tokenLoginEnv struct {
	keys   *jwks.Keys
	certs  *jwt.KeycloakCertProvider
	config configuration.Config
}

func newTokenLoginEnv(t *testing.T) *tokenLoginEnv {
	keys, err := jwks.New("kid1")
	if err != nil {
		t.Fatal(err)
	}
	server := keys.Serve()
	t.Cleanup(server.Close)
	return &tokenLoginEnv{
		keys:  keys,
		certs: &jwt.KeycloakCertProvider{CertUrl: server.URL},
		config: &configuration.ConfigStruct{
			TokenLoginMountpoint: "web",
			KeycloakCertUrl:      server.URL,
			MqttAuthMethod:       "certificate",
			ForceCleanSession:    true,
			AuthClientId:         "connector",
			AuthClientSecret:     "secret",
		},
	}
}

func (this *tokenLoginEnv) token(t *testing.T, keys *jwks.Keys, sub string, exp time.Time) string {
	claims := map[string]interface{}{"sub": sub, "typ": "Bearer"}
	if !exp.IsZero() {
		claims["exp"] = exp.Unix()
	}
	token, err := keys.Sign(claims)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func callLogin(env *tokenLoginEnv, certs *jwt.KeycloakCertProvider, msg LoginWebhookMsg) map[string]interface{} {
	body, _ := json.Marshal(msg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	login(rec, req, env.config, nil, nil, certs, env.config.GetLogger())
	result := map[string]interface{}{}
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	return result
}

func isOk(result map[string]interface{}) bool {
	return result["result"] == "ok"
}

func TestTokenLogin(t *testing.T) {
	env := newTokenLoginEnv(t)
	otherKeys, err := jwks.New("kid1") // same kid, other key: the signature must not verify
	if err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour)
	validToken := env.token(t, env.keys, "user-a", exp)

	base := LoginWebhookMsg{Mountpoint: "web", ClientId: "wclient", Username: "user-a", CleanSession: true}
	with := func(f func(m *LoginWebhookMsg)) LoginWebhookMsg {
		m := base
		f(&m)
		return m
	}

	t.Run("ok", func(t *testing.T) {
		result := callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Password = validToken }))
		if !isOk(result) {
			t.Fatal(result)
		}
		modifiers, _ := result["modifiers"].(map[string]interface{})
		if mp, present := modifiers["mountpoint"]; !present || mp != "" {
			t.Errorf("expected empty mountpoint modifier, got %#v", modifiers)
		}
		if modifiers["client_id"] != tokenSessionClientId(exp, "wclient") {
			t.Errorf("unexpected client id modifier %#v", modifiers)
		}
		if !strings.HasPrefix(modifiers["client_id"].(string), "web:") || !strings.HasSuffix(modifiers["client_id"].(string), ":wclient") {
			t.Errorf("unexpected client id modifier %#v", modifiers)
		}
	})
	t.Run("bearer prefix", func(t *testing.T) {
		result := callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Password = "Bearer " + validToken }))
		if !isOk(result) {
			t.Fatal(result)
		}
	})
	t.Run("no password", func(t *testing.T) {
		if isOk(callLogin(env, env.certs, base)) {
			t.Fatal("expected refusal")
		}
	})
	t.Run("garbage password", func(t *testing.T) {
		if isOk(callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Password = "garbage" }))) {
			t.Fatal("expected refusal")
		}
	})
	t.Run("wrong sub", func(t *testing.T) {
		token := env.token(t, env.keys, "user-b", exp)
		if isOk(callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Password = token }))) {
			t.Fatal("expected refusal")
		}
	})
	t.Run("expired", func(t *testing.T) {
		token := env.token(t, env.keys, "user-a", time.Now().Add(-time.Minute))
		if isOk(callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Password = token }))) {
			t.Fatal("expected refusal")
		}
	})
	t.Run("no expiry", func(t *testing.T) {
		token := env.token(t, env.keys, "user-a", time.Time{})
		if isOk(callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Password = token }))) {
			t.Fatal("expected refusal")
		}
	})
	t.Run("bad signature", func(t *testing.T) {
		token := env.token(t, otherKeys, "user-a", exp)
		if isOk(callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Password = token }))) {
			t.Fatal("expected refusal")
		}
	})
	t.Run("unsigned token", func(t *testing.T) {
		parts := strings.Split(validToken, ".")
		token := parts[0] + "." + parts[1] + "."
		if isOk(callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Password = token }))) {
			t.Fatal("expected refusal")
		}
	})
	t.Run("cert url empty", func(t *testing.T) {
		config := *env.config
		config.KeycloakCertUrl = ""
		empty := &tokenLoginEnv{keys: env.keys, config: &config}
		if isOk(callLogin(empty, nil, with(func(m *LoginWebhookMsg) { m.Password = validToken }))) {
			t.Fatal("expected refusal")
		}
	})
	t.Run("persistent session", func(t *testing.T) {
		result := callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Password = validToken; m.CleanSession = false }))
		if isOk(result) {
			t.Fatal("expected refusal")
		}
	})
	t.Run("persistent session without forced clean session", func(t *testing.T) {
		env.config.ForceCleanSession = false
		env.config.CleanSessionAllowUserList = []string{"user-a"}
		defer func() {
			env.config.ForceCleanSession = true
			env.config.CleanSessionAllowUserList = nil
		}()
		result := callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Password = validToken; m.CleanSession = false }))
		if isOk(result) {
			t.Fatal("expected refusal: a token session is always clean")
		}
	})
	t.Run("password is not logged", func(t *testing.T) {
		// the refusal answer carries neither token nor password
		result := callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Password = env.token(t, env.keys, "user-b", exp) }))
		buf, _ := json.Marshal(result)
		if strings.Contains(string(buf), "eyJ") {
			t.Errorf("answer contains a token: %s", buf)
		}
	})
	t.Run("other mountpoint", func(t *testing.T) {
		if isOk(callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Mountpoint = "other"; m.Password = validToken }))) {
			t.Fatal("expected refusal")
		}
	})
	t.Run("mountpoint without token login configured", func(t *testing.T) {
		config := *env.config
		config.TokenLoginMountpoint = ""
		disabled := &tokenLoginEnv{keys: env.keys, config: &config}
		if isOk(callLogin(disabled, env.certs, with(func(m *LoginWebhookMsg) { m.Password = validToken }))) {
			t.Fatal("expected refusal")
		}
	})
	t.Run("reserved client id on the empty mountpoint", func(t *testing.T) {
		if isOk(callLogin(env, env.certs, with(func(m *LoginWebhookMsg) { m.Mountpoint = ""; m.ClientId = "web:4102444800:x" }))) {
			t.Fatal("expected refusal")
		}
		// the connector itself is no exception
		if isOk(callLogin(env, env.certs, with(func(m *LoginWebhookMsg) {
			m.Mountpoint = ""
			m.ClientId = "web:4102444800:x"
			m.Username = "connector"
			m.Password = "secret"
		}))) {
			t.Fatal("expected refusal")
		}
	})
	t.Run("existing behaviour on the empty mountpoint", func(t *testing.T) {
		// the connector's own login, which needs no connector or token exchange
		env.config.MqttAuthMethod = "password"
		defer func() { env.config.MqttAuthMethod = "certificate" }()
		result := callLogin(env, env.certs, LoginWebhookMsg{ClientId: "connector-client", Username: "connector", Password: "secret", CleanSession: true})
		if !isOk(result) || result["modifiers"] != nil {
			t.Errorf("expected plain ok, got %#v", result)
		}
		if isOk(callLogin(env, env.certs, LoginWebhookMsg{ClientId: "connector-client", Username: "connector", Password: "wrong", CleanSession: true})) {
			t.Error("expected refusal of wrong connector secret")
		}
		// the clean session rule still applies
		if isOk(callLogin(env, env.certs, LoginWebhookMsg{ClientId: "hub", Username: "someone", Password: "x"})) {
			t.Error("expected refusal of persistent session")
		}
	})
}

func callPublish(env *tokenLoginEnv, msg PublishWebhookMsg) map[string]interface{} {
	body, _ := json.Marshal(msg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/publish", bytes.NewReader(body))
	publish(rec, req, env.config, nil, nil)
	result := map[string]interface{}{}
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	return result
}

func TestPublishRefusesTokenSessions(t *testing.T) {
	env := newTokenLoginEnv(t)
	result := callPublish(env, PublishWebhookMsg{Username: "user-a", ClientId: "web:4102444800:wclient", Topic: "event/user-a/d/s", Payload: "e30="})
	if isOk(result) || result["result"] == nil {
		t.Errorf("expected error answer, got %#v", result)
	}
	// even the connector's own username does not bypass it
	result = callPublish(env, PublishWebhookMsg{Username: "connector", ClientId: "web:4102444800:wclient", Topic: "event/a/b/c"})
	if isOk(result) {
		t.Errorf("expected error answer, got %#v", result)
	}
	// other client ids are handled as before (connector username is answered with ok)
	result = callPublish(env, PublishWebhookMsg{Username: "connector", ClientId: "client", Topic: "event/a/b/c"})
	if !isOk(result) {
		t.Errorf("expected ok, got %#v", result)
	}
}

type acceptAll struct{}

func (acceptAll) Subscribe(clientId string, user string, topic string) (handler.Result, error) {
	return handler.Accepted, nil
}
func (acceptAll) Publish(clientId string, user string, topic string, payload []byte, qos int, size float64) (handler.Result, error) {
	return handler.Accepted, nil
}

func TestSubscribeRestrictsTokenSessions(t *testing.T) {
	env := newTokenLoginEnv(t)
	topics := []string{"event/user-a/d/s", "command/user-a/d/s", "#", "$share/g/event/user-a/d/s", "$share/g/command/u/d/s", "events/x", "/event/x", "", "+/user-a/d/s", "$SYS/#"}
	msg := SubscribeWebhookMsg{Username: "user-a"}
	for _, topic := range topics {
		msg.Topics = append(msg.Topics, WebhookmsgTopic{Topic: topic, Qos: 1})
	}
	call := func(clientId string) map[string]int64 {
		msg.ClientId = clientId
		body, _ := json.Marshal(msg)
		rec := httptest.NewRecorder()
		subscribe(rec, httptest.NewRequest(http.MethodPost, "/subscribe", bytes.NewReader(body)), env.config, []handler.Handler{acceptAll{}})
		resp := SubscriptionResponse{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err, rec.Body.String())
		}
		result := map[string]int64{}
		for _, topic := range resp.Topics {
			result[topic.Topic] = topic.Qos
		}
		return result
	}

	web := call("web:4102444800:wclient")
	for topic, qos := range web {
		expectAccepted := topic == "event/user-a/d/s"
		if expectAccepted && qos != 1 {
			t.Errorf("token session: expected %q to be accepted, got qos %v", topic, qos)
		}
		if !expectAccepted && qos != 128 {
			t.Errorf("token session: expected %q to be rejected, got qos %v", topic, qos)
		}
	}
	if len(web) != len(topics) {
		t.Errorf("expected an answer per topic, got %#v", web)
	}

	// everybody else is not restricted by this rule
	other := call("hub-1")
	for topic, qos := range other {
		if qos != 1 {
			t.Errorf("other client: expected %q to be accepted, got qos %v", topic, qos)
		}
	}
}

type fakeAdminApi struct {
	mux          sync.Mutex
	sessions     []string
	disconnected []string
	failShow     bool
	listQuery    string
}

func (this *fakeAdminApi) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/session/show", func(writer http.ResponseWriter, request *http.Request) {
		this.mux.Lock()
		defer this.mux.Unlock()
		this.listQuery = request.URL.RawQuery
		if this.failShow {
			http.Error(writer, "boom", http.StatusInternalServerError)
			return
		}
		table := []map[string]interface{}{}
		for _, id := range this.sessions {
			table = append(table, map[string]interface{}{"client_id": id})
		}
		_ = json.NewEncoder(writer).Encode(map[string]interface{}{"table": table})
	})
	mux.HandleFunc("/api/v1/session/disconnect", func(writer http.ResponseWriter, request *http.Request) {
		this.mux.Lock()
		defer this.mux.Unlock()
		this.disconnected = append(this.disconnected, request.URL.Query().Get("client-id"))
		_ = json.NewEncoder(writer).Encode(map[string]interface{}{"text": "ok"})
	})
	return mux
}

func (this *fakeAdminApi) Disconnected() []string {
	this.mux.Lock()
	defer this.mux.Unlock()
	return append([]string{}, this.disconnected...)
}

func TestSweepTokenSessions(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	api := &fakeAdminApi{sessions: []string{
		"web:1799999999:expired",
		"web:1800000000:exactly-now",
		"web:1800000001:valid",
		"web:1900000000:web:nested:id",
		"web:garbage:bad-exp",
		"web:noexpiry",
		"hub-1",
		"mywebclient",
		"web:1000000000:x:y:z",
	}}
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	config := &configuration.ConfigStruct{}
	if err := sweepTokenSessions(server.URL, now, config.GetLogger()); err != nil {
		t.Fatal(err)
	}
	expected := []string{"web:1799999999:expired", "web:1800000000:exactly-now", "web:1000000000:x:y:z"}
	got := api.Disconnected()
	if strings.Join(got, "|") != strings.Join(expected, "|") {
		t.Errorf("expected %v, got %v", expected, got)
	}
	if !strings.Contains(api.listQuery, "--limit=") {
		t.Errorf("expected a session limit in the list request, got %q", api.listQuery)
	}

	api.failShow = true
	if err := sweepTokenSessions(server.URL, now, config.GetLogger()); err == nil {
		t.Error("expected an error if the sessions can not be listed")
	}
}

func TestSweeperStart(t *testing.T) {
	api := &fakeAdminApi{sessions: []string{"web:1:expired", "web:4102444800:valid"}}
	server := httptest.NewServer(api.Handler())
	defer server.Close()

	old := tokenSessionSweepInterval
	tokenSessionSweepInterval = 20 * time.Millisecond
	defer func() { tokenSessionSweepInterval = old }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartTokenSessionSweeper(ctx, &configuration.ConfigStruct{TokenLoginMountpoint: "web", VmqAdminApiUrl: server.URL})

	deadline := time.Now().Add(5 * time.Second)
	for len(api.Disconnected()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := api.Disconnected(); len(got) == 0 || got[0] != "web:1:expired" {
		t.Errorf("unexpected disconnects %v", got)
	}
	for _, id := range api.Disconnected() {
		if id != "web:1:expired" {
			t.Errorf("unexpected disconnect %v", id)
		}
	}
	cancel()

	// no admin api, token login disabled: nothing starts
	for _, config := range []*configuration.ConfigStruct{
		{TokenLoginMountpoint: "web", VmqAdminApiUrl: ""},
		{TokenLoginMountpoint: "web", VmqAdminApiUrl: "-"},
		{TokenLoginMountpoint: "", VmqAdminApiUrl: server.URL},
	} {
		before := len(api.Disconnected())
		ctx2, cancel2 := context.WithCancel(context.Background())
		StartTokenSessionSweeper(ctx2, config)
		time.Sleep(100 * time.Millisecond)
		cancel2()
		if len(api.Disconnected()) != before {
			t.Errorf("sweeper must not run for %#v", config)
		}
	}
}

func TestDisconnectCommandListUnchanged(t *testing.T) {
	api := &fakeAdminApi{sessions: []string{"a", "b"}}
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	ids, err := listSessionsClientIdsOfUser(server.URL, "us er")
	if err != nil || len(ids) != 2 {
		t.Fatal(ids, err)
	}
	if api.listQuery != "--user=us+er" {
		t.Errorf("unexpected query %q", api.listQuery)
	}
}

func TestTokenSessionUnsubscribeLogsNoDisconnect(t *testing.T) {
	config := &configuration.ConfigStruct{AuthClientId: "connector"}
	body, _ := json.Marshal(UnsubscribeWebhookMsg{
		Username: "user-a",
		ClientId: tokenSessionClientId(time.Now().Add(time.Hour), "wclient"),
		Topics:   []string{"command/dev1/svc1"},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/unsubscribe", bytes.NewReader(body))
	// without connector and connection logger: a token session must not reach either of them
	unsubscribe(rec, req, config, nil, nil)
	result := map[string]interface{}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err, rec.Body.String())
	}
	if result["result"] != "ok" {
		t.Fatalf("unexpected answer %v", rec.Body.String())
	}
}
