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

package test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/kafka"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	"log"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestWithClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer time.Sleep(10 * time.Second) //wait for container shutdown
	defer cancel()

	config, err := lib.LoadConfig("../config.json")
	if err != nil {
		t.Error(err)
		return
	}
	config.Debug = true
	config.FatalKafkaError = false
	config.Validate = true
	config.ValidateAllowUnknownField = true
	config.ValidateAllowMissingField = true
	config.Log = "stdout"
	config.SyncKafka = true
	config.SyncKafkaIdempotent = true

	config, err = server.New(ctx, config)
	if err != nil {
		t.Error(err)
		return
	}

	testCharacteristicName := "test2"

	_, err = createCharacteristic("test1", config)
	if err != nil {
		t.Error(err)
		return
	}
	characteristicId, err := createCharacteristic(testCharacteristicName, config)
	if err != nil {
		t.Error(err)
		return
	}
	deviceTypeId, getServiceTopic, setServiceTopic, err := createDeviceType(config, config.DeviceManagerUrl, characteristicId)
	if err != nil {
		t.Error(err)
		return
	}

	//time.Sleep(10 * time.Second)

	c, err := client.New(config.MqttBroker, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{
		{
			Name:    "test1",
			Uri:     "test1",
			IotType: deviceTypeId,
		},
	})
	if err != nil {
		t.Error(err)
		return
	}

	//will later be used for faulty event
	cerr, err := client.New(config.MqttBroker, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{})
	if err != nil {
		t.Error(err)
		return
	}

	//time.Sleep(2 * time.Second) //wait for mqtt connection

	defer c.Stop()

	/*
		use zway switch multilevel, with services:
		exact: {"level": 0} -> nil
		sepl_get: nil -> {"level": 0, "title": "STRING", "updateTime": 0}

		on protocol "zway-connector" with message-segments: "metrics"
	*/

	var testState float64 = 0
	mux := sync.Mutex{}

	err = c.ListenCommand("test1", "sepl_get", func(request platform_connector_lib.CommandRequestMsg) (response platform_connector_lib.CommandResponseMsg, err error) {
		mux.Lock()
		defer mux.Unlock()
		resp, err := json.Marshal(map[string]interface{}{
			"level":      testState,
			"title":      "level",
			"updateTime": 42,
		})
		if err != nil {
			return response, err
		}
		return platform_connector_lib.CommandResponseMsg{"metrics": string(resp)}, nil
	})
	if err != nil {
		t.Error(err)
		return
	}
	err = c.ListenCommand("test1", "exact", func(request platform_connector_lib.CommandRequestMsg) (response platform_connector_lib.CommandResponseMsg, err error) {
		requestMetrics := map[string]interface{}{}
		err = json.Unmarshal([]byte(request["metrics"]), &requestMetrics)
		if err != nil {
			return response, err
		}
		level, ok := requestMetrics["level"].(float64)
		if !ok {
			return response, errors.New("unable to interpret request message")
		}
		mux.Lock()
		testState = level
		mux.Unlock()
		return platform_connector_lib.CommandResponseMsg{}, nil
	})
	if err != nil {
		t.Error(err)
		return
	}

	consumedEvents := [][]byte{}
	eventConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", getServiceTopic, func(topic string, msg []byte, t time.Time) error {
		consumedEvents = append(consumedEvents, msg)
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	if err != nil {
		t.Error(err)
		return
	}
	defer eventConsumer.Stop()

	consumedAnalytics := [][]byte{}
	analyticConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "analytics-foo", func(topic string, msg []byte, t time.Time) error {
		consumedAnalytics = append(consumedAnalytics, msg)
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	if err != nil {
		t.Error(err)
		return
	}
	defer analyticConsumer.Stop()

	consumedRespEvents := [][]byte{}
	respEventConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", setServiceTopic, func(topic string, msg []byte, t time.Time) error {
		consumedRespEvents = append(consumedRespEvents, msg)
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	if err != nil {
		t.Error(err)
		return
	}
	defer respEventConsumer.Stop()

	consumedResponses := [][]byte{}
	responseConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "response", func(topic string, msg []byte, t time.Time) error {
		consumedResponses = append(consumedResponses, msg)
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	if err != nil {
		t.Error(err)
		return
	}
	defer responseConsumer.Stop()

	err = c.Publish("fog/analytics/analytics-foo", map[string]interface{}{"operator_id": "foo"})
	if err != nil {
		t.Error(err)
		return
	}

	err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})

	if err != nil {
		t.Error(err)
		return
	}

	//expect error
	err = cerr.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": "nope", "error_to_expect": "wrong structure and type"}`})
	if err == nil {
		t.Error("expected error")
		return
	}

	producer, err := kafka.PrepareProducer(config.ZookeeperUrl, true, true)
	if err != nil {
		t.Error(err)
		return
	}
	producer.Log(log.New(os.Stdout, "[TEST-KAFKA] ", 0))

	time.Sleep(5 * time.Second) //wait for creation of devices
	testCommand, err := createTestCommandMsg(config, "test1", "exact", map[string]interface{}{
		"level":      9,
		"title":      "level",
		"updateTime": 42,
	})
	if err != nil {
		t.Error(err)
		return
	}

	testCommandMsg, err := json.Marshal(testCommand)
	if err != nil {
		t.Error(err)
		return
	}

	err = producer.Produce(config.Protocol, string(testCommandMsg))
	if err != nil {
		t.Error(err)
		return
	}

	testCommand, err = createTestCommandMsg(config, "test1", "sepl_get", nil)
	if err != nil {
		t.Error(err)
		return
	}

	testCommandMsg, err = json.Marshal(testCommand)
	if err != nil {
		t.Error(err)
		return
	}

	err = producer.Produce(config.Protocol, string(testCommandMsg))
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(20 * time.Second) //wait for command to finish

	if testState != 9 {
		t.Error("unexpected command result", testState)
		return
	}

	if len(consumedAnalytics) != 1 {
		t.Error("unexpected consumedAnalytics result len", len(consumedAnalytics))
		return
	}

	if len(consumedEvents) != 2 {
		t.Error("unexpected event result len", len(consumedEvents))
		for _, event := range consumedEvents {
			t.Log(string(event))
		}
		return
	}

	if len(consumedResponses) != 2 {
		t.Error("unexpected response result len", len(consumedResponses))
		return
	}

	if len(consumedRespEvents) != 1 {
		t.Error("unexpected response event result len", len(consumedRespEvents))
		return
	}

	type EventTestType struct {
		DeviceId  string                            `json:"device_id"`
		ServiceId string                            `json:"service_id"`
		Value     map[string]map[string]interface{} `json:"value"`
	}
	eventResult := EventTestType{}
	err = json.Unmarshal(consumedEvents[0], &eventResult)
	if err != nil {
		t.Error("unable to unbarshal event msg", err, string(consumedEvents[0]))
		return
	}
	if eventResult.ServiceId == "" || eventResult.DeviceId == "" {
		t.Error("missing envelope values", eventResult, string(consumedEvents[0]))
		return
	}

	if eventResult.Value["metrics"]["level"].(float64) != float64(42) {
		t.Error("unexpected event result", eventResult.Value, string(consumedEvents[0]), reflect.TypeOf(eventResult.Value["metrics"]["level"].(float64)))
		return
	}

	if eventResult.Value["metrics"]["level_unit"].(string) != testCharacteristicName {
		t.Error("unexpected event result", eventResult.Value, string(consumedEvents[0]), reflect.TypeOf(eventResult.Value["metrics"]["level"].(float64)))
		return
	}

	var respResult model.ProtocolMsg
	err = json.Unmarshal(consumedResponses[1], &respResult)
	if err != nil {
		t.Error("unable to unbarshal response msg", string(consumedResponses[1]))
		return
	}
	respResult.Trace = nil

	expectedResponse, err := createTestCommandMsg(config, "test1", "sepl_get", map[string]interface{}{
		"level":      9,
		"title":      "level",
		"updateTime": 42,
	})
	if err != nil {
		t.Error("unable to create expected response", err)
		return
	}
	expectedResponse.Request.Input, expectedResponse.Response.Output = expectedResponse.Response.Output, expectedResponse.Request.Input
	b, err := json.Marshal(expectedResponse)
	if err != nil {
		t.Error(err)
		return
	}
	var expectedProtocolMsg model.ProtocolMsg
	err = json.Unmarshal(b, &expectedProtocolMsg)
	if err != nil {
		t.Error(err)
		return
	}
	if !reflect.DeepEqual(expectedProtocolMsg, respResult) {
		t.Error("unexpected response ", "Got:\n", string(consumedResponses[1]), "\n\n\nExpected:\n", string(b))
		return
	}
}
