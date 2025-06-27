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
	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler/response"
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
	asyncAPI.Info.Description = "topics or parts of topics in '[]' are placeholders"

	asyncAPI.AddServer("kafka", spec.Server{
		URL:      conf.KafkaUrl,
		Protocol: "kafka",
	})

	asyncAPI.AddServer("mqtt", spec.Server{
		URL:      conf.MqttBroker,
		Protocol: "mqtt",
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
		Name: "[service-topic]",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"kafka"},
			Description: "[service-topic] is a service.Id with replaced '#' and ':' by '_'",
		},
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
		BaseChannelItem: &spec.ChannelItem{
			Description: "may be configured by config.KafkaResponseTopic",
			Servers:     []string{"kafka"},
		},
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
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"kafka"},
			Description: "may be configured by config.Protocol",
		},
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
		BaseChannelItem: &spec.ChannelItem{
			Servers: []string{"kafka"},
		},
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
		BaseChannelItem: &spec.ChannelItem{
			Servers: []string{"kafka"},
		},
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
		BaseChannelItem: &spec.ChannelItem{
			Servers: []string{"kafka"},
		},
		Subscribe: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "DeviceLog",
				Title: "DeviceLog",
			},
			MessageSample: new(connectionlog.DeviceLog),
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "command/[owner]/[device-local-id]/[service-local-id]",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"mqtt"},
			Description: "if config.ForceTopicsWithOwner==false -> topic = command/[device-local-id]/[service-local-id]",
		},
		Subscribe: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "DeviceCommand",
				Title: "DeviceCommand",
			},
			MessageSample: new(lib.RequestEnvelope),
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "response/[owner]/[device-local-id]/[service-local-id]",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"mqtt"},
			Description: "technically not received by mqtt subscription. published mqtt messages will be received by vernemqtt webhooks. if config.ForceTopicsWithOwner==false -> topic = response/[device-local-id]/[service-local-id]",
		},
		Publish: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "CommandResponse",
				Title: "CommandResponse",
			},
			MessageSample: new(response.ResponseEnvelope),
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "event/[owner]/[device-local-id]/[service-local-id]",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"mqtt"},
			Description: "technically not received by mqtt subscription. published mqtt messages will be received by vernemqtt webhooks. if config.ForceTopicsWithOwner==false -> topic = event/[device-local-id]/[service-local-id]",
		},
		Publish: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:        "Event",
				Title:       "Event",
				Description: "map of protocol-segment-name to raw payload. the payload is described in DeviceType.Service",
			},
			MessageSample: new(platform_connector_lib.EventMsg),
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "processes/[hub-id]/[anything]",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"mqtt"},
			Description: "connector checks only access rights. real handling is performed by github.com/SENERGY-Platform/process-sync. technically not received by mqtt subscription. published mqtt messages will be received by vernemqtt webhooks.",
		},
		Publish: &asyncapi.MessageSample{},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "export/[user-id]/[anything]",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"mqtt"},
			Description: "connector checks only access rights. real handling is performed by other service. technically no mqtt interaction but vernemqtt webhook. connector checks if user may subscribe to this topic.",
		},
		Subscribe: &asyncapi.MessageSample{},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "notifications/[user-id]/[anything]",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"mqtt"},
			Description: "connector checks only access rights. real handling is performed by other service. technically no mqtt interaction but vernemqtt webhook. connector checks if user may subscribe to this topic.",
		},
		Subscribe: &asyncapi.MessageSample{},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "error",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"mqtt"},
			Description: "connector notifies user of the received error. technically no mqtt interaction but vernemqtt webhook.",
		},
		Publish: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "Error-Message",
				Title: "Error-Message",
			},
			MessageSample: "",
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "error/device/[local-device-id]",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"mqtt"},
			Description: "connector notifies user of the received error. technically no mqtt interaction but vernemqtt webhook.",
		},
		Publish: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "Error-Message",
				Title: "Error-Message",
			},
			MessageSample: "",
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "error/command/[correlation-id]",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"mqtt"},
			Description: "connector notifies user of the received error and sends a command response with the error to kafka. technically no mqtt interaction but vernemqtt webhook.",
		},
		Publish: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "Error-Message",
				Title: "Error-Message",
			},
			MessageSample: "",
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "error/device/[owner-id]/[local-device-id]",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"mqtt"},
			Description: "connector notifies user of the received error. technically no mqtt interaction but vernemqtt webhook.",
		},
		Publish: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "Error-Message",
				Title: "Error-Message",
			},
			MessageSample: "",
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "error/command/[owner-id]/[correlation-id]",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"mqtt"},
			Description: "connector notifies user of the received error and sends a command response with the error to kafka. technically no mqtt interaction but vernemqtt webhook.",
		},
		Publish: &asyncapi.MessageSample{
			MessageEntity: spec.MessageEntity{
				Name:  "Error-Message",
				Title: "Error-Message",
			},
			MessageSample: "",
		},
	}))

	mustNotFail(reflector.AddChannel(asyncapi.ChannelInfo{
		Name: "fog/analytics/[anything]",
		BaseChannelItem: &spec.ChannelItem{
			Servers:     []string{"mqtt"},
			Description: "technically not received by mqtt subscription. published mqtt messages will be received by vernemqtt webhooks.",
		},
		Publish: &asyncapi.MessageSample{},
	}))

	buff, err := reflector.Schema.MarshalJSON()
	mustNotFail(err)

	fmt.Println(string(buff))
	mustNotFail(os.WriteFile("asyncapi.json", buff, 0o600))
}
