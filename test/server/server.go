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

package server

import (
	"context"
	"log"
	"net"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"

	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler/event"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server/docker"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server/mock/auth"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server/mock/iot"
)

func getOutboundIP() net.IP {
	conn, err := net.Dial("udp", "9.9.9.9:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

func New(ctx context.Context, wg *sync.WaitGroup, startConfig configuration.Config, mqttVersion client.MqttVersion) (config configuration.Config, brokerUrlForClients string, err error) {
	config = startConfig

	err = auth.Mock(config, ctx)
	if err != nil {
		return config, "", err
	}

	config.WebhookPort, err = GetFreePort()
	if err != nil {
		log.Println("unable to find free port", err)
		return config, "", err
	}

	config.HttpCommandConsumerPort, err = GetFreePort()
	if err != nil {
		log.Println("unable to find free port", err)
		return config, "", err
	}

	hostIp := getOutboundIP().String()

	config.KafkaUrl, err = docker.Kafka(ctx, wg)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		return config, "", err
	}

	err = iot.Mock(config, ctx, true)
	if err != nil {
		return config, "", err
	}

	memcachePort, memcacheIp, err := docker.Memcached(ctx, wg)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		return config, "", err
	}
	config.MemcachedUrl = memcacheIp + ":" + memcachePort
	config.IotCacheUrls = memcacheIp + ":" + memcachePort
	config.TokenCacheUrls = memcacheIp + ":" + memcachePort

	if mqttVersion == client.MQTT5 {
		config.MqttVersion = "5"
	}
	var brokerUrlForConnector string
	brokerUrlForConnector, brokerUrlForClients, config.VmqAdminApiUrl, err = docker.Vernemqtt(ctx, wg, hostIp+":"+config.WebhookPort, config)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		return config, "", err
	}
	config.MqttBroker = brokerUrlForConnector

	//transform local-address to address in docker container
	deviceManagerUrlStruct := strings.Split(config.DeviceManagerUrl, ":")
	deviceManagerUrl := "http://" + hostIp + ":" + deviceManagerUrlStruct[len(deviceManagerUrlStruct)-1]
	log.Println("DEBUG: semantic url transformation:", config.DeviceManagerUrl, "-->", deviceManagerUrl)

	err = lib.Start(ctx, config, event.NewTestWaitingRoom())
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		return config, "", err
	}

	return config, brokerUrlForClients, nil
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func GetFreePort() (string, error) {
	temp, err := getFreePort()
	return strconv.Itoa(temp), err
}

type VoidWriter struct{}

func (v VoidWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}
