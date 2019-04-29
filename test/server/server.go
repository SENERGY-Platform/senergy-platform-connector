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
	"encoding/json"
	"github.com/SENERGY-Platform/iot-device-repository/lib/persistence/ordf"
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/connectionlog"
	"github.com/SENERGY-Platform/platform-connector-lib/correlation"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/bradfitz/gomemcache/memcache"
	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	"github.com/segmentio/kafka-go"
	"github.com/streadway/amqp"
	"github.com/wvanbergen/kazoo-go"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
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

	var wait sync.WaitGroup

	listMux := sync.Mutex{}
	var globalError error
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

	//zookeeper
	wait.Add(1)
	go func() {
		defer wait.Done()
		closeZk, _, zkIp, err := Zookeeper(pool)
		listMux.Lock()
		closerList = append(closerList, closeZk)
		listMux.Unlock()
		if err != nil {
			globalError = err
			return
		}
		config.ZookeeperUrl = zkIp + ":2181"

		//kafka
		closeKafka, err := Kafka(pool, config.ZookeeperUrl)
		listMux.Lock()
		closerList = append(closerList, closeKafka)
		listMux.Unlock()
		if err != nil {
			globalError = err
			return
		}
	}()

	wait.Add(1)
	go func() {
		defer wait.Done()

		var wait2 sync.WaitGroup

		var elasticIp string
		var ontoIp string
		var amqpIp string

		wait2.Add(1)
		go func() {
			defer wait2.Done()
			//amqp
			closeAmqp, _, ip, err := Amqp(pool)
			amqpIp = ip
			listMux.Lock()
			closerList = append(closerList, closeAmqp)
			listMux.Unlock()
			if err != nil {
				globalError = err
				return
			}
			config.AmqpUrl = "amqp://guest:guest@" + amqpIp + ":5672"
		}()

		wait2.Add(1)
		go func() {
			defer wait2.Done()
			//elasticsearch
			closeElastic, _, ip, err := Elasticsearch(pool)
			elasticIp = ip
			listMux.Lock()
			closerList = append(closerList, closeElastic)
			listMux.Unlock()
			if err != nil {
				globalError = err
				return
			}
		}()

		wait2.Add(1)
		go func() {
			defer wait2.Done()
			//iot-onto
			closeOnto, _, ip, err := IotOntology(pool)
			ontoIp = ip
			listMux.Lock()
			closerList = append(closerList, closeOnto)
			listMux.Unlock()
			if err != nil {
				globalError = err
				return
			}
		}()

		wait2.Wait()

		if globalError != nil {
			return
		}

		//permsearch
		closePerm, _, permIp, err := PermSearch(pool, amqpIp, elasticIp)
		listMux.Lock()
		closerList = append(closerList, closePerm)
		listMux.Unlock()
		if err != nil {
			globalError = err
			return
		}

		//iot-repo
		closeIot, _, iotIp, err := IotRepo(pool, ontoIp, amqpIp, permIp)
		listMux.Lock()
		closerList = append(closerList, closeIot)
		listMux.Unlock()
		if err != nil {
			globalError = err
			return
		}
		config.IotRepoUrl = "http://" + iotIp + ":8080"
	}()

	wait.Add(1)
	go func() {
		defer wait.Done()
		//memcached
		closeMem, _, memIp, err := Memcached(pool)
		listMux.Lock()
		closerList = append(closerList, closeMem)
		listMux.Unlock()
		if err != nil {
			globalError = err
			return
		}
		config.MemcachedUrl = memIp + ":11211"
		config.IotCacheUrls = memIp + ":11211"
		config.TokenCacheUrls = memIp + ":11211"
	}()

	wait.Add(1)
	go func() {
		defer wait.Done()
		//keycloak
		closeKeycloak, _, keycloakIp, err := Keycloak(pool)
		listMux.Lock()
		closerList = append(closerList, closeKeycloak)
		listMux.Unlock()
		if err != nil {
			globalError = err
			return
		}
		config.AuthEndpoint = "http://" + keycloakIp + ":8080"
		config.AuthClientSecret = "d61daec4-40d6-4d3e-98c9-f3b515696fc6"
		config.AuthClientId = "connector"
	}()

	wait.Add(1)
	go func() {
		defer wait.Done()
		//vernemq
		closeVerne, _, verneIp, err := Vernemqtt(pool, hostIp+":"+config.WebhookPort)
		listMux.Lock()
		closerList = append(closerList, closeVerne)
		listMux.Unlock()
		if err != nil {
			globalError = err
			return
		}
		config.MqttBroker = "tcp://" + verneIp + ":1883"
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
		IotRepoUrl:               config.IotRepoUrl,
		KafkaEventTopic:          config.KafkaEventTopic,
		KafkaResponseTopic:       config.KafkaResponseTopic,

		DeviceExpiration:     int32(config.DeviceExpiration),
		DeviceTypeExpiration: int32(config.DeviceTypeExpiration),
		IotCacheUrl:          strings.Split(config.IotCacheUrls, ","),

		TokenCacheUrl:        lib.StringToList(config.TokenCacheUrls),
		TokenCacheExpiration: int32(config.TokenCacheExpiration),

		SyncKafka:             config.SyncKafka,
		SyncKafkaIdempotent:   config.SyncKafkaIdempotent,
		KafkaProducerPoolSize: config.KafkaProducerPoolSize,
	})

	connector.SetKafkaLogger(log.New(os.Stdout, "[CONNECTOR-KAFKA] ", 0))

	logger, err := connectionlog.New(config.AmqpUrl, "", config.GatewayLogTopic, config.DeviceLogTopic)
	if err != nil {
		log.Println("ERROR: unable to start connectionlog:", err)
		close(closerList)
		return config, shutdown, err
	}
	closerList = append(closerList, func() {
		logger.Stop()
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

func Keycloak(pool *dockertest.Pool) (closer func(), hostPort string, ipAddress string, err error) {
	log.Println("start keycloak")
	keycloak, err := pool.Run("fgseitsrancher.wifa.intern.uni-leipzig.de:5000/testkeycloak", "test", []string{
		"KEYCLOAK_USER=sepl",
		"KEYCLOAK_PASSWORD=sepl",
		"PROXY_ADDRESS_FORWARDING=true",
	})
	if err != nil {
		return func() {}, "", "", err
	}
	hostPort = keycloak.GetPort("8080/tcp")
	err = pool.Retry(func() error {
		//get admin access token
		form := url.Values{}
		form.Add("username", "sepl")
		form.Add("password", "sepl")
		form.Add("grant_type", "password")
		form.Add("client_id", "admin-cli")
		resp, err := http.Post(
			"http://"+keycloak.Container.NetworkSettings.IPAddress+":8080/auth/realms/master/protocol/openid-connect/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()))
		if err != nil {
			log.Println("unable to request admin token", err)
			return err
		}
		tokenMsg := map[string]interface{}{}
		err = json.NewDecoder(resp.Body).Decode(&tokenMsg)
		if err != nil {
			log.Println("unable to decode admin token", err)
			return err
		}
		return nil
	})
	return func() { keycloak.Close() }, hostPort, keycloak.Container.NetworkSettings.IPAddress, err
}

func Kafka(pool *dockertest.Pool, zookeeperUrl string) (closer func(), err error) {
	kafkaport, err := getFreePort()
	if err != nil {
		log.Fatalf("Could not find new port: %s", err)
	}
	networks, _ := pool.Client.ListNetworks()
	hostIp := ""
	for _, network := range networks {
		if network.Name == "bridge" {
			hostIp = network.IPAM.Config[0].Gateway
		}
	}
	log.Println("host ip: ", hostIp)
	env := []string{
		"ALLOW_PLAINTEXT_LISTENER=yes",
		"KAFKA_LISTENERS=OUTSIDE://:9092",
		"KAFKA_ADVERTISED_LISTENERS=OUTSIDE://" + hostIp + ":" + strconv.Itoa(kafkaport),
		"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=OUTSIDE:PLAINTEXT",
		"KAFKA_INTER_BROKER_LISTENER_NAME=OUTSIDE",
		"KAFKA_ZOOKEEPER_CONNECT=" + zookeeperUrl,
	}
	log.Println("start kafka with env ", env)
	kafkaContainer, err := pool.RunWithOptions(&dockertest.RunOptions{Repository: "bitnami/kafka", Tag: "latest", Env: env, PortBindings: map[docker.Port][]docker.PortBinding{
		"9092/tcp": {{HostIP: "", HostPort: strconv.Itoa(kafkaport)}},
	}})
	if err != nil {
		return func() {}, err
	}
	err = pool.Retry(func() error {
		log.Println("try kafka connection...")
		conn, err := kafka.Dial("tcp", hostIp+":"+strconv.Itoa(kafkaport))
		if err != nil {
			log.Println(err)
			return err
		}
		defer conn.Close()
		return nil
	})
	return func() { kafkaContainer.Close() }, err
}

func Zookeeper(pool *dockertest.Pool) (closer func(), hostPort string, ipAddress string, err error) {
	zkport, err := getFreePort()
	if err != nil {
		log.Fatalf("Could not find new port: %s", err)
	}
	env := []string{}
	log.Println("start zookeeper on ", zkport)
	zkContainer, err := pool.RunWithOptions(&dockertest.RunOptions{Repository: "wurstmeister/zookeeper", Tag: "latest", Env: env, PortBindings: map[docker.Port][]docker.PortBinding{
		"2181/tcp": {{HostIP: "", HostPort: strconv.Itoa(zkport)}},
	}})
	if err != nil {
		return func() {}, "", "", err
	}
	hostPort = strconv.Itoa(zkport)
	err = pool.Retry(func() error {
		log.Println("try zk connection...")
		zookeeper := kazoo.NewConfig()
		zk, chroot := kazoo.ParseConnectionString(zkContainer.Container.NetworkSettings.IPAddress)
		zookeeper.Chroot = chroot
		kz, err := kazoo.NewKazoo(zk, zookeeper)
		if err != nil {
			log.Println("kazoo", err)
			return err
		}
		_, err = kz.Brokers()
		if err != nil && strings.TrimSpace(err.Error()) != strings.TrimSpace("zk: node does not exist") {
			log.Println("brokers", err)
			return err
		}
		return nil
	})
	return func() { zkContainer.Close() }, hostPort, zkContainer.Container.NetworkSettings.IPAddress, err
}

func Amqp(pool *dockertest.Pool) (closer func(), hostPort string, ipAddress string, err error) {
	log.Println("start rabbitmq")
	rabbitmq, err := pool.Run("rabbitmq", "3-management", []string{})
	if err != nil {
		return func() {}, "", "", err
	}
	hostPort = rabbitmq.GetPort("5672/tcp")
	err = pool.Retry(func() error {
		log.Println("try amqp connection...")
		conn, err := amqp.Dial("amqp://guest:guest@" + rabbitmq.Container.NetworkSettings.IPAddress + ":5672/")
		if err != nil {
			return err
		}
		defer conn.Close()
		c, err := conn.Channel()
		defer c.Close()
		return err
	})
	return func() { rabbitmq.Close() }, hostPort, rabbitmq.Container.NetworkSettings.IPAddress, err
}

func Memcached(pool *dockertest.Pool) (closer func(), hostPort string, ipAddress string, err error) {
	log.Println("start memcached")
	mem, err := pool.Run("memcached", "1.5.12-alpine", []string{})
	if err != nil {
		return func() {}, "", "", err
	}
	hostPort = mem.GetPort("11211/tcp")
	err = pool.Retry(func() error {
		log.Println("try memcache connection...")
		_, err := memcache.New(mem.Container.NetworkSettings.IPAddress + ":11211").Get("foo")
		if err == memcache.ErrCacheMiss {
			return nil
		}
		if err != nil {
			log.Println(err)
		}
		return err
	})
	return func() { mem.Close() }, hostPort, mem.Container.NetworkSettings.IPAddress, err
}

func IotOntology(pool *dockertest.Pool) (closer func(), hostPort string, ipAddress string, err error) {
	log.Println("start iot ontology")
	onto, err := pool.Run("fgseitsrancher.wifa.intern.uni-leipzig.de:5000/iot-ontology", "unstable", []string{
		"DBA_PASSWORD=myDbaPassword",
		"DEFAULT_GRAPH=iot",
	})
	if err != nil {
		return func() {}, "", "", err
	}
	hostPort = onto.GetPort("8890/tcp")
	err = pool.Retry(func() error {
		log.Println("try onto connection...")
		db := ordf.Persistence{
			Endpoint:  "http://" + onto.Container.NetworkSettings.IPAddress + ":8890/sparql",
			Graph:     "iot",
			User:      "dba",
			Pw:        "myDbaPassword",
			SparqlLog: "false",
		}
		_, err := db.IdExists("something")
		if err != nil {
			log.Println(err)
		}
		return err
	})
	return func() { onto.Close() }, hostPort, onto.Container.NetworkSettings.IPAddress, err
}

func IotRepo(pool *dockertest.Pool, ontoIp string, amqpIp string, permsearchIp string) (closer func(), hostPort string, ipAddress string, err error) {
	log.Println("start iot repo")
	repo, err := pool.Run("fgseitsrancher.wifa.intern.uni-leipzig.de:5000/iot-device-repository", "unstable", []string{
		"SPARQL_ENDPOINT=" + "http://" + ontoIp + ":8890/sparql",
		"AMQP_URL=" + "amqp://guest:guest@" + amqpIp + ":5672/",
		"PERMISSIONS_URL=" + "http://" + permsearchIp + ":8080",
	})
	if err != nil {
		return func() {}, "", "", err
	}
	hostPort = repo.GetPort("8080/tcp")
	err = pool.Retry(func() error {
		log.Println("try repo connection...")
		_, err := http.Get("http://" + repo.Container.NetworkSettings.IPAddress + ":8080/deviceType/foo")
		if err != nil {
			log.Println(err)
		}
		return err
	})
	return func() { repo.Close() }, hostPort, repo.Container.NetworkSettings.IPAddress, err
}

func PermSearch(pool *dockertest.Pool, amqpIp string, elasticIp string) (closer func(), hostPort string, ipAddress string, err error) {
	log.Println("start permsearch")
	repo, err := pool.Run("fgseitsrancher.wifa.intern.uni-leipzig.de:5000/permissionsearch", "unstable", []string{
		"AMQP_URL=" + "amqp://guest:guest@" + amqpIp + ":5672/",
		"ELASTIC_URL=" + "http://" + elasticIp + ":9200",
	})
	if err != nil {
		return func() {}, "", "", err
	}
	hostPort = repo.GetPort("8080/tcp")
	err = pool.Retry(func() error {
		log.Println("try permsearch connection...")
		_, err := http.Get("http://" + repo.Container.NetworkSettings.IPAddress + ":8080/jwt/check/deviceinstance/foo/r/bool")
		if err != nil {
			log.Println(err)
		}
		return err
	})
	return func() { repo.Close() }, hostPort, repo.Container.NetworkSettings.IPAddress, err
}

func Elasticsearch(pool *dockertest.Pool) (closer func(), hostPort string, ipAddress string, err error) {
	log.Println("start elasticsearch")
	repo, err := pool.Run("docker.elastic.co/elasticsearch/elasticsearch", "6.4.3", []string{"discovery.type=single-node"})
	if err != nil {
		return func() {}, "", "", err
	}
	hostPort = repo.GetPort("9200/tcp")
	err = pool.Retry(func() error {
		log.Println("try elastic connection...")
		_, err := http.Get("http://" + repo.Container.NetworkSettings.IPAddress + ":9200/_cluster/health")
		if err != nil {
			log.Println(err)
		}
		return err
	})
	return func() { repo.Close() }, hostPort, repo.Container.NetworkSettings.IPAddress, err
}

func Vernemqtt(pool *dockertest.Pool, connecorUrl string) (closer func(), hostPort string, ipAddress string, err error) {
	log.Println("start vernemqtt")
	log.Println(strings.Join([]string{
		"DOCKER_VERNEMQ_LOG__CONSOLE__LEVEL=debug",
		"DOCKER_VERNEMQ_SHARED_SUBSCRIPTION_POLICY=random",
		"DOCKER_VERNEMQ_PLUGINS__VMQ_WEBHOOKS=on",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE__HOOK=auth_on_subscribe",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE__ENDPOINT=http://" + connecorUrl + "/subscribe",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH__HOOK=auth_on_publish",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH__ENDPOINT=http://" + connecorUrl + "/publish",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG__HOOK=auth_on_register",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG__ENDPOINT=http://" + connecorUrl + "/login",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLOFF__HOOK=on_client_offline",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLOFF__ENDPOINT=http://" + connecorUrl + "/disconnect",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR__HOOK=on_unsubscribe",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR__ENDPOINT=http://" + connecorUrl + "/unsubscribe",
		"DOCKER_VERNEMQ_PLUGINS__VMQ_PASSWD=off",
		"DOCKER_VERNEMQ_PLUGINS__VMQ_ACL=off",
	}, "\n"))
	repo, err := pool.Run("erlio/docker-vernemq", "latest", []string{
		"DOCKER_VERNEMQ_LOG__CONSOLE__LEVEL=debug",
		"DOCKER_VERNEMQ_SHARED_SUBSCRIPTION_POLICY=random",
		"DOCKER_VERNEMQ_PLUGINS__VMQ_WEBHOOKS=on",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE__HOOK=auth_on_subscribe",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLSUBSCRIBE__ENDPOINT=http://" + connecorUrl + "/subscribe",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH__HOOK=auth_on_publish",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLPUBLISH__ENDPOINT=http://" + connecorUrl + "/publish",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG__HOOK=auth_on_register",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLREG__ENDPOINT=http://" + connecorUrl + "/login",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLOFF__HOOK=on_client_offline",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLOFF__ENDPOINT=http://" + connecorUrl + "/disconnect",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR__HOOK=on_unsubscribe",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLUNSUBSCR__ENDPOINT=http://" + connecorUrl + "/unsubscribe",
		"DOCKER_VERNEMQ_PLUGINS__VMQ_PASSWD=off",
		"DOCKER_VERNEMQ_PLUGINS__VMQ_ACL=off",
	})
	if err != nil {
		return func() {}, "", "", err
	}
	hostPort = repo.GetPort("1883/tcp")
	time.Sleep(2 * time.Second)
	return func() { repo.Close() }, hostPort, repo.Container.NetworkSettings.IPAddress, err
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
