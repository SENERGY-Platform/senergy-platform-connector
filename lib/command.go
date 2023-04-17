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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/correlation"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"log"
	"time"
)

type commandQueueValue struct {
	commandRequest model.ProtocolMsg
	requestMsg     platform_connector_lib.CommandRequestMsg
	t              time.Time
}

func GetQueuedCommandHandler(ctx context.Context, correlationservice *correlation.CorrelationService, mqtt Mqtt, config configuration.Config) platform_connector_lib.AsyncCommandHandler {
	queue := make(chan commandQueueValue, config.CommandWorkerCount)
	handler := GetCommandHandler(correlationservice, mqtt, config)
	for i := int64(0); i < config.CommandWorkerCount; i++ {
		go func() {
			for msg := range queue {
				err := handler(msg.commandRequest, msg.requestMsg, msg.t)
				if err != nil {
					log.Println("ERROR: ", err)
				}
			}
		}()
	}
	go func() {
		<-ctx.Done()
		close(queue)
	}()
	return func(commandRequest model.ProtocolMsg, requestMsg platform_connector_lib.CommandRequestMsg, t time.Time) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = errors.New(fmt.Sprint(r))
			}
		}()
		queue <- commandQueueValue{
			commandRequest: commandRequest,
			requestMsg:     requestMsg,
			t:              t,
		}
		return err
	}
}

func GetCommandHandler(correlationservice *correlation.CorrelationService, mqtt Mqtt, config configuration.Config) platform_connector_lib.AsyncCommandHandler {
	return func(commandRequest model.ProtocolMsg, requestMsg platform_connector_lib.CommandRequestMsg, t time.Time) (err error) {
		commandRequest.Trace = append(commandRequest.Trace, model.Trace{
			Timestamp: time.Now().UnixNano(),
			TimeUnit:  "unix_nano",
			Location:  "github.com/SENERGY-Platform/senergy-platform-connector GetCommandHandler()",
		})
		if config.Debug {
			log.Println("DEBUG: receive command", commandRequest.Metadata.Device.Id, commandRequest.Metadata.Service.Id, commandRequest.Request.Input)
		}
		correlationId, err := correlationservice.Save(commandRequest)
		if err != nil {
			log.Println("ERROR: unable to save correlation", err)
			return err
		}
		envelope := RequestEnvelope{Payload: requestMsg, CorrelationId: correlationId, Time: t.Unix(), CompletionStrategy: commandRequest.TaskInfo.CompletionStrategy}
		b, err := json.Marshal(envelope)
		if err != nil {
			log.Println("ERROR: unable to marshal envelope", err)
			return err
		}
		if config.Debug {
			log.Println("DEBUG: send command to mqtt", "command/"+commandRequest.Metadata.Device.LocalId+"/"+commandRequest.Metadata.Service.LocalId, envelope)
		}
		return mqtt.Publish("command/"+commandRequest.Metadata.Device.LocalId+"/"+commandRequest.Metadata.Service.LocalId, string(b))
	}
}
