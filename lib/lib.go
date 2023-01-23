/*
 * Copyright 2021 InfAI (CC SES)
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
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlimit"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/platform-connector-lib/correlation"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler/command"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler/errormessage"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler/event"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler/export"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler/fog"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler/notifications"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler/process"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler/response"
	"github.com/Shopify/sarama"
	"log"
	"strings"
	"time"
)

func Start(parentCtx context.Context, config configuration.Config) (err error) {
	ctx, cancel := context.WithCancel(parentCtx)
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	asyncFlushFrequency, err := time.ParseDuration(config.AsyncFlushFrequency)
	if err != nil {
		return err
	}

	correlationTimeout := 200 * time.Millisecond
	if timeout, err := time.ParseDuration(config.CorrelationTimeout); err != nil {
		log.Println("WARNING: invalid CorrelationTimeout; use default 200ms")
	} else {
		correlationTimeout = timeout
	}

	memcaacheUrls := StringToList(config.MemcachedUrl)

	correlationservice := correlation.New(int32(config.CorrelationExpiration), int(config.CorrelationMaxIdleConns), correlationTimeout, memcaacheUrls...)

	connector := platform_connector_lib.New(platform_connector_lib.Config{
		PartitionsNum:            config.KafkaPartitionNum,
		ReplicationFactor:        config.KafkaReplicationFactor,
		FatalKafkaError:          config.FatalKafkaError,
		Protocol:                 config.Protocol,
		KafkaGroupName:           config.KafkaGroupName,
		KafkaUrl:                 config.KafkaUrl,
		AuthExpirationTimeBuffer: config.AuthExpirationTimeBuffer,
		JwtExpiration:            config.JwtExpiration,
		JwtPrivateKey:            config.JwtPrivateKey,
		JwtIssuer:                config.JwtIssuer,
		AuthClientSecret:         config.AuthClientSecret,
		AuthClientId:             config.AuthClientId,
		AuthEndpoint:             config.AuthEndpoint,
		DeviceManagerUrl:         config.DeviceManagerUrl,
		DeviceRepoUrl:            config.DeviceRepoUrl,
		KafkaResponseTopic:       config.KafkaResponseTopic,

		IotCacheUrl:              StringToList(config.IotCacheUrls),
		DeviceExpiration:         int32(config.DeviceExpiration),
		DeviceTypeExpiration:     int32(config.DeviceTypeExpiration),
		CharacteristicExpiration: int32(config.CharacteristicExpiration),

		TokenCacheUrl:        StringToList(config.TokenCacheUrls),
		TokenCacheExpiration: int32(config.TokenCacheExpiration),

		Debug: config.Debug,

		Validate:                  config.Validate,
		ValidateAllowMissingField: config.ValidateAllowMissingField,
		ValidateAllowUnknownField: config.ValidateAllowUnknownField,

		PublishToPostgres: config.PublishToPostgres,
		PostgresHost:      config.PostgresHost,
		PostgresPort:      config.PostgresPort,
		PostgresUser:      config.PostgresUser,
		PostgresPw:        config.PostgresPw,
		PostgresDb:        config.PostgresDb,

		HttpCommandConsumerPort: config.HttpCommandConsumerPort,

		SyncCompression:     getKafkaCompression(config.SyncCompression),
		AsyncCompression:    getKafkaCompression(config.AsyncCompression),
		AsyncFlushFrequency: asyncFlushFrequency,
		AsyncFlushMessages:  int(config.AsyncFlushMessages),
		AsyncPgThreadMax:    int(config.AsyncPgThreadMax),

		KafkaConsumerMinBytes: int(config.KafkaConsumerMinBytes),
		KafkaConsumerMaxBytes: int(config.KafkaConsumerMaxBytes),
		KafkaConsumerMaxWait:  config.KafkaConsumerMaxWait,

		IotCacheTimeout:      config.IotCacheTimeout,
		IotCacheMaxIdleConns: int(config.IotCacheMaxIdleConns),

		StatisticsInterval: config.StatisticsInterval,

		DeviceTypeTopic: config.DeviceTypeTopic,

		NotificationUrl: config.NotificationUrl,
		PermQueryUrl:    config.PermQueryUrl,

		KafkaTopicConfigs: config.KafkaTopicConfigs,

		NotificationsIgnoreDuplicatesWithinS: config.NotificationsIgnoreDuplicatesWithinS,
	})

	connector.StatisticsLogger(ctx)

	if config.Debug {
		connector.SetKafkaLogger(log.New(log.Writer(), "[CONNECTOR-KAFKA] ", 0))
		connector.IotCache.Debug = true
	}

	err = connector.InitProducer(ctx, []platform_connector_lib.Qos{platform_connector_lib.Async, platform_connector_lib.Sync, platform_connector_lib.SyncIdempotent})
	if err != nil {
		log.Println("ERROR: producer ", err)
		return err
	}

	logProducer, err := connector.GetProducer(platform_connector_lib.Qos(config.ConnectionLogQos))
	if err != nil {
		log.Println("ERROR: logger ", err)
		return err
	}
	logger, err := connectionlog.NewWithProducer(logProducer, config.DeviceLogTopic, config.GatewayLogTopic)
	if err != nil {
		log.Println("ERROR: logger ", err)
		return err
	}

	handlers := []handler.Handler{
		event.New(config, connector),
		response.New(config, connector, correlationservice),
		command.New(config, connector, logger),
		process.New(connector),
		export.New(connector.Security()),
		notifications.New(connector.Security()),
		errormessage.New(connector, correlationservice),
	}
	if config.FogHandlerTopicPrefix != "" && config.FogHandlerTopicPrefix != "-" {
		handlers = append(handlers, fog.NewHandler(connector, config.FogHandlerTopicPrefix))
	}

	var connectionLimit *connectionlimit.ConnectionLimitHandler
	if config.ConnectionLimitCount > 0 {
		connectionLimit = connectionlimit.New(config.ConnectionLimitCount, int32(config.ConnectionLimitDurationInSeconds), memcaacheUrls...)
	}

	InitWebhooks(config, connector, logger, handlers, connectionLimit)

	if config.StartupDelay != 0 {
		time.Sleep(time.Duration(config.StartupDelay) * time.Second)
	}

	var mqtt *Mqtt
	for i := 0; i < 10; i++ {
		mqtt, err = MqttStart(ctx, config)
		if err == nil {
			break
		}
		time.Sleep(5 * time.Second)
	}
	if err != nil {
		log.Println("ERROR: unable to start mqtt connection ", err)
		return err
	}

	if config.CommandWorkerCount > 1 {
		connector.SetAsyncCommandHandler(GetQueuedCommandHandler(ctx, correlationservice, mqtt, config))
	} else {
		connector.SetAsyncCommandHandler(GetCommandHandler(correlationservice, mqtt, config))
	}

	err = connector.StartConsumer(ctx)
	if err != nil {
		log.Println("ERROR: logger ", err)
		return err
	}
	return nil
}

func getKafkaCompression(compression string) sarama.CompressionCodec {
	switch strings.ToLower(compression) {
	case "":
		return sarama.CompressionNone
	case "-":
		return sarama.CompressionNone
	case "none":
		return sarama.CompressionNone
	case "gzip":
		return sarama.CompressionGZIP
	case "snappy":
		return sarama.CompressionSnappy
	}
	log.Println("WARNING: unknown compression", compression, "fallback to none")
	return sarama.CompressionNone
}
