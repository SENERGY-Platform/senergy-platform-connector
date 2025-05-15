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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/httpcommand"
	"github.com/SENERGY-Platform/platform-connector-lib/kafka"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	_ "github.com/lib/pq"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWithErrorClient(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := configuration.LoadConfig("../config.json")
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
	config.ForceCommandSubscriptionServiceSingleLevelWildcard = false
	config.MutedUserNotificationTitles = nil

	notifyCalls := map[string][]string{}
	notifyServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		temp, _ := io.ReadAll(request.Body)
		notifyCalls[request.URL.Path] = append(notifyCalls[request.URL.Path], string(temp))
	}))
	defer notifyServer.Close()
	config.NotificationUrl = notifyServer.URL

	var brokerUrlForClients string
	config, brokerUrlForClients, err = server.New(ctx, wg, config, client.MQTT4)
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
	deviceTypeId, _, _, _, _, err := createDeviceType(config, config.DeviceManagerUrl, characteristicId)
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
	}, config.MqttAuthMethod, client.MQTT4, client.OwnerInTopicDefault)
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Stop()

	testErrorMessage := "test error message"

	err = c.ListenCommand("test1", "sepl_get", func(request platform_connector_lib.CommandRequestMsg) (response platform_connector_lib.CommandResponseMsg, err error) {
		return response, errors.New(testErrorMessage)
	})
	if err != nil {
		t.Error(err)
		return
	}
	err = c.ListenCommand("test1", "exact", func(request platform_connector_lib.CommandRequestMsg) (response platform_connector_lib.CommandResponseMsg, err error) {
		return response, errors.New(testErrorMessage)
	})
	if err != nil {
		t.Error(err)
		return
	}

	err = c.SendDeviceError("test1", "test device error")
	if err != nil {
		t.Error(err)
		return
	}

	err = c.SendClientError("test client error")
	if err != nil {
		t.Error(err)
		return
	}

	consumedErrors := [][]byte{}
	err = kafka.NewConsumer(ctx, kafka.ConsumerConfig{
		KafkaUrl: config.KafkaUrl,
		GroupId:  "test_client",
		Topic:    "errors",
		MinBytes: 1000,
		MaxBytes: 1000000,
		MaxWait:  100 * time.Millisecond,
	}, func(topic string, msg []byte, t time.Time) error {
		consumedErrors = append(consumedErrors, msg)
		return nil
	}, func(err error) {
		t.Error(err)
	})
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

	producer, err := kafka.PrepareProducer(ctx, config.KafkaUrl, true, true, partitionsNum, replFactor)
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

	testCommand.Metadata.ErrorTo = "errors"

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

	testCommand.Metadata.ErrorTo = "errors"

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

	if len(consumedErrors) != 2 {
		t.Error("unexpected consumedErrors result len", len(consumedErrors))
		return
	}

	for _, consumedError := range consumedErrors {
		var errMsg model.ProtocolMsg
		err := json.Unmarshal(consumedError, &errMsg)
		if err != nil {
			t.Error(err)
			return
		}
		expected := `"` + testErrorMessage + `"`
		actual := errMsg.Response.Output["error"]
		if actual != expected {
			t.Error(actual, expected)
			return
		}

	}

	if len(notifyCalls["/notifications"]) != 4 {
		t.Error(notifyCalls["/notifications"])
		return
	}
	for _, notification := range notifyCalls["/notifications"] {
		if !strings.HasPrefix(notification, `{"userId":"sepl","title":`) {
			t.Error(notification)
			return
		}
	}

}

func TestWithClientMqttErrorOnEventValidationError(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := configuration.LoadConfig("../config.json")
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
	config.MqttErrorOnEventValidationError = true
	config.ForceCommandSubscriptionServiceSingleLevelWildcard = false

	var brokerUrlForClients string
	config, brokerUrlForClients, err = server.New(ctx, wg, config, client.MQTT4)
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
	deviceTypeId, _, getServiceTopic, _, setServiceTopic, err := createDeviceType(config, config.DeviceManagerUrl, characteristicId)
	if err != nil {
		t.Error(err)
		return
	}

	//time.Sleep(10 * time.Second)

	c, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{
		{
			Name:    "test1",
			Uri:     "test1",
			IotType: deviceTypeId,
		},
	}, config.MqttAuthMethod, client.MQTT4, client.OwnerInTopicDefault)
	if err != nil {
		t.Error(err)
		return
	}

	//will later be used for faulty event
	cerr, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{}, config.MqttAuthMethod, client.MQTT4, client.OwnerInTopicDefault)
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
	err = kafka.NewConsumer(ctx, kafka.ConsumerConfig{
		KafkaUrl: config.KafkaUrl,
		GroupId:  "test_client",
		Topic:    getServiceTopic,
		MinBytes: 1000,
		MaxBytes: 1000000,
		MaxWait:  100 * time.Millisecond,
	}, func(topic string, msg []byte, t time.Time) error {
		consumedEvents = append(consumedEvents, msg)
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
		KafkaUrl: config.KafkaUrl,
		GroupId:  "test_client",
		Topic:    "analytics-foo",
		MinBytes: 1000,
		MaxBytes: 1000000,
		MaxWait:  100 * time.Millisecond,
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
		KafkaUrl: config.KafkaUrl,
		GroupId:  "test_client",
		Topic:    setServiceTopic,
		MinBytes: 1000,
		MaxBytes: 1000000,
		MaxWait:  100 * time.Millisecond,
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
		KafkaUrl: config.KafkaUrl,
		GroupId:  "test_client",
		Topic:    "response",
		MinBytes: 1000,
		MaxBytes: 1000000,
		MaxWait:  100 * time.Millisecond,
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

	//expect error (which will not be forwarded to mqtt client)
	_ = cerr.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": "nope", "error_to_expect": "wrong structure and type"}`})

	partitionsNum := 1
	replFactor := 1
	if config.KafkaPartitionNum != 0 {
		partitionsNum = config.KafkaPartitionNum
	}
	if config.KafkaReplicationFactor != 0 {
		replFactor = config.KafkaReplicationFactor
	}

	producer, err := kafka.PrepareProducer(ctx, config.KafkaUrl, true, true, partitionsNum, replFactor)
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

func TestHttpCommandMqttErrorOnEventValidationError(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := configuration.LoadConfig("../config.json")
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
	config.MqttErrorOnEventValidationError = true
	config.ForceCommandSubscriptionServiceSingleLevelWildcard = false

	var brokerUrlForClients string
	config, brokerUrlForClients, err = server.New(ctx, wg, config, client.MQTT4)
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
	deviceTypeId, _, getServiceTopic, _, setServiceTopic, err := createDeviceType(config, config.DeviceManagerUrl, characteristicId)
	if err != nil {
		t.Error(err)
		return
	}

	//time.Sleep(10 * time.Second)

	c, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{
		{
			Name:    "test1",
			Uri:     "test1",
			IotType: deviceTypeId,
		},
	}, config.MqttAuthMethod, client.MQTT4, client.OwnerInTopicDefault)
	if err != nil {
		t.Error(err)
		return
	}

	//will later be used for faulty event
	cerr, err := client.New(brokerUrlForClients, config.DeviceManagerUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{}, config.MqttAuthMethod, client.MQTT4, client.OwnerInTopicDefault)
	if err != nil {
		t.Error(err)
		return
	}

	//time.Sleep(2 * time.Second) //wait for mqtt connection

	defer c.Stop()

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
	err = kafka.NewConsumer(ctx, kafka.ConsumerConfig{
		KafkaUrl: config.KafkaUrl,
		GroupId:  "test_client",
		Topic:    getServiceTopic,
		MinBytes: 1000,
		MaxBytes: 1000000,
		MaxWait:  100 * time.Millisecond,
	}, func(topic string, msg []byte, t time.Time) error {
		consumedEvents = append(consumedEvents, msg)
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
		KafkaUrl: config.KafkaUrl,
		GroupId:  "test_client",
		Topic:    setServiceTopic,
		MinBytes: 1000,
		MaxBytes: 1000000,
		MaxWait:  100 * time.Millisecond,
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
	responsePort, err := server.GetFreePort()
	if err != nil {
		t.Error(err)
		return
	}
	err = httpcommand.StartConsumer(ctx, responsePort, func(msg []byte) error {
		consumedResponses = append(consumedResponses, msg)
		return nil
	})
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
	_ = cerr.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": "nope", "error_to_expect": "wrong structure and type"}`})

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
	testCommand.Metadata.ResponseTo = "http://localhost:" + responsePort + "/commands" // commands path because reuse of the httpcommand package as response receiver

	testCommandMsg, err := json.Marshal(testCommand)
	if err != nil {
		t.Error(err)
		return
	}

	resp, err := http.Post("http://localhost:"+config.HttpCommandConsumerPort+"/commands", "application/json", bytes.NewReader(testCommandMsg))
	if err != nil {
		t.Error(err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		temp, _ := io.ReadAll(resp.Body)
		t.Error(resp.StatusCode, string(temp))
		return
	}

	testCommand, err = createTestCommandMsg(config, "test1", "sepl_get", nil)
	if err != nil {
		t.Error(err)
		return
	}
	testCommand.Metadata.ResponseTo = "http://localhost:" + responsePort + "/commands" // commands path because reuse of the httpcommand package as response receiver

	testCommandMsg, err = json.Marshal(testCommand)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(2 * time.Second)

	resp, err = http.Post("http://localhost:"+config.HttpCommandConsumerPort+"/commands", "application/json", bytes.NewReader(testCommandMsg))
	if err != nil {
		t.Error(err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		temp, _ := io.ReadAll(resp.Body)
		t.Error(resp.StatusCode, string(temp))
		return
	}

	time.Sleep(20 * time.Second) //wait for command to finish

	if testState != 9 {
		t.Error("unexpected command result", testState)
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
	expectedResponse.Metadata.ResponseTo = "http://localhost:" + responsePort + "/commands"
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
