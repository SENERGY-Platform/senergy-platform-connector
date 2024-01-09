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
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/kafka"
	"log"
	"reflect"
	"testing"
)

type MockProducerProvider struct {
	Producer kafka.ProducerInterface
}

func (this MockProducerProvider) GetProducer(qos platform_connector_lib.Qos) (producer kafka.ProducerInterface, err error) {
	return this.Producer, nil
}

func TestHandlerPublish(t *testing.T) {
	km := &KafkaMock{Published: []KafkaMockMessage{}}
	handler := NewHandler(MockProducerProvider{km}, "fog/")

	t.Run(testFogPublish(
		"fog/analytics/foo",
		`{"operator_id":"bar"}`,
		nil,
		km,
		Unhandled,
		[]KafkaMockMessage{},
	))

	t.Run(testFogPublish(
		"somthing/fog/analytics/foo",
		`{"operator_id":"foo"}`,
		handler,
		km,
		Unhandled,
		[]KafkaMockMessage{},
	))

	t.Run(testFogPublish(
		"somthing/fog/analytics/foo",
		``,
		handler,
		km,
		Unhandled,
		[]KafkaMockMessage{},
	))

	t.Run(testFogPublish(
		"fog/foo",
		`{"operator_id":"bar"}`,
		handler,
		km,
		Accepted,
		[]KafkaMockMessage{},
	))

	t.Run(testFogPublish(
		"fog/analytics/foo",
		``,
		handler,
		km,
		Error,
		[]KafkaMockMessage{},
	))

	t.Run(testFogPublish(
		"fog/analytics/foo",
		`{"operator_id":"bar"}`,
		handler,
		km,
		Accepted,
		[]KafkaMockMessage{{
			Topic:   "foo",
			Key:     "bar",
			Message: `{"operator_id":"bar"}`,
		}},
	))
}

func TestHandlerSubscribe(t *testing.T) {
	handler := NewHandler(nil, "fog/")

	t.Run(testFogSubscribe("fog/analytics/nil", nil, Unhandled))
	t.Run(testFogSubscribe("foo/bar", handler, Unhandled))
	t.Run(testFogSubscribe("fog/foo", handler, Accepted))
	t.Run(testFogSubscribe("fog/analytics", handler, Accepted))

}

func testFogSubscribe(topic string, handler *Handler, expectedResult Result) (string, func(t *testing.T)) {
	return topic, func(t *testing.T) {
		result, err := handler.Subscribe("", "", topic)
		if err != nil {
			if expectedResult != Error {
				t.Error(result, err)
				return
			}
			if result != Error {
				t.Error("expect Error result with err", result, err)
				return
			}
		}
		if result != expectedResult {
			t.Error(result, expectedResult)
			return
		}
	}
}

func testFogPublish(topic string, payload string, handler *Handler, km *KafkaMock, expectedResult Result, expectedProduced []KafkaMockMessage) (string, func(t *testing.T)) {
	return topic, func(t *testing.T) {
		startCount := len(km.Published)
		result, err := handler.Publish("", "", topic, []byte(payload), 2, 0)
		if err != nil {
			if expectedResult != Error {
				t.Error(result, err)
				return
			}
			if result != Error {
				t.Error("expect Error result with err", result, err)
				return
			}
		}
		if result != expectedResult {
			t.Error(result, expectedResult)
			return
		}
		produced := km.Published[startCount:]
		if !reflect.DeepEqual(produced, expectedProduced) {
			t.Error(produced, expectedProduced)
			return
		}
	}
}

type KafkaMockMessage struct {
	Topic   string
	Key     string
	Message string
}

type KafkaMock struct {
	Published []KafkaMockMessage
}

func (this *KafkaMock) Produce(topic string, message string) (err error) {
	this.Published = append(this.Published, KafkaMockMessage{
		Topic:   topic,
		Key:     "",
		Message: message,
	})
	return nil
}

func (this *KafkaMock) ProduceWithKey(topic string, message string, key string) (err error) {
	this.Published = append(this.Published, KafkaMockMessage{
		Topic:   topic,
		Key:     key,
		Message: message,
	})
	return nil
}

func (this *KafkaMock) Log(logger *log.Logger) {}

func (this *KafkaMock) Close() {}
