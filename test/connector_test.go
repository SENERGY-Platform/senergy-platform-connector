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
	"log"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/SENERGY-Platform/mgw-wmbus-dc/pkg/util"
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/kafka"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func createConf(authentication string) (config configuration.Config, err error) {
	config, err = configuration.LoadConfig("../config.json")
	config.Debug = true
	config.FatalKafkaError = false
	config.Validate = true
	config.ValidateAllowUnknownField = true
	config.ValidateAllowMissingField = true
	config.Log = "stdout"
	config.MqttAuthMethod = authentication
	config.AuthClientId = "connector"
	config.ForceCommandSubscriptionServiceSingleLevelWildcard = false
	config.InitTopics = true
	return config, err
}

func TestMqtt(t *testing.T) {
	t.Skip("collection of test")
	t.Run("TestWithPasswordAuthenticationAtMQTT", TestWithPasswordAuthenticationAtMQTT)
	t.Run("TestWithCertificateAuthenticationAtMQTT", TestWithCertificateAuthenticationAtMQTT)
	t.Run("TestWithPasswordAuthenticationAtMQTT5", TestWithPasswordAuthenticationAtMQTT5)
	t.Run("TestWithCertificateAuthenticationAtMQTT5", TestWithCertificateAuthenticationAtMQTT5)
}

func TestWithPasswordAuthenticationAtMQTT(t *testing.T) {
	authenticationMethod := "password"
	testClient(authenticationMethod, client.MQTT4, t)
}

func TestWithCertificateAuthenticationAtMQTT(t *testing.T) {
	authenticationMethod := "certificate"
	testClient(authenticationMethod, client.MQTT4, t)
}

func TestWithPasswordAuthenticationAtMQTT5(t *testing.T) {
	authenticationMethod := "password"
	testClient(authenticationMethod, client.MQTT5, t)
}

func TestWithCertificateAuthenticationAtMQTT5(t *testing.T) {
	authenticationMethod := "certificate"
	testClient(authenticationMethod, client.MQTT5, t)
}

func testClient(authenticationMethod string, mqttVersion client.MqttVersion, t *testing.T) {
	wg := &sync.WaitGroup{}
	defer t.Log("wg done")
	defer wg.Wait()
	defer t.Log("wait for wg")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	expectedTime := time.Date(2006, time.August, 3, 11, 4, 5, 0, time.UTC)

	config, err := createConf(authenticationMethod)
	if err != nil {
		t.Error(err)
		return
	}

	var brokerUrlForClients string
	config, brokerUrlForClients, err = server.New(ctx, wg, config, mqttVersion)
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
	deviceTypeId, _, getServiceTopic, _, setServiceTopic, err := createDeviceType(config, config.DeviceManagerUrl, characteristicId, nil)
	if err != nil {
		t.Error(err)
		return
	}

	deviceTypeIdNimbus, _, _, _, _, err := createDeviceType(config, config.DeviceManagerUrl, characteristicId, nil)
	if err != nil {
		t.Error(err)
		return
	}
	config.NimbusDeviceTypeId = deviceTypeIdNimbus

	//time.Sleep(10 * time.Second)

	wmbusTypeId, err := util.DeviceTypeId("(TCH) (0x6850)", "type (0x80)", "0x69", uuid.MustParse(config.WmbusDeviceTypeNamespace))
	if err != nil {
		t.Error(err)
		return
	}
	_, _, _, _, _, err = createDeviceType(config, config.DeviceManagerUrl, characteristicId, &wmbusTypeId)
	if err != nil {
		t.Error(err)
		return
	}

	c, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{
		{
			Name:    "test1",
			Uri:     "test1",
			IotType: deviceTypeId,
		},
		{
			Name:    "nimbus",
			Uri:     "nimbus",
			IotType: deviceTypeIdNimbus,
		},
		{
			Name:    "WmbusTest",
			Uri:     "TCH_0x6850_type_0x80_93231141_0x69",
			IotType: wmbusTypeId,
		},
	}, authenticationMethod, mqttVersion, client.OwnerInTopicDefault, func() time.Time { return expectedTime })
	if err != nil {
		t.Error(err)
		return
	}

	//will later be used for faulty event
	cerr, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{}, authenticationMethod, mqttVersion, client.OwnerInTopicDefault, func() time.Time { return expectedTime })
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

	t.Log("listen for commands")
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

	t.Log("start kafka consumer")
	consumedEvents := [][]byte{}
	receivedExpectedTimestampsCount := 0
	receivedOtherTimestampsCount := 0
	err = kafka.NewConsumer(ctx, kafka.ConsumerConfig{
		KafkaUrl:         config.KafkaUrl,
		GroupId:          "test_client",
		Topic:            getServiceTopic,
		MinBytes:         1000,
		MaxBytes:         1000000,
		MaxWait:          100 * time.Millisecond,
		InitTopic:        true,
		AllowOldMessages: true,
	}, func(topic string, msg []byte, timestamp time.Time) error {
		consumedEvents = append(consumedEvents, msg)
		if timestamp != expectedTime {
			t.Log("unexpected timestamp", timestamp, expectedTime)
			receivedOtherTimestampsCount++
		} else {
			receivedExpectedTimestampsCount++
		}
		return nil
	}, func(err error) {
		t.Error(err)
	})
	if err != nil {
		t.Error(err)
		return
	}

	consumedAnalytics := [][]byte{}
	err = kafka.NewConsumer(ctx, kafka.ConsumerConfig{
		KafkaUrl:  config.KafkaUrl,
		GroupId:   "test_client",
		Topic:     "analytics-foo",
		MinBytes:  1000,
		MaxBytes:  1000000,
		MaxWait:   100 * time.Millisecond,
		InitTopic: true,
	}, func(topic string, msg []byte, t time.Time) error {
		consumedAnalytics = append(consumedAnalytics, msg)
		return nil
	}, func(err error) {
		t.Error(err)
	})
	if err != nil {
		t.Error(err)
		return
	}

	consumedRespEvents := [][]byte{}
	err = kafka.NewConsumer(ctx, kafka.ConsumerConfig{
		KafkaUrl:  config.KafkaUrl,
		GroupId:   "test_client",
		Topic:     setServiceTopic,
		MinBytes:  1000,
		MaxBytes:  1000000,
		MaxWait:   100 * time.Millisecond,
		InitTopic: true,
	}, func(topic string, msg []byte, t time.Time) error {
		consumedRespEvents = append(consumedRespEvents, msg)
		return nil
	}, func(err error) {
		t.Error(err)
	})
	if err != nil {
		t.Error(err)
		return
	}

	consumedResponses := [][]byte{}
	err = kafka.NewConsumer(ctx, kafka.ConsumerConfig{
		KafkaUrl:  config.KafkaUrl,
		GroupId:   "test_client",
		Topic:     "response",
		MinBytes:  1000,
		MaxBytes:  1000000,
		MaxWait:   100 * time.Millisecond,
		InitTopic: true,
	}, func(topic string, msg []byte, t time.Time) error {
		consumedResponses = append(consumedResponses, msg)
		return nil
	}, func(err error) {
		t.Error(err)
	})
	if err != nil {
		t.Error(err)
		return
	}

	t.Log("publish events")
	err = c.Publish("fog/analytics/upstream/messages/analytics-foo", map[string]interface{}{"operator_id": "foo"}, 2)
	if err != nil {
		t.Error(err)
		return
	}

	err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})
	if err != nil {
		t.Error(err)
		return
	}

	err = c.SendEvent("nimbus", "encrypted", map[platform_connector_lib.ProtocolSegmentName]string{"data": `{
	"telegram":"32446850411123936980F219A0019F29FA04702FBF02D808DB080000D40D0100000000000000001E348069253E234B472A0000000000000000611B",
	"manufacturer":"(TCH) (0x6850)",
	"meter_id":"93231141",
	"type":"type (0x80)",
	"version":"0x69"
	}`})
	if err != nil {
		t.Error(err)
		return
	}

	//expect error
	err = cerr.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": "nope", "error_to_expect": "wrong structure and type"}`})
	if err != nil {
		t.Error(err)
		return
	}

	partitionsNum := 1
	replFactor := 1
	if config.KafkaPartitionNum != 0 {
		partitionsNum = config.KafkaPartitionNum
	}
	if config.KafkaReplicationFactor != 0 {
		replFactor = config.KafkaReplicationFactor
	}

	t.Log("publish commands")
	producer, err := kafka.PrepareProducer(ctx, config.KafkaUrl, true, true, partitionsNum, replFactor, true)
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

	time.Sleep(2 * time.Second)

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

	time.Sleep(2 * time.Second)

	err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 43, "title": "event", "updateTime": 0}`})
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(20 * time.Second) //wait for command to finish

	t.Log("check state")

	if testState != 9 {
		t.Error("unexpected command result", testState)
	}

	if len(consumedAnalytics) != 1 {
		t.Error("unexpected consumedAnalytics result len", len(consumedAnalytics))
	}

	if len(consumedEvents) != 3 {
		t.Error("unexpected event result len", len(consumedEvents))
		for _, event := range consumedEvents {
			t.Log(string(event))
		}
	}

	if receivedExpectedTimestampsCount != 2 {
		t.Error("unexpected number of expected timestamps", receivedExpectedTimestampsCount)
	}
	if receivedOtherTimestampsCount != 1 {
		t.Error("unexpected number of other timestamps", receivedOtherTimestampsCount)
	}

	if len(consumedResponses) != 2 {
		t.Error("unexpected response result len", len(consumedResponses))
		return
	}

	if len(consumedRespEvents) != 1 {
		t.Error("unexpected response event result len", len(consumedRespEvents))
	}

	type EventTestType struct {
		DeviceId  string                            `json:"device_id"`
		ServiceId string                            `json:"service_id"`
		Value     map[string]map[string]interface{} `json:"value"`
	}
	if len(consumedEvents) == 0 {
		t.Error("no events consumed")
		return
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
		t.Errorf("unexpected response\n%#v\n%#v\n", expectedProtocolMsg, respResult)
		return
	}
}

type testMessagePostgres struct {
	time               time.Time
	metrics_title      string
	metrics_updateTime int
	metrics_level      int
	metrics_level_unit string
	metrics_missing    interface{}
}
