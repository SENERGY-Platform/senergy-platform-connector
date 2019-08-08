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

package lib

import (
	"encoding/json"
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/correlation"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"log"
	"time"
)

func GetCommandHandler(correlationservice *correlation.CorrelationService, mqtt *Mqtt, config Config) platform_connector_lib.AsyncCommandHandler {
	return func(commandRequest model.ProtocolMsg, requestMsg platform_connector_lib.CommandRequestMsg, t time.Time) (err error) {
		if config.Debug {
			log.Println("DEBUG: receive command", commandRequest.DeviceInstanceId, commandRequest.ServiceId, commandRequest.ProtocolParts)
		}
		correlationId, err := correlationservice.Save(commandRequest)
		if err != nil {
			log.Println("ERROR: unable to save correlation", err)
			return err
		}
		envelope := RequestEnvelope{Payload: requestMsg, CorrelationId: correlationId, Time: t.Unix(), CompletionStrategy: commandRequest.CompletionStrategy}
		b, err := json.Marshal(envelope)
		if err != nil {
			log.Println("ERROR: unable to marshal envelope", err)
			return err
		}
		if config.Debug {
			log.Println("DEBUG: send command to mqtt", "command/"+commandRequest.DeviceUrl+"/"+commandRequest.ServiceUrl, envelope)
		}
		return mqtt.Publish("command/"+commandRequest.DeviceUrl+"/"+commandRequest.ServiceUrl, string(b))
	}
}
