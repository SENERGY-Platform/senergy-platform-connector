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
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/SENERGY-Platform/converter/lib/converter/characteristics"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"

	"reflect"
	"sync"
	"testing"
	"time"

	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/kafka"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	_ "github.com/lib/pq"
)

func TestMultipleProtocolSegments(t *testing.T) {
	authenticationMethod := "password"
	mqttVersion := client.MQTT4

	wg := &sync.WaitGroup{}
	defer t.Log("wg done")
	defer wg.Wait()
	defer t.Log("wait for wg")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	psqlconn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", config.PostgresHost,
		config.PostgresPort, config.PostgresUser, config.PostgresPw, config.PostgresDb)

	// open database
	db, err := sql.Open("postgres", psqlconn)
	if err != nil {
		t.Fatal("could not establish db")
	}
	err = db.Ping()
	if err != nil {
		t.Fatal("could not connect to db")
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
	deviceTypeId, _, serviceTopic, err := createMultiProtocolSegmentDeviceType(config, config.DeviceManagerUrl, characteristicId)
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
	}, authenticationMethod, mqttVersion, config.TopicsWithOwner)
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Stop()

	t.Log("start kafka consumer")
	consumedEvents := [][]byte{}
	err = kafka.NewConsumer(ctx, kafka.ConsumerConfig{
		KafkaUrl: config.KafkaUrl,
		GroupId:  "test_client",
		Topic:    serviceTopic,
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

	t.Log("publish events")
	err = c.Publish("fog/analytics/upstream/messages/analytics-foo", map[string]interface{}{"operator_id": "foo"}, 2)
	if err != nil {
		t.Error(err)
		return
	}

	err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`, "other_seg": "foo"})
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(20 * time.Second)

	t.Log("check state")

	if len(consumedEvents) != 1 {
		t.Error("unexpected event result len", len(consumedEvents))
		for _, event := range consumedEvents {
			t.Log(string(event))
		}
		return
	}

	type EventTestType struct {
		DeviceId  string `json:"device_id"`
		ServiceId string `json:"service_id"`
		Value     struct {
			Metrics  map[string]interface{} `json:"metrics"`
			OtherVar string                 `json:"other_var"`
		} `json:"value"`
	}
	eventResult := EventTestType{}
	err = json.Unmarshal(consumedEvents[0], &eventResult)
	if err != nil {
		t.Error("unable to unmarshal event msg", err, string(consumedEvents[0]))
		return
	}
	if eventResult.ServiceId == "" || eventResult.DeviceId == "" {
		t.Error("missing envelope values", eventResult, string(consumedEvents[0]))
		return
	}

	if eventResult.Value.Metrics["level"].(float64) != float64(42) {
		t.Error("unexpected event result", eventResult.Value, string(consumedEvents[0]), reflect.TypeOf(eventResult.Value.Metrics["level"].(float64)))
		return
	}

	if eventResult.Value.Metrics["level_unit"].(string) != testCharacteristicName {
		t.Error("unexpected event result", eventResult.Value, string(consumedEvents[0]), reflect.TypeOf(eventResult.Value.Metrics["level"].(float64)))
		return
	}

	t.Logf("%#v", eventResult.Value)
	if eventResult.Value.OtherVar != "foo" {
		t.Error("unexpected event result", eventResult.Value, string(consumedEvents[0]), reflect.TypeOf(eventResult.Value.OtherVar))
		return
	}
}

func createMultiProtocolSegmentDeviceType(conf configuration.Config, managerUrl string, characteristicId string) (typeId string, serviceId string, serviceTopic string, err error) {
	protocol := model.Protocol{}
	err = adminjwt.PostJSON(managerUrl+"/protocols", model.Protocol{
		Name:             conf.Protocol,
		Handler:          conf.Protocol,
		ProtocolSegments: []model.ProtocolSegment{{Name: "metrics"}, {Name: "other_seg"}},
	}, &protocol)
	if err != nil {
		return "", "", "", err
	}

	time.Sleep(10 * time.Second)

	devicetype := model.DeviceType{}
	err = adminjwt.PostJSON(managerUrl+"/device-types", model.DeviceType{
		Name: "foo",
		Services: []model.Service{
			{
				Name:        "sepl_get",
				LocalId:     "sepl_get",
				Description: "sepl_get",
				ProtocolId:  protocol.Id,
				Attributes: []model.Attribute{
					{
						Key:   "senergy/time_path",
						Value: "metrics.updateTime",
					},
				},
				Outputs: []model.Content{
					{
						ProtocolSegmentId: protocol.ProtocolSegments[0].Id,
						Serialization:     "json",
						ContentVariable: model.ContentVariable{
							Name: "metrics",
							Type: model.Structure,
							SubContentVariables: []model.ContentVariable{
								{
									Name:             "updateTime",
									Type:             model.Integer,
									CharacteristicId: characteristics.UnixSeconds,
								},
								{
									Name:             "level",
									Type:             model.Integer,
									CharacteristicId: characteristicId,
								},
								{
									Name:          "level_unit",
									Type:          model.String,
									UnitReference: "level",
								},
								{
									Name: "title",
									Type: model.String,
								},
								{
									Name: "missing",
									Type: model.String,
								},
							},
						},
					},
					{
						ProtocolSegmentId: protocol.ProtocolSegments[1].Id,
						Serialization:     "plain-text",
						ContentVariable: model.ContentVariable{
							Name: "other_var",
							Type: model.String,
						},
					},
				},
			},
		},
	}, &devicetype)

	if err != nil {
		return "", "", "", err
	}
	time.Sleep(10 * time.Second)
	return devicetype.Id, devicetype.Services[0].Id, model.ServiceIdToTopic(devicetype.Services[0].Id), nil
}
