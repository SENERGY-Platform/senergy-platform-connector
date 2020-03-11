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

type VoidWriter struct{}

func (v VoidWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func main() {
	time.Sleep(5 * time.Second) //wait for routing tables in cluster

	configLocation := flag.String("config", "config.json", "configuration file")
	flag.Parse()

	config, err := lib.LoadConfig(*configLocation)
	if err != nil {
		log.Fatal("ERROR: unable to load config ", err)
	}

	switch config.Log {
	case "void":
		log.SetOutput(VoidWriter{})
	case "":
		break
	case "stdout":
		log.SetOutput(os.Stdout)
	case "stderr":
		log.SetOutput(os.Stderr)
	default:
		f, err := os.OpenFile(config.Log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		log.SetOutput(f)
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
		DeviceManagerUrl:         config.DeviceManagerUrl,
		DeviceRepoUrl:            config.DeviceRepoUrl,
		KafkaResponseTopic:       config.KafkaResponseTopic,

		IotCacheUrl:          lib.StringToList(config.IotCacheUrls),
		DeviceExpiration:     int32(config.DeviceExpiration),
		DeviceTypeExpiration: int32(config.DeviceTypeExpiration),

		TokenCacheUrl:        lib.StringToList(config.TokenCacheUrls),
		TokenCacheExpiration: int32(config.TokenCacheExpiration),

		SyncKafka:           config.SyncKafka,
		SyncKafkaIdempotent: config.SyncKafkaIdempotent,
		Debug:               config.Debug,

		Validate:                  config.Validate,
		ValidateAllowMissingField: config.ValidateAllowMissingField,
		ValidateAllowUnknownField: config.ValidateAllowUnknownField,
	})

	if config.Debug {
		connector.SetKafkaLogger(log.New(log.Writer(), "[CONNECTOR-KAFKA] ", 0))
		connector.IotCache.Debug = true
	}

	logger, err := connectionlog.New(config.ZookeeperUrl, config.SyncKafka, config.SyncKafkaIdempotent, config.DeviceLogTopic, config.GatewayLogTopic)
	if err != nil {
		log.Fatal("ERROR: logger ", err)
	}
	defer logger.Close()

	go lib.InitWebhooks(config, connector, logger, correlationservice)

	if config.StartupDelay != 0 {
		time.Sleep(time.Duration(config.StartupDelay) * time.Second)
	}

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
