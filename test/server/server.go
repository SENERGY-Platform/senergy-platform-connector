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

	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server/docker"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server/mock/auth"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server/mock/iot"
	"github.com/ory/dockertest/v3"
)

func New(basectx context.Context, startConfig configuration.Config) (config configuration.Config, err error) {
	config = startConfig

	ctx, cancel := context.WithCancel(basectx)

	err = auth.Mock(config, ctx)
	if err != nil {
		cancel()
		return config, err
	}

	config.WebhookPort, err = GetFreePort()
	if err != nil {
		log.Println("unable to find free port", err)
		return config, err
	}

	config.HttpCommandConsumerPort, err = GetFreePort()
	if err != nil {
		log.Println("unable to find free port", err)
		return config, err
	}

	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Println("Could not connect to docker: ", err)
		return config, err
	}

	_, zk, err := docker.Zookeeper(pool, ctx)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		cancel()
		return config, err
	}
	zookeeperUrl := zk + ":2181"

	config.KafkaUrl, err = docker.Kafka(pool, ctx, zookeeperUrl)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		cancel()
		return config, err
	}

	err = iot.Mock(config, ctx)
	if err != nil {
		cancel()
		return config, err
	}

	_, memcacheIp, err := docker.Memcached(pool, ctx)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		cancel()
		return config, err
	}
	config.MemcachedUrl = memcacheIp + ":11211"
	config.IotCacheUrls = memcacheIp + ":11211"
	config.TokenCacheUrls = memcacheIp + ":11211"

	network, err := pool.Client.NetworkInfo("bridge")
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		cancel()
		return config, err
	}
	hostIp := network.IPAM.Config[0].Gateway
	config.MqttBroker, err = docker.Vernemqtt(pool, ctx, hostIp+":"+config.WebhookPort, config)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		cancel()
		return config, err
	}

	config.PostgresHost, config.PostgresPort, config.PostgresUser, config.PostgresPw, config.PostgresDb, err = docker.Timescale(pool, ctx)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		cancel()
		return config, err
	}

	hostIp = "127.0.0.1"
	networks, _ := pool.Client.ListNetworks()
	for _, network := range networks {
		if network.Name == "bridge" {
			hostIp = network.IPAM.Config[0].Gateway
			break
		}
	}

	//transform local-address to address in docker container
	deviceManagerUrlStruct := strings.Split(config.DeviceManagerUrl, ":")
	deviceManagerUrl := "http://" + hostIp + ":" + deviceManagerUrlStruct[len(deviceManagerUrlStruct)-1]
	log.Println("DEBUG: semantic url transformation:", config.DeviceManagerUrl, "-->", deviceManagerUrl)

	err = docker.Tableworker(pool, ctx, config.PostgresHost, config.PostgresPort, config.PostgresUser, config.PostgresPw, config.PostgresDb, config.KafkaUrl, deviceManagerUrl)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		cancel()
		return config, err
	}

	err = lib.Start(ctx, config)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		cancel()
		return config, err
	}

	return config, nil
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
