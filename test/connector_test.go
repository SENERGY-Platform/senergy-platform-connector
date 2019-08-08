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
	"encoding/json"
	"errors"
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/iot"
	"github.com/SENERGY-Platform/platform-connector-lib/kafka"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	"github.com/eclipse/paho.mqtt.golang"
	"log"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestErrorSubscription(t *testing.T) {
	config, err := lib.LoadConfig("../config.json")
	if err != nil {
		t.Error(err)
		return
	}
	config, shutdown, err := server.New(config)
	if err != nil {
		t.Error(err)
		return
	}
	if true {
		defer shutdown()
	}

	c, err := client.New(config.MqttBroker, config.IotRepoUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{
		{
			Name:    "test1",
			Uri:     "test1",
			IotType: "iot#80550847-a151-4de4-806a-50503b2fdf62",
			Tags:    []string{},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Stop()

	//kafka consumer to ensure no timouts on webhook because topics had to be created
	eventConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "iot_dc3c326c-8420-4af1-be0d-dcabfdacc90e", func(topic string, msg []byte, t time.Time) error {
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	defer eventConsumer.Stop()
	eventConsumer2, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "event", func(topic string, msg []byte, t time.Time) error {
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	defer eventConsumer2.Stop()

	time.Sleep(5 * time.Second)

	token := c.Mqtt().Subscribe("#", 1, func(i mqtt.Client, message mqtt.Message) {
		t.Error("should never be called", string(message.Payload()))
	})
	if token.Wait() && token.Error() != nil {
		t.Error(token.Error())
		return
	}

	time.Sleep(1 * time.Second)

	log.Println("DEBUG: send event")
	err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(5 * time.Second)
}

func TestErrorPublish(t *testing.T) {
	config, err := lib.LoadConfig("../config.json")
	if err != nil {
		t.Error(err)
		return
	}
	config, shutdown, err := server.New(config)
	if err != nil {
		t.Error(err)
		return
	}
	if true {
		defer shutdown()
	}

	c, err := client.New(config.MqttBroker, config.IotRepoUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{
		{
			Name:    "test1",
			Uri:     "test1",
			IotType: "iot#80550847-a151-4de4-806a-50503b2fdf62",
			Tags:    []string{},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Stop()

	//kafka consumer to ensure no timouts on webhook because topics had to be created
	eventConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "iot_dc3c326c-8420-4af1-be0d-dcabfdacc90e", func(topic string, msg []byte, t time.Time) error {
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	defer eventConsumer.Stop()
	eventConsumer2, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "event", func(topic string, msg []byte, t time.Time) error {
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	defer eventConsumer2.Stop()

	time.Sleep(5 * time.Second)

	log.Println("DEBUG: send event")
	err = c.SendEvent("foo", "bar", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})
	if err != nil {
		return
	}

	t.Error("miss expected error")
}

func TestWithClient(t *testing.T) {

	config, err := lib.LoadConfig("../config.json")
	if err != nil {
		t.Error(err)
		return
	}
	config, shutdown, err := server.New(config)
	if err != nil {
		t.Error(err)
		return
	}
	if true {
		defer shutdown()
	}

	c, err := client.New(config.MqttBroker, config.IotRepoUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{
		{
			Name:    "test1",
			Uri:     "test1",
			IotType: "iot#80550847-a151-4de4-806a-50503b2fdf62",
			Tags:    []string{},
		},
	})
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

	err = kafka.InitTopic(config.ZookeeperUrl, config.KafkaEventTopic)
	if err != nil {
		t.Error(err)
		return
	}

	consumedEvents := [][]byte{}
	eventConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "iot_dc3c326c-8420-4af1-be0d-dcabfdacc90e", func(topic string, msg []byte, t time.Time) error {
		consumedEvents = append(consumedEvents, msg)
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	defer eventConsumer.Stop()

	consumedResponses := [][]byte{}
	responseConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "response", func(topic string, msg []byte, t time.Time) error {
		consumedResponses = append(consumedResponses, msg)
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	defer responseConsumer.Stop()

	err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})

	if err != nil {
		t.Error(err)
		return
	}

	producer, err := kafka.PrepareProducer(config.ZookeeperUrl, true, true)
	if err != nil {
		t.Error(err)
		return
	}
	producer.Log(log.New(os.Stdout, "[TEST-KAFKA] ", 0))

	time.Sleep(2 * time.Second) //wait for creation of devices
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

	time.Sleep(2 * time.Second) //wait for command to finish

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

	time.Sleep(3 * time.Second) //wait for command to finish

	if testState != 9 {
		t.Error("unexpectet command result", testState)
		return
	}

	if len(consumedEvents) != 1 {
		t.Error("unexpectet event result len", len(consumedEvents))
		return
	}

	if len(consumedResponses) != 2 {
		t.Error("unexpectet response result len", len(consumedResponses))
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

	var respResult interface{}
	err = json.Unmarshal(consumedResponses[1], &respResult)
	if err != nil {
		t.Error("unable to unbarshal response msg", string(consumedResponses[1]))
		return
	}

	expectedResponse, err := createTestCommandMsg(config, "test1", "sepl_get", map[string]interface{}{
		"level":      9,
		"title":      "level",
		"updateTime": 42,
	})
	if err != nil {
		t.Error("unable to create expected response", err)
		return
	}
	b, err := json.Marshal(expectedResponse.Value)
	if err != nil {
		t.Error(err)
		return
	}
	var expectedProtocolMsg interface{}
	err = json.Unmarshal(b, &expectedProtocolMsg)
	if err != nil {
		t.Error(err)
		return
	}
	if !reflect.DeepEqual(expectedProtocolMsg, respResult) {
		t.Error("unexpectet response ", string(consumedResponses[1]), "\n\n\n", string(b))
		return
	}
}

func TestWithClientReconnect(t *testing.T) {

	config, err := lib.LoadConfig("../config.json")
	if err != nil {
		t.Error(err)
		return
	}
	config, shutdown, err := server.New(config)
	if err != nil {
		t.Error(err)
		return
	}
	if true {
		defer shutdown()
	}

	c1, err := client.New(config.MqttBroker, config.IotRepoUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{
		{
			Name:    "test1",
			Uri:     "test1",
			IotType: "iot#80550847-a151-4de4-806a-50503b2fdf62",
			Tags:    []string{},
		},
	})
	c1.Stop()
	time.Sleep(1 * time.Second)

	c, err := client.New(config.MqttBroker, config.IotRepoUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", c1.HubId, "testname", []client.DeviceRepresentation{
		{
			Name:    "test1",
			Uri:     "test1",
			IotType: "iot#80550847-a151-4de4-806a-50503b2fdf62",
			Tags:    []string{},
		},
	})
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

	err = kafka.InitTopic(config.ZookeeperUrl, config.KafkaEventTopic)
	if err != nil {
		t.Error(err)
		return
	}

	consumedEvents := [][]byte{}
	eventConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "iot_dc3c326c-8420-4af1-be0d-dcabfdacc90e", func(topic string, msg []byte, t time.Time) error {
		consumedEvents = append(consumedEvents, msg)
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	defer eventConsumer.Stop()

	consumedResponses := [][]byte{}
	responseConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "response", func(topic string, msg []byte, t time.Time) error {
		consumedResponses = append(consumedResponses, msg)
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	defer responseConsumer.Stop()

	err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})

	if err != nil {
		t.Error(err)
		return
	}

	producer, err := kafka.PrepareProducer(config.ZookeeperUrl, true, true)
	if err != nil {
		t.Error(err)
		return
	}
	producer.Log(log.New(os.Stdout, "[TEST-KAFKA] ", 0))

	time.Sleep(2 * time.Second) //wait for creation of devices
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

	time.Sleep(2 * time.Second) //wait for command to finish

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

	time.Sleep(3 * time.Second) //wait for command to finish

	if testState != 9 {
		t.Error("unexpectet command result", testState)
		return
	}

	if len(consumedEvents) != 1 {
		t.Error("unexpectet event result len", len(consumedEvents))
		return
	}

	if len(consumedResponses) != 2 {
		t.Error("unexpectet response result len", len(consumedResponses))
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

	var respResult interface{}
	err = json.Unmarshal(consumedResponses[1], &respResult)
	if err != nil {
		t.Error("unable to unbarshal response msg", string(consumedResponses[1]))
		return
	}

	expectedResponse, err := createTestCommandMsg(config, "test1", "sepl_get", map[string]interface{}{
		"level":      9,
		"title":      "level",
		"updateTime": 42,
	})
	if err != nil {
		t.Error("unable to create expected response", err)
		return
	}
	b, err := json.Marshal(expectedResponse.Value)
	if err != nil {
		t.Error(err)
		return
	}
	var expectedProtocolMsg interface{}
	err = json.Unmarshal(b, &expectedProtocolMsg)
	if err != nil {
		t.Error(err)
		return
	}
	if !reflect.DeepEqual(expectedProtocolMsg, respResult) {
		t.Error("unexpectet response ", string(consumedResponses[1]), "\n\n\n", string(b))
		return
	}
}

func createTestCommandMsg(config lib.Config, deviceUri string, serviceUri string, msg map[string]interface{}) (result model.Envelope, err error) {
	token, err := security.New(config.AuthEndpoint, config.AuthClientId, config.AuthClientSecret, config.JwtIssuer, config.JwtPrivateKey, config.JwtExpiration, config.AuthExpirationTimeBuffer, 0, []string{}).Access()
	if err != nil {
		return result, err
	}
	iot := iot.New(config.IotRepoUrl, config.DeviceRepoUrl, "")
	device, err := iot.DeviceUrlToIotDevice(deviceUri, token)
	if err != nil {
		return result, err
	}
	dt, err := iot.GetDeviceType(device.DeviceType, token)

	found := false
	for _, service := range dt.Services {
		if service.Url == serviceUri {
			found = true
			result.ServiceId = service.Id
			result.DeviceId = device.Id
			value := model.ProtocolMsg{
				Service:          service,
				ServiceId:        service.Id,
				ServiceUrl:       serviceUri,
				DeviceUrl:        deviceUri,
				DeviceInstanceId: device.Id,
				OutputName:       "result",
				TaskId:           "",
				WorkerId:         "",
			}

			if msg != nil {
				payload, err := json.Marshal(msg)
				if err != nil {
					return result, err
				}
				value.ProtocolParts = []model.ProtocolPart{{Value: string(payload), Name: "metrics"}}
			}

			result.Value = value

		}
	}

	if !found {
		err = errors.New("unable to find device for command creation")
	}

	return
}

func TestUnsubscribe(t *testing.T) {

	config, err := lib.LoadConfig("../config.json")
	if err != nil {
		t.Error(err)
		return
	}
	config, shutdown, err := server.New(config)
	if err != nil {
		t.Error(err)
		return
	}
	if true {
		defer shutdown()
	}

	c, err := client.New(config.MqttBroker, config.IotRepoUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{
		{
			Name:    "test1",
			Uri:     "test1",
			IotType: "iot#80550847-a151-4de4-806a-50503b2fdf62",
			Tags:    []string{},
		},
	})
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

	err = kafka.InitTopic(config.ZookeeperUrl, config.KafkaEventTopic)
	if err != nil {
		t.Error(err)
		return
	}

	consumedEvents := [][]byte{}
	eventConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "iot_dc3c326c-8420-4af1-be0d-dcabfdacc90e", func(topic string, msg []byte, t time.Time) error {
		consumedEvents = append(consumedEvents, msg)
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	defer eventConsumer.Stop()

	consumedResponses := [][]byte{}
	responseConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "response", func(topic string, msg []byte, t time.Time) error {
		consumedResponses = append(consumedResponses, msg)
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	defer responseConsumer.Stop()

	err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})

	if err != nil {
		t.Error(err)
		return
	}

	producer, err := kafka.PrepareProducer(config.ZookeeperUrl, true, true)
	if err != nil {
		t.Error(err)
		return
	}
	producer.Log(log.New(os.Stdout, "[TEST-KAFKA] ", 0))

	time.Sleep(2 * time.Second) //wait for creation of devices
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

	time.Sleep(2 * time.Second) //wait for command to finish

	err = c.Unsubscribe("test1", "exact")
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(2 * time.Second)
	testCommand2, err := createTestCommandMsg(config, "test1", "exact", map[string]interface{}{
		"level":      66,
		"title":      "level",
		"updateTime": 42,
	})

	if err != nil {
		t.Error(err)
		return
	}

	testCommandMsg2, err := json.Marshal(testCommand2)
	if err != nil {
		t.Error(err)
		return
	}

	err = producer.Produce(config.Protocol, string(testCommandMsg2))
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(2 * time.Second) //wait for command to finish

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

	time.Sleep(3 * time.Second) //wait for command to finish

	if testState != 9 {
		t.Error("unexpectet command result", testState)
		return
	}

	if len(consumedEvents) != 1 {
		t.Error("unexpectet event result len", len(consumedEvents))
		return
	}

	if len(consumedResponses) != 2 {
		t.Error("unexpectet response result len", len(consumedResponses))
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

	var respResult interface{}
	err = json.Unmarshal(consumedResponses[1], &respResult)
	if err != nil {
		t.Error("unable to unbarshal response msg", string(consumedResponses[1]))
		return
	}

	expectedResponse, err := createTestCommandMsg(config, "test1", "sepl_get", map[string]interface{}{
		"level":      9,
		"title":      "level",
		"updateTime": 42,
	})
	if err != nil {
		t.Error("unable to create expected response", err)
		return
	}
	b, err := json.Marshal(expectedResponse.Value)
	if err != nil {
		t.Error(err)
		return
	}
	var expectedProtocolMsg interface{}
	err = json.Unmarshal(b, &expectedProtocolMsg)
	if err != nil {
		t.Error(err)
		return
	}
	if !reflect.DeepEqual(expectedProtocolMsg, respResult) {
		t.Error("unexpectet response ", string(consumedResponses[1]), "\n\n\n", string(b))
		return
	}
}
