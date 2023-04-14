/*
 * Copyright 2023 InfAI (CC SES)
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

package helper

import (
	"context"
	"net/http"
	"time"
)

type HttpTimeoutHandler struct {
	Timeout        time.Duration
	TimeoutHandler func()
	RequestHandler http.Handler
}

func (this *HttpTimeoutHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), this.Timeout)
	go func() {
		defer cancel()
		this.RequestHandler.ServeHTTP(resp, req)
	}()
	<-ctx.Done()
	if ctx.Err() != nil && ctx.Err() != context.Canceled {
		this.TimeoutHandler()
	}
}
