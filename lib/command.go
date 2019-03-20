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
)

func GetCommandHandler(correlationservice *correlation.CorrelationService, mqtt *Mqtt) platform_connector_lib.AsyncCommandHandler {
	return func(commandRequest model.ProtocolMsg, requestMsg platform_connector_lib.CommandRequestMsg) (err error) {
		correlationId, err := correlationservice.Save(commandRequest)
		if err != nil {
			log.Println("ERROR: unable to save correlation", err)
			return err
		}
		envelope := RequestEnvelope{Payload: requestMsg, CorrelationId: correlationId}
		b, err := json.Marshal(envelope)
		if err != nil {
			log.Println("ERROR: unable to marshal envelope", err)
			return err
		}
		return mqtt.Publish("command/"+commandRequest.DeviceUrl+"/"+commandRequest.ServiceUrl, string(b))
	}
}
