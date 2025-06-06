/*
 * Copyright 2025 InfAI (CC SES)
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

package main

import (
	"flag"
	"fmt"
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"log"
	"os"

	"github.com/swaggest/go-asyncapi/reflector/asyncapi-2.4.0"
	"github.com/swaggest/go-asyncapi/spec-2.4.0"
)

//go:generate go run main.go

type Envelope struct {
	DeviceId  string                 `json:"device_id,omitempty"`
	ServiceId string                 `json:"service_id,omitempty"`
	Value     map[string]interface{} `json:"value"`
}

func main() {
	configLocation := flag.String("config", "../../config.json", "configuration file")
	flag.Parse()

	conf, err := configuration.LoadConfig(*configLocation)
	if err != nil {
		log.Fatal("ERROR: unable to load config", err)
	}

	asyncAPI := spec.AsyncAPI{}
	asyncAPI.Info.Title = "Senergy-Platform-Connector"

	asyncAPI.AddServer("kafka", spec.Server{
		URL:      conf.KafkaUrl,
		Protocol: "kafka",
	})

	reflector := asyncapi.Reflector{}
	reflector.Schema = &asyncAPI

	mustNotFail := func(err error) {
		if err != nil {
			panic(err.Error())
		}
	}

	//"topic is a service.Id with replaced '#' and ':' by '_'"
	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "Service-Topic",
		Subscribe: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "Envelope",
				Title: "Envelope",
			},
			MessageSample: new(Envelope),
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: conf.KafkaResponseTopic,
		Subscribe: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "ProtocolMsg",
				Title: "ProtocolMsg",
			},
			MessageSample: new(model.ProtocolMsg),
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: conf.Protocol,
		Publish: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "ProtocolMsg",
				Title: "ProtocolMsg",
			},
			MessageSample: new(model.ProtocolMsg),
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: conf.DeviceTypeTopic,
		Publish: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "DeviceTypeCommand",
				Title: "DeviceTypeCommand",
			},
			MessageSample: new(platform_connector_lib.DeviceTypeCommand),
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: conf.GatewayLogTopic,
		Subscribe: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "HubLog",
				Title: "HubLog",
			},
			MessageSample: new(connectionlog.HubLog),
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: conf.DeviceLogTopic,
		Subscribe: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "DeviceLog",
				Title: "DeviceLog",
			},
			MessageSample: new(connectionlog.DeviceLog),
		},
	}))

	buff, err := reflector.Schema.MarshalJSON()
	mustNotFail(err)

	fmt.Println(string(buff))
	mustNotFail(os.WriteFile("asyncapi.json", buff, 0o600))
}
