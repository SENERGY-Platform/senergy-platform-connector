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

package main

import (
	"flag"
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/platform-connector-lib/correlation"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	time.Sleep(5 * time.Second) //wait for routing tables in cluster

	configLocation := flag.String("config", "config.json", "configuration file")
	flag.Parse()

	config, err := lib.LoadConfig(*configLocation)
	if err != nil {
		log.Fatal("ERROR: unable to load config ", err)
	}

	if config.KafkaEventTopic == "-" || config.KafkaEventTopic == "false" {
		config.KafkaEventTopic = ""
	}

	correlationservice := correlation.New(int32(config.CorrelationExpiration), lib.StringToList(config.MemcachedUrl)...)

	connector := platform_connector_lib.New(platform_connector_lib.Config{
		FatalKafkaError:          config.FatalKafkaError,
		Protocol:                 config.Protocol,
		KafkaGroupName:           config.KafkaGroupName,
		ZookeeperUrl:             config.ZookeeperUrl,
		AuthExpirationTimeBuffer: config.AuthExpirationTimeBuffer,
		JwtExpiration:            config.JwtExpiration,
		JwtPrivateKey:            config.JwtPrivateKey,
		JwtIssuer:                config.JwtIssuer,
		AuthClientSecret:         config.AuthClientSecret,
		AuthClientId:             config.AuthClientId,
		AuthEndpoint:             config.AuthEndpoint,
		IotRepoUrl:               config.IotRepoUrl,
		DeviceRepoUrl:            config.DeviceRepoUrl,
		KafkaEventTopic:          config.KafkaEventTopic,
		KafkaResponseTopic:       config.KafkaResponseTopic,

		IotCacheUrl:          lib.StringToList(config.IotCacheUrls),
		DeviceExpiration:     int32(config.DeviceExpiration),
		DeviceTypeExpiration: int32(config.DeviceTypeExpiration),

		TokenCacheUrl:        lib.StringToList(config.TokenCacheUrls),
		TokenCacheExpiration: int32(config.TokenCacheExpiration),

		SyncKafka:           config.SyncKafka,
		SyncKafkaIdempotent: config.SyncKafkaIdempotent,
		Debug:               config.Debug,
	})

	if config.Debug {
		connector.SetKafkaLogger(log.New(os.Stdout, "[CONNECTOR-KAFKA] ", 0))
		connector.IotCache.Debug = true
	}

	logger, err := connectionlog.New(config.AmqpUrl, "", config.GatewayLogTopic, config.DeviceLogTopic)
	if err != nil {
		log.Fatal("ERROR: logger ", err)
	}
	defer logger.Stop()
	logger.Debug = config.Debug

	go lib.InitWebhooks(config, connector, logger, correlationservice)

	mqtt, err := lib.MqttStart(config)
	if err != nil {
		log.Fatal("ERROR: unable to start mqtt connection ", err)
	}
	defer mqtt.Close()

	connector.SetAsyncCommandHandler(lib.GetCommandHandler(correlationservice, mqtt, config))

	err = connector.Start()
	if err != nil {
		log.Fatal("ERROR: logger ", err)
	}
	defer connector.Stop()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	sig := <-shutdown
	log.Println("received shutdown signal", sig)

}
