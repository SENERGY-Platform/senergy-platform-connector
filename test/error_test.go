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
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	_ "github.com/lib/pq"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWithErrorClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer time.Sleep(10 * time.Second) //wait for container shutdown
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
	config.PublishToPostgres = true

	notifyCalls := map[string][]string{}
	notifyServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		temp, _ := io.ReadAll(request.Body)
		notifyCalls[request.URL.Path] = append(notifyCalls[request.URL.Path], string(temp))
	}))
	defer notifyServer.Close()
	config.NotificationUrl = notifyServer.URL

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
	deviceTypeId, _, _, _, _, err := createDeviceType(config, config.DeviceManagerUrl, characteristicId)
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
