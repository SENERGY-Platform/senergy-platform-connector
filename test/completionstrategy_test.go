package test

import (
	"encoding/json"
	"errors"
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/kafka"
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

func TestCompletionStrategy(t *testing.T) {
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

	deviceTypeId, serviceTopic, err := server.CreateDeviceType(config, config.DeviceManagerUrl)
	if err != nil {
		t.Error(err)
		return
	}

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
	consumermux := sync.Mutex{}
	eventConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", serviceTopic, func(topic string, msg []byte, t time.Time) error {
		consumermux.Lock()
		defer consumermux.Unlock()
		consumedEvents = append(consumedEvents, msg)
		return nil
	}, func(err error, consumer *kafka.Consumer) {
		t.Error(err)
	})
	defer eventConsumer.Stop()

	consumedResponses := [][]byte{}
	responseConsumer, err := kafka.NewConsumer(config.ZookeeperUrl, "test_client", "response", func(topic string, msg []byte, t time.Time) error {
		consumermux.Lock()
		defer consumermux.Unlock()
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

	time.Sleep(3 * time.Second) //wait for creation of devices
	testCommand, err := createTestCommandMsg(config, "test1", "exact", map[string]interface{}{
		"level":      13,
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

	testCommand, err = createOptimisticTestCommandMsg(config, "test1", "exact", map[string]interface{}{
		"level":      9,
		"title":      "level",
		"updateTime": 42,
	})
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

	time.Sleep(30 * time.Second) //wait for command to finish

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
