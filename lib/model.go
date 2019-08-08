/*
 * Copyright 2019 InfAI (CC SES)
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

package lib

import "github.com/SENERGY-Platform/platform-connector-lib"

type RequestEnvelope struct {
	CorrelationId      string                                   `json:"correlation_id"`
	Payload            platform_connector_lib.CommandRequestMsg `json:"payload"`
	Time               int64                                    `json:"timestamp"`
	CompletionStrategy string                                   `json:"completion_strategy"`
}

type ResponseEnvelope struct {
	CorrelationId string                                    `json:"correlation_id"`
	Payload       platform_connector_lib.CommandResponseMsg `json:"payload"`
}

type PublishWebhookMsg struct {
	Username string `json:"username"`
	ClientId string `json:"client_id"`
	Topic    string `json:"topic"`
	Payload  string `json:"payload"`
}

type WebhookmsgTopic struct {
	Topic string `json:"topic"`
	Qos   int64  `json:"qos"`
}

type SubscribeWebhookMsg struct {
	Username string            `json:"username"`
	ClientId string            `json:"client_id"`
	Topics   []WebhookmsgTopic `json:"topics"`
}

type UnsubscribeWebhookMsg struct {
	Username string   `json:"username"`
	Topics   []string `json:"topics"`
}

type LoginWebhookMsg struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	ClientId     string `json:"client_id"`
	CleanSession bool   `json:"clean_session"`
}

type DisconnectWebhookMsg struct {
	ClientId string `json:"client_id"`
}

type EventHandler func(username string, topic string, payload string)
