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
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"testing"
	"time"
)

// VerneMQ reuses pooled webhook connections that sat idle for longer than the read timeout.
func TestServerKeepsIdleConnectionPastReadTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := newServer(listener.Addr().String(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	client := &http.Client{Transport: &http.Transport{IdleConnTimeout: time.Minute}}
	url := "http://" + listener.Addr().String() + "/publish"

	get := func() (reused bool) {
		req, err := http.NewRequest(http.MethodPost, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
		}))
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return reused
	}

	get()
	time.Sleep(server.ReadTimeout + time.Second)
	if !get() {
		t.Error("server closed the idle connection before the second request")
	}
}
