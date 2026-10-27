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

package health

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/kafka"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
)

const testHub = "urn:infai:ses:hub:1"
const testUser = "gateway-user"
const testUserId = "user-id-1"
const testMaxSize = 1024
const testCacheDuration = 10 * time.Minute

type producedMessage struct {
	Topic   string
	Key     string
	Message string
}

type mockProducer struct {
	mux       sync.Mutex
	err       error
	published []producedMessage
}

func (this *mockProducer) Produce(topic string, message string) error {
	return this.ProduceWithKey(topic, message, "")
}

func (this *mockProducer) ProduceWithKey(topic string, message string, key string) error {
	this.mux.Lock()
	defer this.mux.Unlock()
	if this.err != nil {
		return this.err
	}
	this.published = append(this.published, producedMessage{Topic: topic, Key: key, Message: message})
	return nil
}

func (this *mockProducer) ProduceWithTimestamp(topic string, message string, key string, _ time.Time) error {
	return this.ProduceWithKey(topic, message, key)
}

func (this *mockProducer) messages() []producedMessage {
	this.mux.Lock()
	defer this.mux.Unlock()
	return append([]producedMessage{}, this.published...)
}

type mockProducerProvider struct {
	mux      sync.Mutex
	producer *mockProducer
	err      error
	qos      []platform_connector_lib.Qos
}

func (this *mockProducerProvider) GetProducer(qos platform_connector_lib.Qos) (kafka.ProducerInterface, error) {
	this.mux.Lock()
	defer this.mux.Unlock()
	this.qos = append(this.qos, qos)
	if this.err != nil {
		return nil, this.err
	}
	return this.producer, nil
}

type mockTokens struct {
	mux    sync.Mutex
	tokens map[string]security.JwtToken
	err    error
	calls  int
}

func (this *mockTokens) GetCachedUserToken(username string, _ model.RemoteInfo) (security.JwtToken, error) {
	this.mux.Lock()
	defer this.mux.Unlock()
	this.calls++
	if this.err != nil {
		return "", this.err
	}
	token, ok := this.tokens[username]
	if !ok {
		return "", errors.New("unknown user")
	}
	return token, nil
}

type hubCall struct {
	hubId string
	token security.JwtToken
}

type mockHubs struct {
	mux        sync.Mutex
	accessible map[hubCall]bool
	err        error
	calls      []hubCall
}

// ExistsHub mimics iot.ExistsHub, which reports exists=true together with a transport error.
func (this *mockHubs) ExistsHub(id string, cred security.JwtToken) (bool, error) {
	this.mux.Lock()
	defer this.mux.Unlock()
	this.calls = append(this.calls, hubCall{hubId: id, token: cred})
	if this.err != nil {
		return true, this.err
	}
	return this.accessible[hubCall{hubId: id, token: cred}], nil
}

func (this *mockHubs) callCount() int {
	this.mux.Lock()
	defer this.mux.Unlock()
	return len(this.calls)
}

func testToken(claims string) security.JwtToken {
	enc := base64.RawURLEncoding
	return security.JwtToken("Bearer " + enc.EncodeToString([]byte(`{"alg":"none"}`)) + "." + enc.EncodeToString([]byte(claims)) + ".sig")
}

type testEnv struct {
	handler  *Handler
	producer *mockProducer
	provider *mockProducerProvider
	tokens   *mockTokens
	hubs     *mockHubs
	clock    *testClock
}

type testClock struct {
	mux sync.Mutex
	now time.Time
}

func (this *testClock) Now() time.Time {
	this.mux.Lock()
	defer this.mux.Unlock()
	return this.now
}

func (this *testClock) Advance(d time.Duration) {
	this.mux.Lock()
	defer this.mux.Unlock()
	this.now = this.now.Add(d)
}

func newTestEnv(t *testing.T, cacheDuration time.Duration) *testEnv {
	t.Helper()
	userToken := testToken(`{"sub":"` + testUserId + `"}`)
	otherToken := testToken(`{"sub":"other-user-id"}`)
	env := &testEnv{
		producer: &mockProducer{},
		tokens: &mockTokens{tokens: map[string]security.JwtToken{
			testUser:     userToken,
			"other-user": otherToken,
			"no-sub":     testToken(`{"realm_access":{"roles":["user"]}}`),
		}},
		hubs: &mockHubs{accessible: map[hubCall]bool{
			{hubId: testHub, token: userToken}:                                        true,
			{hubId: "urn:infai:ses:hub:2", token: userToken}:                          true,
			{hubId: testHub, token: otherToken}:                                       true,
			{hubId: testHub, token: testToken(`{"realm_access":{"roles":["user"]}}`)}: true,
		}},
		clock: &testClock{now: time.Date(2026, 10, 27, 14, 0, 0, 123456789, time.FixedZone("CET", 3600))},
	}
	env.provider = &mockProducerProvider{producer: env.producer}
	h, err := New(Config{
		Topic:                 "health",
		KafkaTopic:            "gateway_health",
		MaxPayloadSize:        testMaxSize,
		HubCheckCacheDuration: cacheDuration,
	}, env.provider, env.tokens, env.hubs, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.now = env.clock.Now
	env.handler = h
	return env
}

func (this *testEnv) publish(t *testing.T, clientId string, user string, payload string, expected handler.Result) error {
	t.Helper()
	result, err := this.handler.Publish(clientId, user, "health", []byte(payload), 2, float64(len(payload)))
	if result != expected {
		t.Fatalf("expected result %v, got %v (err=%v)", expected, result, err)
	}
	// the vernemqtt publish webhook calls err.Error() for Rejected and Error
	if (result == handler.Rejected || result == handler.Error) && err == nil {
		t.Fatalf("result %v without error", result)
	}
	if (result == handler.Accepted || result == handler.Unhandled) && err != nil {
		t.Fatalf("result %v with error %v", result, err)
	}
	return err
}

func (this *testEnv) expectNothingProduced(t *testing.T) {
	t.Helper()
	if msgs := this.producer.messages(); len(msgs) != 0 {
		t.Fatalf("expected no kafka message, got %v", msgs)
	}
}

const validReport = `{"version":1,"time":"2026-10-27T14:00:00Z","state":2,"reasons":["a"],"core":null,"core_error":"","modules":null,"modules_error":""}`

func TestPublishOtherTopicUnhandled(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	for _, topic := range []string{"", "health/", "health/x", "Health", "xhealth", "event/a/b/c", "fog/health", "$share/g/health"} {
		result, err := env.handler.Publish(testHub, testUser, topic, []byte(validReport), 2, 0)
		if result != handler.Unhandled || err != nil {
			t.Errorf("topic %q: expected Unhandled, got %v %v", topic, result, err)
		}
	}
	if env.tokens.calls != 0 || env.hubs.callCount() != 0 {
		t.Error("unhandled topics must not call dependencies")
	}
	env.expectNothingProduced(t)
}

func TestSubscribe(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	result, err := env.handler.Subscribe(testHub, testUser, "health")
	// a non-nil error would make HandleTopicSubscribe fail the whole subscribe request
	if result != handler.Rejected || err != nil {
		t.Errorf("expected Rejected without error, got %v %v", result, err)
	}
	for _, topic := range []string{"health/x", "#", "+", "event/a/b/c"} {
		result, err = env.handler.Subscribe(testHub, testUser, topic)
		if result != handler.Unhandled || err != nil {
			t.Errorf("topic %q: expected Unhandled, got %v %v", topic, result, err)
		}
	}
}

func TestPublishPayloadSize(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	padded := func(n int) string {
		prefix := `{"x":"`
		suffix := `"}`
		return prefix + strings.Repeat("a", n-len(prefix)-len(suffix)) + suffix
	}
	err := env.publish(t, testHub, testUser, padded(testMaxSize+1), handler.Rejected)
	if !errors.Is(err, ErrReportTooLarge) {
		t.Errorf("unexpected error %v", err)
	}
	if env.tokens.calls != 0 || env.hubs.callCount() != 0 {
		t.Error("oversize report must be rejected before any lookup")
	}
	env.expectNothingProduced(t)

	env.publish(t, testHub, testUser, padded(testMaxSize), handler.Accepted)
	if len(env.producer.messages()) != 1 {
		t.Error("report of exactly the max size must be forwarded")
	}
}

func TestPublishInvalidJson(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	for _, payload := range []string{
		"",
		"   ",
		"not json",
		"[1,2]",
		`"string"`,
		"null",
		"42",
		"{",
		`{"a":1} {"b":2}`,
		`{"a":1},"hub_id":"x"`,
		"{\"a\":\"\xff\"}",
		"\xef\xbb\xbf{}",
	} {
		err := env.publish(t, testHub, testUser, payload, handler.Rejected)
		if !errors.Is(err, ErrInvalidReport) {
			t.Errorf("payload %q: unexpected error %v", payload, err)
		}
	}
	if env.tokens.calls != 0 || env.hubs.callCount() != 0 {
		t.Error("invalid report must be rejected before any lookup")
	}
	env.expectNothingProduced(t)
}

func TestPublishTokenError(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	env.tokens.err = errors.New("keycloak down")
	env.publish(t, testHub, testUser, validReport, handler.Error)
	if env.hubs.callCount() != 0 {
		t.Error("hub check without token")
	}
	env.expectNothingProduced(t)
}

func TestPublishForeignHubRejected(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	for i := 0; i < 2; i++ {
		err := env.publish(t, "urn:infai:ses:hub:foreign", testUser, validReport, handler.Rejected)
		if !errors.Is(err, ErrHubAccessDenied) {
			t.Fatalf("unexpected error %v", err)
		}
	}
	if env.hubs.callCount() != 2 {
		t.Errorf("negative results must not be cached, got %v ExistsHub calls", env.hubs.callCount())
	}
	env.expectNothingProduced(t)
}

func TestPublishInvalidClientIdRejectedWithoutLookup(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	for _, clientId := range []string{"", ".", ".."} {
		err := env.publish(t, clientId, testUser, validReport, handler.Rejected)
		if !errors.Is(err, ErrHubAccessDenied) {
			t.Errorf("client id %q: unexpected error %v", clientId, err)
		}
	}
	if env.hubs.callCount() != 0 {
		t.Error("invalid client ids must not reach ExistsHub")
	}
	env.expectNothingProduced(t)
}

func TestPublishHubCheckErrorNotCached(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	env.hubs.err = errors.New("connection refused")
	env.publish(t, testHub, testUser, validReport, handler.Error)
	env.expectNothingProduced(t)

	env.hubs.err = nil
	env.hubs.accessible = map[hubCall]bool{}
	err := env.publish(t, testHub, testUser, validReport, handler.Rejected)
	if !errors.Is(err, ErrHubAccessDenied) {
		t.Errorf("unexpected error %v", err)
	}
	if env.hubs.callCount() != 2 {
		t.Errorf("an error must not be cached, got %v ExistsHub calls", env.hubs.callCount())
	}
	env.expectNothingProduced(t)
}

func TestPublishAccepted(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	// whitespace and html characters would be changed by json.Marshal of a json.RawMessage
	report := "{\n  \"version\": 1, \"state\": 2,\n  \"reasons\": [\"a < b & c > d\", \" \"],\n  \"core\": {\"x\": [1, 2.50, 1e3]}, \"modules\": null\n}"
	env.publish(t, testHub, testUser, report, handler.Accepted)

	msgs := env.producer.messages()
	if len(msgs) != 1 {
		t.Fatalf("expected exactly one message, got %v", msgs)
	}
	if msgs[0].Topic != "gateway_health" || msgs[0].Key != testHub {
		t.Errorf("unexpected topic/key %q %q", msgs[0].Topic, msgs[0].Key)
	}
	if len(env.provider.qos) != 1 || env.provider.qos[0] != platform_connector_lib.Sync {
		t.Errorf("expected sync producer, got %v", env.provider.qos)
	}
	if !json.Valid([]byte(msgs[0].Message)) {
		t.Fatalf("invalid kafka message %q", msgs[0].Message)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(msgs[0].Message), &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 4 {
		t.Errorf("unexpected fields %v", fields)
	}
	var msg Message
	if err := json.Unmarshal([]byte(msgs[0].Message), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.HubId != testHub || msg.UserId != testUserId {
		t.Errorf("unexpected ids %q %q", msg.HubId, msg.UserId)
	}
	if msg.Time != "2026-10-27T13:00:00.123456789Z" {
		t.Errorf("unexpected time %q", msg.Time)
	}
	if !bytes.Equal(msg.Report, []byte(report)) {
		t.Errorf("report not verbatim:\n%q\n%q", msg.Report, report)
	}
	if !strings.HasPrefix(msgs[0].Message, `{"hub_id":`) || !strings.HasSuffix(msgs[0].Message, `,"report":`+report+`}`) {
		t.Errorf("unexpected layout %q", msgs[0].Message)
	}
}

func TestMessageTimeFixedWidth(t *testing.T) {
	for _, tc := range []struct {
		time     time.Time
		expected string
	}{
		{time.Date(2026, 10, 27, 14, 0, 0, 0, time.UTC), "2026-10-27T14:00:00.000000000Z"},
		{time.Date(2026, 10, 27, 14, 0, 0, 100000000, time.UTC), "2026-10-27T14:00:00.100000000Z"},
		{time.Date(2026, 10, 27, 14, 0, 0, 1, time.FixedZone("x", -7200)), "2026-10-27T16:00:00.000000001Z"},
	} {
		raw, err := encodeMessage(testHub, testUserId, tc.time, []byte(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Time != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, msg.Time)
		}
		parsed, err := time.Parse(time.RFC3339Nano, msg.Time)
		if err != nil || !parsed.Equal(tc.time) {
			t.Errorf("time %q does not parse back to %v: %v", msg.Time, tc.time, err)
		}
	}
}

func TestPublishMissingUserId(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	err := env.publish(t, testHub, "no-sub", validReport, handler.Error)
	if !errors.Is(err, ErrMissingUserId) {
		t.Errorf("unexpected error %v", err)
	}
	env.expectNothingProduced(t)
}

func TestPublishProducerErrors(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	env.provider.err = errors.New("no producer")
	env.publish(t, testHub, testUser, validReport, handler.Error)
	env.provider.err = nil
	env.producer.err = errors.New("kafka down")
	env.publish(t, testHub, testUser, validReport, handler.Error)
	env.expectNothingProduced(t)
}

func TestHubCheckCache(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	env.publish(t, testHub, testUser, validReport, handler.Accepted)
	env.publish(t, testHub, testUser, validReport, handler.Accepted)
	if env.hubs.callCount() != 1 {
		t.Fatalf("expected cache hit, got %v ExistsHub calls", env.hubs.callCount())
	}
	// the cache key is (user, hub), not the hub alone
	env.publish(t, testHub, "other-user", validReport, handler.Accepted)
	env.publish(t, "urn:infai:ses:hub:2", testUser, validReport, handler.Accepted)
	if env.hubs.callCount() != 3 {
		t.Fatalf("expected separate cache entries, got %v ExistsHub calls", env.hubs.callCount())
	}
	env.publish(t, "urn:infai:ses:hub:2", "other-user", validReport, handler.Rejected)
	if env.hubs.callCount() != 4 {
		t.Fatalf("access of one user must not be reused for another, got %v ExistsHub calls", env.hubs.callCount())
	}
	if len(env.producer.messages()) != 4 {
		t.Errorf("expected 4 messages, got %v", len(env.producer.messages()))
	}
}

func TestHubCheckCacheExpiry(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	env.publish(t, testHub, testUser, validReport, handler.Accepted)
	env.clock.Advance(testCacheDuration - time.Nanosecond)
	env.publish(t, testHub, testUser, validReport, handler.Accepted)
	if env.hubs.callCount() != 1 {
		t.Fatalf("entry expired early, got %v ExistsHub calls", env.hubs.callCount())
	}
	env.clock.Advance(time.Nanosecond)
	env.hubs.accessible = map[hubCall]bool{}
	env.publish(t, testHub, testUser, validReport, handler.Rejected)
	if env.hubs.callCount() != 2 {
		t.Fatalf("expected a new check at expiry, got %v ExistsHub calls", env.hubs.callCount())
	}
	env.publish(t, testHub, testUser, validReport, handler.Rejected)
	if env.hubs.callCount() != 3 {
		t.Fatalf("revoked access must not stay cached, got %v ExistsHub calls", env.hubs.callCount())
	}
}

func TestHubCheckCacheDisabled(t *testing.T) {
	env := newTestEnv(t, 0)
	env.publish(t, testHub, testUser, validReport, handler.Accepted)
	env.publish(t, testHub, testUser, validReport, handler.Accepted)
	if env.hubs.callCount() != 2 {
		t.Errorf("expected no caching, got %v ExistsHub calls", env.hubs.callCount())
	}
}

func cacheSize(c *hubCheckCache) int {
	c.mux.Lock()
	defer c.mux.Unlock()
	return len(c.expiry)
}

func TestHubCheckCacheBound(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newHubCheckCache(time.Minute, 3)
	for i, hub := range []string{"a", "b", "c"} {
		c.add(hubCheckKey{user: "u", hubId: hub}, start.Add(time.Duration(i)*time.Second))
	}
	if cacheSize(c) != 3 {
		t.Fatalf("expected 3 entries, got %v", cacheSize(c))
	}
	// refreshing a present key must not drop the others
	c.add(hubCheckKey{user: "u", hubId: "a"}, start.Add(3*time.Second))
	if cacheSize(c) != 3 {
		t.Fatalf("expected 3 entries after refresh, got %v", cacheSize(c))
	}
	c.add(hubCheckKey{user: "u", hubId: "d"}, start.Add(4*time.Second))
	if cacheSize(c) != 1 || !c.has(hubCheckKey{user: "u", hubId: "d"}, start.Add(5*time.Second)) {
		t.Fatalf("expected only the new entry after exceeding the bound, got %v", cacheSize(c))
	}
}

func TestHubCheckCachePrune(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newHubCheckCache(time.Minute, 100)
	c.add(hubCheckKey{user: "u", hubId: "a"}, start)
	c.add(hubCheckKey{user: "u", hubId: "b"}, start.Add(30*time.Second))
	c.add(hubCheckKey{user: "u", hubId: "c"}, start.Add(time.Minute))
	// at start+1m the first prune is due: "a" has expired, "b" has not
	if cacheSize(c) != 2 || c.has(hubCheckKey{user: "u", hubId: "a"}, start) {
		t.Fatalf("expected expired entry to be pruned, got %v entries", cacheSize(c))
	}
	// at start+2m b and c have expired as well
	c.add(hubCheckKey{user: "u", hubId: "d"}, start.Add(2*time.Minute))
	if cacheSize(c) != 1 {
		t.Fatalf("expected b and c pruned, got %v entries", cacheSize(c))
	}
}

func TestPublishConcurrent(t *testing.T) {
	env := newTestEnv(t, testCacheDuration)
	const workers = 32
	const perWorker = 50
	wg := sync.WaitGroup{}
	failures := make(chan string, workers*perWorker)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				hub, user, expected := testHub, testUser, handler.Accepted
				if (w+i)%3 == 0 {
					hub, expected = "urn:infai:ses:hub:foreign", handler.Rejected
				}
				if i%10 == 0 {
					env.clock.Advance(time.Minute)
				}
				result, err := env.handler.Publish(hub, user, "health", []byte(validReport), 2, 0)
				if result != expected {
					failures <- hub + ": " + errorString(err)
				}
			}
		}(w)
	}
	wg.Wait()
	close(failures)
	for f := range failures {
		t.Error(f)
	}
	for _, msg := range env.producer.messages() {
		if msg.Key != testHub {
			t.Fatalf("message for foreign hub produced: %v", msg)
		}
	}
}

func errorString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

func TestNewFromConfig(t *testing.T) {
	valid := func() configuration.Config {
		return &configuration.ConfigStruct{
			HealthTopic:                 "health",
			GatewayHealthTopic:          "gateway_health",
			HealthHubCheckCacheDuration: "10m",
			HealthMaxPayloadSize:        65536,
		}
	}
	h, err := NewFromConfig(valid(), &mockProducerProvider{}, &mockTokens{}, &mockHubs{})
	if err != nil {
		t.Fatal(err)
	}
	if h.config.HubCheckCacheDuration != 10*time.Minute || h.config.MaxPayloadSize != 65536 || h.config.KafkaTopic != "gateway_health" {
		t.Errorf("unexpected config %+v", h.config)
	}
	for name, mutate := range map[string]func(c configuration.Config){
		"invalid duration": func(c configuration.Config) { c.HealthHubCheckCacheDuration = "10" },
		"empty duration":   func(c configuration.Config) { c.HealthHubCheckCacheDuration = "" },
		"zero max size":    func(c configuration.Config) { c.HealthMaxPayloadSize = 0 },
		"negative size":    func(c configuration.Config) { c.HealthMaxPayloadSize = -1 },
		"no kafka topic":   func(c configuration.Config) { c.GatewayHealthTopic = "-" },
		"empty kafka":      func(c configuration.Config) { c.GatewayHealthTopic = "" },
		"no mqtt topic":    func(c configuration.Config) { c.HealthTopic = "-" },
	} {
		c := valid()
		mutate(c)
		if _, err := NewFromConfig(c, &mockProducerProvider{}, &mockTokens{}, &mockHubs{}); err == nil {
			t.Errorf("%v: expected error", name)
		}
	}
}

func TestEnabled(t *testing.T) {
	for topic, expected := range map[string]bool{"": false, "-": false, "health": true} {
		if Enabled(topic) != expected {
			t.Errorf("Enabled(%q) != %v", topic, expected)
		}
	}
}
