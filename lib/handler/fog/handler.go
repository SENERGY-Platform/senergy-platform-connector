/*
 * Copyright 2020 InfAI (CC SES)
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

package fog

import (
	"encoding/json"
	"fmt"
	"github.com/SENERGY-Platform/platform-connector-lib/kafka"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"strings"
)

func NewHandler(producer kafka.ProducerInterface, fogTopicPrefix string) *Handler {
	return &Handler{
		producer:                producer,
		fogTopicPrefix:          fogTopicPrefix,
		fogAnalyticsTopicPrefix: fogTopicPrefix + "analytics/",
	}
}

type Handler struct {
	producer                kafka.ProducerInterface
	fogTopicPrefix          string
	fogAnalyticsTopicPrefix string
}

type Result = handler.Result

const (
	Unhandled Result = handler.Unhandled
	Accepted         = handler.Accepted
	Rejected         = handler.Rejected
	Error            = handler.Error
)

//the user param may be used in the future to check auth
func (this *Handler) Subscribe(clientId string, user string, topic string) (result Result, err error) {
	if this == nil {
		return Unhandled, nil
	}
	if !strings.HasPrefix(topic, this.fogTopicPrefix) {
		return Unhandled, nil
	}
	return Accepted, nil
}

//the user param may be used in the future to check auth
func (this *Handler) Publish(clientId string, user string, topic string, payload []byte) (result Result, err error) {
	if this == nil {
		return Unhandled, nil
	}
	if !strings.HasPrefix(topic, this.fogTopicPrefix) {
		return Unhandled, nil
	}
	if !strings.HasPrefix(topic, this.fogAnalyticsTopicPrefix) {
		return Accepted, nil
	}
	target := strings.Replace(topic, this.fogAnalyticsTopicPrefix, "", 1)
	key, err := this.getKey(payload)
	if err != nil {
		return Error, fmt.Errorf("unsupported message format: %w", err)
	}
	err = this.producer.ProduceWithKey(target, string(payload), key)
	if err != nil {
		return Error, err
	}
	return Accepted, nil
}

type KeyWrapper struct {
	Key string `json:"operator_id"`
}

func (this *Handler) getKey(payload []byte) (string, error) {
	wrapper := KeyWrapper{}
	err := json.Unmarshal(payload, &wrapper)
	return wrapper.Key, err
}
