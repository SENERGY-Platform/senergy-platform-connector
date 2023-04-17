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
	"fmt"
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	_ "github.com/lib/pq"
	"log"
	"sync"
	"testing"
	"time"
)

func TestWithPProf(t *testing.T) {
	t.Skip("pprof test disabled")
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := configuration.LoadConfig("../config.json")
	if err != nil {
		t.Error(err)
		return
	}
	config.Debug = false
	config.FatalKafkaError = false
	config.Validate = true
	config.ValidateAllowUnknownField = true
	config.ValidateAllowMissingField = true
	config.Log = "stdout"
	config.PublishToPostgres = true

	var brokerUrlForClients string
	config, brokerUrlForClients, err = server.New(ctx, wg, config, client.MQTT4)
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
	}, config.MqttAuthMethod, client.MQTT4)
	if err != nil {
		t.Error(err)
		return
	}

	defer c.Stop()

	size := 100000
	//size := 10000
	for i := 0; i < size; i++ {
		err = c.SendEventWithQos("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`}, 0)
		if err != nil {
			t.Error(err)
			return
		}
		if i%200 == 0 {
			log.Println("...")
			time.Sleep(time.Second)
		}
	}

	time.Sleep(30 * time.Second)
}
