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
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/platform-connector-lib/correlation"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/ory/dockertest"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

func New(startConfig lib.Config) (config lib.Config, shutdown func(), err error) {
	config = startConfig

	whPort, err := getFreePort()
	if err != nil {
		log.Println("unable to find free port", err)
		return config, func() {}, err
	}
	config.WebhookPort = strconv.Itoa(whPort)

	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Println("Could not connect to docker: %s", err)
		return config, func() {}, err
	}

	network, err := pool.Client.NetworkInfo("bridge")
	if err != nil {
		return config, func() {}, err
	}
	hostIp := network.IPAM.Config[0].Gateway

	closerList := []func(){}
	close := func(list []func()) {
		for i := len(list)/2 - 1; i >= 0; i-- {
			opp := len(list) - 1 - i
			list[i], list[opp] = list[opp], list[i]
		}
		for _, c := range list {
			if c != nil {
				c()
			}
		}
	}

	mux := sync.Mutex{}
	var globalError error
	wait := sync.WaitGroup{}

	//zookeeper
	zkWait := sync.WaitGroup{}
	zkWait.Add(1)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer zkWait.Done()
		closer, _, zkIp, err := Zookeeper(pool)
		mux.Lock()
		defer mux.Unlock()
		closerList = append(closerList, closer)
		if err != nil {
			globalError = err
			return
		}
		config.ZookeeperUrl = zkIp + ":2181"
	}()

	//kafka
	kafkaWait := sync.WaitGroup{}
	kafkaWait.Add(1)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer kafkaWait.Done()
		zkWait.Wait()
		if globalError != nil {
			return
		}
		closer, err := Kafka(pool, config.ZookeeperUrl)
		mux.Lock()
		defer mux.Unlock()
		closerList = append(closerList, closer)
		if err != nil {
			globalError = err
			return
		}
	}()

	var elasticIp string
	var mongoIp string

	//kafka
	elasticWait := sync.WaitGroup{}
	elasticWait.Add(1)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer elasticWait.Done()
		if globalError != nil {
			return
		}
		closer, _, ip, err := Elasticsearch(pool)
		elasticIp = ip
		mux.Lock()
		defer mux.Unlock()
		closerList = append(closerList, closer)
		if err != nil {
			globalError = err
			return
		}
	}()

	mongoWait := sync.WaitGroup{}
	mongoWait.Add(1)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer mongoWait.Done()
		if globalError != nil {
			return
		}
		closer, _, ip, err := MongoTestServer(pool)
		mongoIp = ip
		mux.Lock()
		defer mux.Unlock()
		closerList = append(closerList, closer)
		if err != nil {
			globalError = err
			return
		}
	}()

	var permissionUrl string
	permWait := sync.WaitGroup{}
	permWait.Add(1)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer permWait.Done()
		kafkaWait.Wait()
		elasticWait.Wait()
		if globalError != nil {
			return
		}
		closer, _, permIp, err := PermSearch(pool, config.ZookeeperUrl, elasticIp)
		mux.Lock()
		defer mux.Unlock()
		permissionUrl = "http://" + permIp + ":8080"
		closerList = append(closerList, closer)
		if err != nil {
			globalError = err
			return
		}
	}()

	//device-repo
	deviceRepoWait := sync.WaitGroup{}
	deviceRepoWait.Add(1)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer deviceRepoWait.Done()
		mongoWait.Wait()
		zkWait.Wait()
		kafkaWait.Wait()
		permWait.Wait()
		if globalError != nil {
			return
		}
		closer, _, ip, err := DeviceRepo(pool, mongoIp, config.ZookeeperUrl, permissionUrl)
		mux.Lock()
		defer mux.Unlock()
		config.DeviceRepoUrl = "http://" + ip + ":8080"
		closerList = append(closerList, closer)
		if err != nil {
			globalError = err
			return
		}
	}()

	//device-manager
	deviceManagerWait := sync.WaitGroup{}
	deviceManagerWait.Add(1)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer deviceManagerWait.Done()
		deviceRepoWait.Wait()
		mongoWait.Wait()
		if globalError != nil {
			return
		}
		closer, _, ip, err := DeviceManager(pool, config.ZookeeperUrl, config.DeviceRepoUrl, "-", permissionUrl)
		mux.Lock()
		defer mux.Unlock()
		config.DeviceManagerUrl = "http://" + ip + ":8080"
		closerList = append(closerList, closer)
		if err != nil {
			globalError = err
			return
		}
	}()

	//memcached
	cacheWait := sync.WaitGroup{}
	cacheWait.Add(1)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer cacheWait.Done()
		if globalError != nil {
			return
		}
		closer, _, ip, err := Memcached(pool)
		mux.Lock()
		defer mux.Unlock()
		config.MemcachedUrl = ip + ":11211"
		config.IotCacheUrls = ip + ":11211"
		config.TokenCacheUrls = ip + ":11211"
		closerList = append(closerList, closer)
		if err != nil {
			globalError = err
			return
		}
	}()

	keycloakWait := sync.WaitGroup{}
	keycloakWait.Add(1)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer keycloakWait.Done()
		if globalError != nil {
			return
		}
		closer, _, ip, err := Keycloak(pool)
		mux.Lock()
		defer mux.Unlock()
		config.AuthEndpoint = "http://" + ip + ":8080"
		config.AuthClientSecret = "d61daec4-40d6-4d3e-98c9-f3b515696fc6"
		config.AuthClientId = "connector"
		closerList = append(closerList, closer)
		if err != nil {
			globalError = err
			return
		}
	}()

	//vernemq
	mqttWait := sync.WaitGroup{}
	mqttWait.Add(1)
	wait.Add(1)
	go func() {
		defer wait.Done()
		defer mqttWait.Done()
		if globalError != nil {
			return
		}
		closer, _, ip, err := Vernemqtt(pool, hostIp+":"+config.WebhookPort)
		mux.Lock()
		defer mux.Unlock()
		config.MqttBroker = "tcp://" + ip + ":1883"
		closerList = append(closerList, closer)
		if err != nil {
			globalError = err
			return
		}
	}()

	wait.Wait()
	if globalError != nil {
		close(closerList)
		return config, shutdown, globalError
	}
	//senergy-connector

	correlationservice := correlation.New(10, lib.StringToList(config.MemcachedUrl)...)
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

		DeviceExpiration:     int32(config.DeviceExpiration),
		DeviceTypeExpiration: int32(config.DeviceTypeExpiration),
		IotCacheUrl:          strings.Split(config.IotCacheUrls, ","),

		TokenCacheUrl:        lib.StringToList(config.TokenCacheUrls),
		TokenCacheExpiration: int32(config.TokenCacheExpiration),

		SyncKafka:           config.SyncKafka,
		SyncKafkaIdempotent: config.SyncKafkaIdempotent,
		Debug:               config.Debug,
	})

	connector.SetKafkaLogger(log.New(os.Stdout, "[CONNECTOR-KAFKA] ", 0))

	logger, err := connectionlog.New(config.ZookeeperUrl, config.SyncKafka, config.SyncKafkaIdempotent, config.DeviceLogTopic, config.GatewayLogTopic)
	if err != nil {
		log.Println("ERROR: unable to start connectionlog:", err)
		close(closerList)
		return config, shutdown, err
	}
	closerList = append(closerList, func() {
		logger.Close()
	})

	go lib.InitWebhooks(config, connector, logger, correlationservice)

	mqtt, err := lib.MqttStart(config)
	if err != nil {
		close(closerList)
		return config, shutdown, err
	}

	connector.SetAsyncCommandHandler(lib.GetCommandHandler(correlationservice, mqtt, config))

	err = connector.Start()
	if err != nil {
		close(closerList)
		return config, shutdown, err
	}
	closerList = append(closerList, func() {
		connector.Stop()
	})

	return config, func() { close(closerList) }, nil
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
