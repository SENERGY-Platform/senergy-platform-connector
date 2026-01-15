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
	"time"

	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/correlation"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
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
					config.GetLogger().Error("unable to handle command", "error", err)
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
		config.GetLogger().Debug("received command", "device_id", commandRequest.Metadata.Device.Id, "service_id", commandRequest.Metadata.Service.Id, "input", commandRequest.Request.Input)
		correlationId, err := correlationservice.Save(commandRequest)
		if err != nil {
			config.GetLogger().Error("unable to save correlation", "error", err)
			return err
		}
		envelope := RequestEnvelope{Payload: requestMsg, CorrelationId: correlationId, Time: t.Unix(), CompletionStrategy: commandRequest.TaskInfo.CompletionStrategy}
		b, err := json.Marshal(envelope)
		if err != nil {
			config.GetLogger().Error("unable to marshal envelope", "error", err)
			return err
		}
		topic := "command/" + commandRequest.Metadata.Device.OwnerId + "/" + commandRequest.Metadata.Device.LocalId + "/" + commandRequest.Metadata.Service.LocalId
		config.GetLogger().Debug("send command to mqtt", "topic", topic)
		err = mqtt.Publish(topic, string(b))
		if err != nil {
			return err
		}
		if !config.ForceTopicsWithOwner {
			topic := "command/" + commandRequest.Metadata.Device.LocalId + "/" + commandRequest.Metadata.Service.LocalId
			config.GetLogger().Debug("send command to mqtt", "topic", topic)
			err = mqtt.Publish(topic, string(b))
			if err != nil {
				return err
			}
		}
		return nil
	}
}
