/*
 * Copyright 2021 InfAI (CC SES)
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

package export

import (
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"strings"
)

type Security interface {
	GetUserId(username string) (userid string, err error)
}

func New(security Security) *Handler {
	return &Handler{
		security: security,
	}
}

type Handler struct {
	security Security
}

func (this *Handler) Subscribe(_ string, user string, topic string) (result handler.Result, err error) {
	if !strings.HasPrefix(topic, "notifications") {
		return handler.Unhandled, nil
	}
	parts := strings.Split(topic, "/")
	if len(parts) < 2 {
		return handler.Rejected, nil
	}
	userId, err := this.security.GetUserId(user)
	if err != nil {
		return handler.Error, err
	}
	if userId != parts[1] {
		return handler.Rejected, nil
	}
	return handler.Accepted, nil
}

func (this *Handler) Publish(_ string, _ string, _ string, _ []byte, _ int) (result handler.Result, err error) {
	return handler.Unhandled, nil
}
