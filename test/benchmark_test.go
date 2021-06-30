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
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	paho "github.com/eclipse/paho.mqtt.golang"
	"log"
	"sync"
	"testing"
	"time"
)

func BenchmarkPublish(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer time.Sleep(10 * time.Second) //wait for container shutdown
	defer cancel()

	config, err := configuration.LoadConfig("../config.json")
	if err != nil {
		b.Error(err)
		return
	}
	config.Debug = false
	config.FatalKafkaError = false
	config.Validate = true
	config.ValidateAllowUnknownField = true
	config.ValidateAllowMissingField = true
	config.Log = "stdout"
	config.SyncKafka = true
	config.SyncKafkaIdempotent = true

	config, err = server.New(ctx, config)
	if err != nil {
		b.Error(err)
		return
	}

	testCharacteristicName := "test2"

	_, err = createCharacteristic("test1", config)
	if err != nil {
		b.Error(err)
		return
	}
	characteristicId, err := createCharacteristic(testCharacteristicName, config)
	if err != nil {
		b.Error(err)
		return
	}
	deviceTypeId, _, _, err := createDeviceType(config, config.DeviceManagerUrl, characteristicId)
	if err != nil {
		b.Error(err)
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
		b.Error(err)
		return
	}

	defer c.Stop()

	options := paho.NewClientOptions().
		SetUsername(config.AuthClientId).
		SetAutoReconnect(true).
		SetCleanSession(true).
		AddBroker(config.MqttBroker)
	listener := paho.NewClient(options)
	if token := listener.Connect(); token.Wait() && token.Error() != nil {
		log.Println("Error on listener.Connect(): ", token.Error())
		b.Error(token.Error())
		return
	}

	events := []string{}
	mux := sync.Mutex{}
	doneTime := 2 * time.Second
	ticker := time.NewTicker(20 * time.Second)
	token := listener.Subscribe("event/test1/sepl_get", 1, func(client paho.Client, message paho.Message) {
		//log.Println("event:", string(message.Payload()))
		mux.Lock()
		defer mux.Unlock()
		events = append(events, string(message.Payload()))
		ticker.Reset(doneTime)
	})
	if token.Wait() && token.Error() != nil {
		log.Println("Error on listener.Subscribe(): ", token.Error())
		b.Error(token.Error())
		return
	}

	b.ResetTimer()
	b.Run("send event", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})
			if err != nil {
				log.Println("ERROR:", err)
				b.Error(err)
				return
			}
		}
	})

	b.Run("send event large", func(b *testing.B) {
		large := `{"time":"2021-02-01T05:00:03.280Z","location_ec-generator_gesamtwirkleistung":2207.443127441406,"location_ec-gesamt_gesamtwirkleistung":3958.2597578125,"location_ec-prozess_gesamtwirkleistung":3271.347515625,"location_ec-roboter_gesamtwirkleistung":600.3756289062501,"location_roboter-ausgabe_gesamtwirkleistung":662.5674282226562,"location_roboter-eingabe_gesamtwirkleistung":344.682021484375,"location_transport-gesamt_gesamtwirkleistung":1605.263171875,"location_wm1-gesamt_gesamtwirkleistung":9470.428890625,"location_wm1-heizung-reinigen_gesamtwirkleistung":2395.017423828125,"location_wm1-heizung-trocknung_gesamtwirkleistung":0.0,"location_wm2-gesamt_gesamtwirkleistung":24943.22471875,"location_wm2-heizung-reinigen_gesamtwirkleistung":7294.0812421875,"location_wm2-heizung-trocknung_gesamtwirkleistung":0.0,"location_wm2-vakuumpumpe_gesamtwirkleistung":5473.771453125001,"module_1_state":3,"module_1_errorcode":0,"module_1_errorindex":0,"module_2_state":3,"module_2_errorcode":0,"module_2_errorindex":0,"module_4_state":3,"module_4_errorcode":0,"module_4_errorindex":0,"module_5_state":3,"module_5_errorcode":0,"module_5_errorindex":0,"module_6_state":3,"module_6_errorcode":0,"module_6_errorindex":0,"module_1_processdata":[{"station":1,"process":1,"part":1,"main_id":"de6ccf143313332ab0c430504dbbd8d076458bc0a51a9fb3c36c4dce65ce3d0b","errorcode":0,"result":null},{"station":1,"process":1,"part":2,"main_id":"41361e5961b88590b2dee2f396e72406e8b5d7749e320bc982f65f8f6674db26","errorcode":0,"result":null}],"module_2_processdata":null}`
		for i := 0; i < b.N; i++ {
			err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": large})
			if err != nil {
				log.Println("ERROR:", err)
				b.Error(err)
				return
			}
		}
	})

	<-ticker.C
	ticker.Stop()
	mux.Lock()
	defer mux.Unlock()
	b.Log(len(events))
}
