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

package event

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
)

func TestSubscribeOwnerNeedsNoAccessCheck(t *testing.T) {
	check := func(ownerId string, localDeviceId string) (bool, error) {
		t.Error("access check must not be called for the owner")
		return false, nil
	}
	parts := subscribeTopicParts{OwnerId: "owner", LocalDeviceId: "dev", ServiceLocalId: "svc"}
	if r := decideEventSubscribe(slog.Default(), "owner", parts, check); r != handler.Accepted {
		t.Error(r)
	}
}

func TestSubscribeDecision(t *testing.T) {
	boom := errors.New("boom")
	tests := map[string]struct {
		parts    subscribeTopicParts
		sub      string
		allowed  bool
		checkErr error
		expected handler.Result
		calls    int
	}{
		"non owner with access":    {parts: subscribeTopicParts{"owner", "dev", "svc"}, sub: "other", allowed: true, expected: handler.Accepted, calls: 1},
		"non owner without access": {parts: subscribeTopicParts{"owner", "dev", "svc"}, sub: "other", allowed: false, expected: handler.Rejected, calls: 1},
		"check error":              {parts: subscribeTopicParts{"owner", "dev", "svc"}, sub: "other", checkErr: boom, expected: handler.Rejected, calls: 1},
		"service plus":             {parts: subscribeTopicParts{"owner", "dev", "+"}, sub: "other", allowed: true, expected: handler.Accepted, calls: 1},
		"service plus owner":       {parts: subscribeTopicParts{"owner", "dev", "+"}, sub: "owner", expected: handler.Accepted},
		"service hash":             {parts: subscribeTopicParts{"owner", "dev", "#"}, sub: "owner", expected: handler.Rejected},
		"service partial wildcard": {parts: subscribeTopicParts{"owner", "dev", "a+"}, sub: "owner", expected: handler.Rejected},
		"owner plus":               {parts: subscribeTopicParts{"+", "dev", "svc"}, sub: "+", expected: handler.Rejected},
		"owner hash":               {parts: subscribeTopicParts{"#", "dev", "svc"}, sub: "other", allowed: true, expected: handler.Rejected},
		"device plus":              {parts: subscribeTopicParts{"owner", "+", "svc"}, sub: "owner", expected: handler.Rejected},
		"device hash":              {parts: subscribeTopicParts{"owner", "#", "svc"}, sub: "owner", expected: handler.Rejected},
		"empty owner":              {parts: subscribeTopicParts{"", "dev", "svc"}, sub: "", expected: handler.Rejected},
		"empty subscriber":         {parts: subscribeTopicParts{"owner", "dev", "svc"}, sub: "", allowed: true, expected: handler.Rejected},
		"empty device":             {parts: subscribeTopicParts{"owner", "", "svc"}, sub: "owner", expected: handler.Rejected},
		"empty service":            {parts: subscribeTopicParts{"owner", "dev", ""}, sub: "owner", expected: handler.Rejected},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			calls := 0
			r := decideEventSubscribe(slog.Default(), test.sub, test.parts, func(ownerId string, localDeviceId string) (bool, error) {
				calls++
				if ownerId != test.parts.OwnerId || localDeviceId != test.parts.LocalDeviceId {
					t.Error(ownerId, localDeviceId)
				}
				return test.allowed, test.checkErr
			})
			if r != test.expected {
				t.Errorf("expected %v, got %v", test.expected, r)
			}
			if calls != test.calls {
				t.Errorf("expected %v access checks, got %v", test.calls, calls)
			}
		})
	}
}

// topics rejected or ignored before the token is touched; the handler has no connector
func TestSubscribeTopicStructure(t *testing.T) {
	h := &Handler{config: &configuration.ConfigStruct{}}
	tests := map[string]handler.Result{
		"event/#":                     handler.Rejected,
		"event/owner/#":               handler.Rejected,
		"event/dev/svc":               handler.Rejected, //three parts: no owner
		"event":                       handler.Rejected,
		"event/owner/dev/svc/extra":   handler.Rejected,
		"event/+/dev/svc":             handler.Rejected,
		"event/#/dev/svc":             handler.Rejected,
		"event/owner/+/svc":           handler.Rejected,
		"event/owner/dev/#":           handler.Rejected,
		"event/owner/./+":             handler.Rejected,
		"event/owner/../+":            handler.Rejected,
		"event/./dev/+":               handler.Rejected,
		"event/owner/dev/..":          handler.Rejected,
		"eventhandling/owner/dev/svc": handler.Unhandled,
		"eventhandling/foo":           handler.Unhandled,
		"command/owner/dev/svc":       handler.Unhandled,
		"foo/bar":                     handler.Unhandled,
	}
	for topic, expected := range tests {
		t.Run(topic, func(t *testing.T) {
			r, err := h.Subscribe("client", "user", topic)
			if err != nil {
				t.Error("handler errors turn into handler.Error and abort the webhook response:", err)
			}
			if r != expected {
				t.Errorf("expected %v, got %v", expected, r)
			}
		})
	}
}

func TestCanReadDevice(t *testing.T) {
	for _, status := range []int{200, 403, 404, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var mux sync.Mutex
			var path, rawPath, ownerId, auth string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mux.Lock()
				path, rawPath, ownerId, auth = r.URL.Path, r.URL.EscapedPath(), r.URL.Query().Get("owner_id"), r.Header.Get("Authorization")
				mux.Unlock()
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = w.Write([]byte(`{"id":"urn:infai:ses:device:1","local_id":"dev/ice 1","owner_id":"own er&x"}`))
				}
			}))
			defer server.Close()

			allowed, err := canReadDevice(server.URL, security.JwtToken("Bearer token-1"), "own er&x", "dev/ice 1")
			switch status {
			case 200:
				if err != nil || !allowed {
					t.Error(allowed, err)
				}
			case 403, 404:
				if err != nil || allowed {
					t.Error(allowed, err)
				}
			default:
				if err == nil || allowed {
					t.Error("expected error for unexpected status", allowed, err)
				}
			}
			mux.Lock()
			defer mux.Unlock()
			if path != "/local-devices/dev/ice 1" || rawPath != "/local-devices/dev%2Fice%201" {
				t.Error(path, rawPath)
			}
			if ownerId != "own er&x" {
				t.Error(ownerId)
			}
			if auth != "Bearer token-1" {
				t.Error(auth)
			}
		})
	}
}

// device-repository cleans "/local-devices/." to its list route with a redirect; following it would
// read the 200 of the list as access to the device.
func TestCanReadDeviceIgnoresRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/local-devices" {
			http.Redirect(w, r, "/local-devices?"+r.URL.RawQuery, http.StatusMovedPermanently)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	allowed, err := canReadDevice(server.URL, security.JwtToken("Bearer t"), "owner", "dev")
	if allowed {
		t.Error("a redirect must not grant access", err)
	}
}

// A device-repository that ignores owner_id answers with the subscriber's own device of that local id.
func TestCanReadDeviceRequiresTheOwnersDevice(t *testing.T) {
	for name, body := range map[string]string{
		"other owner":    `{"local_id":"dev","owner_id":"subscriber"}`,
		"other local id": `{"local_id":"other","owner_id":"owner"}`,
		"no device":      `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			allowed, _ := canReadDevice(server.URL, security.JwtToken("Bearer t"), "owner", "dev")
			if allowed {
				t.Error("access granted for", body)
			}
		})
	}
}

func TestCanReadDeviceTransportError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()
	allowed, err := canReadDevice(url, security.JwtToken("Bearer t"), "owner", "dev")
	if err == nil || allowed {
		t.Error(allowed, err)
	}
}
