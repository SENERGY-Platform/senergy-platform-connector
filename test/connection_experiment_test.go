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

package test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	testdocker "github.com/SENERGY-Platform/senergy-platform-connector/test/server/docker"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	uuid "github.com/satori/go.uuid"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime/debug"
	"sync"
	"testing"
	"time"
)

func TestConnectionExperiment2(t *testing.T) {
	t.Skip("only experiment")
	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	defer cancel()

	topic := "test/topic"
	mgmtUrl := ""

	router := http.NewServeMux()
	router.HandleFunc("/health", func(writer http.ResponseWriter, request *http.Request) {
		log.Println("INFO: /health received")
		msg, err := ioutil.ReadAll(request.Body)
		log.Println("INFO: /health body =", err, string(msg))
		writer.WriteHeader(http.StatusOK)
	})

	router.HandleFunc("/publish", func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprint(writer, `{"result": "ok"}`)
	})

	router.HandleFunc("/subscribe", func(writer http.ResponseWriter, request *http.Request) {
		LogTopicSubscriptions("while subscribe", mgmtUrl, topic)
		LogTopicClients("while subscribe", mgmtUrl, "test")
		fmt.Fprint(writer, `{"result": "ok"}`)
	})

	router.HandleFunc("/login", func(writer http.ResponseWriter, request *http.Request) {
		LogTopicSubscriptions("while login", mgmtUrl, topic)
		LogTopicClients("while login", mgmtUrl, "test")
		fmt.Fprint(writer, `{"result": "ok"}`)
	})

	router.HandleFunc("/wakeup", func(writer http.ResponseWriter, request *http.Request) {
		LogTopicSubscriptions("while wakeup", mgmtUrl, topic)
		LogTopicClients("while wakeup", mgmtUrl, "test")
		fmt.Fprintf(writer, "{}")
	})

	router.HandleFunc("/reg", func(writer http.ResponseWriter, request *http.Request) {
		LogTopicSubscriptions("while reg", mgmtUrl, topic)
		LogTopicClients("while reg", mgmtUrl, "test")
		fmt.Fprintf(writer, "{}")
	})

	//https://vernemq.com/docs/plugindevelopment/sessionlifecycle.html
	router.HandleFunc("/disconnect", func(writer http.ResponseWriter, request *http.Request) {
		LogTopicSubscriptions("while disconnect", mgmtUrl, topic)
		LogTopicClients("while disconnect", mgmtUrl, "test")
		fmt.Fprintf(writer, "{}")
	})

	router.HandleFunc("/gone", func(writer http.ResponseWriter, request *http.Request) {
		LogTopicSubscriptions("while gone", mgmtUrl, topic)
		LogTopicClients("while gone", mgmtUrl, "test")
		fmt.Fprintf(writer, "{}")
	})

	router.HandleFunc("/unsubscribe", func(writer http.ResponseWriter, request *http.Request) {
		defer json.NewEncoder(writer).Encode(map[string]interface{}{"result": "ok"})
		LogTopicSubscriptions("while unsubscribe", mgmtUrl, topic)
		LogTopicClients("while unsubscribe", mgmtUrl, "test")
	})

	var err error
	var brokerUrl string

	brokerUrl, mgmtUrl, err = experimentSetup(ctx, wg, router)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(2 * time.Second)

	c1 := paho.NewClient(paho.NewClientOptions().
		SetClientID("test").
		SetAutoReconnect(false).
		SetCleanSession(true).
		AddBroker(brokerUrl))
	if token := c1.Connect(); token.Wait() && token.Error() != nil {
		log.Println("Error on Client.Connect(): ", token.Error())
		t.Error(err)
		return
	}

	LogTopicSubscriptions("before subscribe", mgmtUrl, topic)
	LogTopicClients("before subscribe", mgmtUrl, "test")

	token := c1.Subscribe(topic, 2, func(client paho.Client, message paho.Message) {})
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return
	}

	LogTopicSubscriptions("before new client", mgmtUrl, topic)
	LogTopicClients("before new client", mgmtUrl, "test")

	c2 := paho.NewClient(paho.NewClientOptions().
		SetClientID("test").
		SetAutoReconnect(false).
		SetCleanSession(true).
		AddBroker(brokerUrl))
	if token := c2.Connect(); token.Wait() && token.Error() != nil {
		log.Println("Error on Client.Connect(): ", token.Error())
		t.Error(err)
		return
	}

	LogTopicSubscriptions("before new subscribe", mgmtUrl, topic)
	LogTopicClients("before new subscribe", mgmtUrl, "test")

	token = c2.Subscribe(topic, 2, func(client paho.Client, message paho.Message) {})
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return
	}

	LogTopicSubscriptions("after new subscribe", mgmtUrl, topic)
	LogTopicClients("after new subscribe", mgmtUrl, "test")

	token = c2.Unsubscribe(topic)
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return
	}

	LogTopicSubscriptions("after unsubscribe", mgmtUrl, topic)
	LogTopicClients("after unsubscribe", mgmtUrl, "test")

	c2.Disconnect(0)

	time.Sleep(2 * time.Second)
}

func TestConnectionExperiment(t *testing.T) {
	t.Skip("only experiment")
	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	defer cancel()

	topic := "test/topic"
	mgmtUrl := ""

	router := http.NewServeMux()
	router.HandleFunc("/health", func(writer http.ResponseWriter, request *http.Request) {
		log.Println("INFO: /health received")
		msg, err := ioutil.ReadAll(request.Body)
		log.Println("INFO: /health body =", err, string(msg))
		writer.WriteHeader(http.StatusOK)
	})

	router.HandleFunc("/publish", func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprint(writer, `{"result": "ok"}`)
	})

	router.HandleFunc("/subscribe", func(writer http.ResponseWriter, request *http.Request) {
		LogTopicSubscriptions("while subscribe", mgmtUrl, topic)
		fmt.Fprint(writer, `{"result": "ok"}`)
	})

	router.HandleFunc("/login", func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprint(writer, `{"result": "ok"}`)
	})

	//https://vernemq.com/docs/plugindevelopment/sessionlifecycle.html
	router.HandleFunc("/disconnect", func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprintf(writer, "{}")
	})

	router.HandleFunc("/unsubscribe", func(writer http.ResponseWriter, request *http.Request) {
		defer json.NewEncoder(writer).Encode(map[string]interface{}{"result": "ok"})
		LogTopicSubscriptions("while unsubscribe", mgmtUrl, topic)
	})

	var err error
	var brokerUrl string

	brokerUrl, mgmtUrl, err = experimentSetup(ctx, wg, router)
	if err != nil {
		t.Error(err)
		return
	}

	time.Sleep(2 * time.Second)

	c1 := paho.NewClient(paho.NewClientOptions().
		SetClientID("test").
		SetAutoReconnect(true).
		SetCleanSession(false).
		AddBroker(brokerUrl))
	if token := c1.Connect(); token.Wait() && token.Error() != nil {
		log.Println("Error on Client.Connect(): ", token.Error())
		t.Error(err)
		return
	}

	c2 := paho.NewClient(paho.NewClientOptions().
		SetClientID("test2").
		SetAutoReconnect(true).
		SetCleanSession(false).
		AddBroker(brokerUrl))
	if token := c2.Connect(); token.Wait() && token.Error() != nil {
		log.Println("Error on Client.Connect(): ", token.Error())
		t.Error(err)
		return
	}

	LogTopicSubscriptions("before subscribe", mgmtUrl, topic)

	token := c1.Subscribe(topic, 2, func(client paho.Client, message paho.Message) {})
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return
	}

	LogTopicSubscriptions("before unsubscribe", mgmtUrl, topic)

	token = c1.Unsubscribe(topic)
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return
	}
	LogTopicSubscriptions("after unsubscribe", mgmtUrl, topic)
	time.Sleep(1 * time.Second)
	LogTopicSubscriptions("1s after unsubscribe", mgmtUrl, topic)

	token = c1.Subscribe(topic, 2, func(client paho.Client, message paho.Message) {})
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return
	}
	token = c2.Subscribe(topic, 2, func(client paho.Client, message paho.Message) {})
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return
	}

	LogTopicSubscriptions("2 subscriptions", mgmtUrl, topic)

	token = c1.Unsubscribe(topic)
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return
	}
	LogTopicSubscriptions("after c1 unsubscribe", mgmtUrl, topic)

	token = c2.Unsubscribe(topic)
	if token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		return
	}
	LogTopicSubscriptions("after c2 unsubscribe", mgmtUrl, topic)
}

func LogTopicSubscriptions(logprefix string, mgmtUrl string, topic string) {
	path := "/api/v1/session/show?--is_online=true&--topic=" + url.QueryEscape(topic) //+ "&--limit=1"
	req, err := http.NewRequest("GET", mgmtUrl+path, nil)
	if err != nil {
		log.Println(logprefix+" ERROR:", err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println(logprefix+" ERROR:", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		buf, _ := ioutil.ReadAll(resp.Body)
		err = errors.New(resp.Status + ":" + string(buf))
		log.Println(logprefix+" ERROR:", err)
		log.Println(mgmtUrl + path)
		return
	}
	temp := SubscriptionWrapper{}
	err = json.NewDecoder(resp.Body).Decode(&temp)
	if err != nil {
		log.Println(logprefix+" ERROR:", err)
		return
	}
	log.Println("SUBSCRIPTIONS:", logprefix, temp.Table)

	time.Sleep(2 * time.Second)
}

func LogTopicClients(logprefix string, mgmtUrl string, clientId string) {
	path := "/api/v1/session/show?--is_online=true&--client_id=" + url.QueryEscape(clientId)
	req, err := http.NewRequest("GET", mgmtUrl+path, nil)
	if err != nil {
		log.Println(logprefix+" ERROR:", err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println(logprefix+" ERROR:", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		buf, _ := ioutil.ReadAll(resp.Body)
		err = errors.New(resp.Status + ":" + string(buf))
		log.Println(logprefix+" ERROR:", err)
		log.Println(mgmtUrl + path)
		return
	}
	temp := SubscriptionWrapper{}
	err = json.NewDecoder(resp.Body).Decode(&temp)
	if err != nil {
		log.Println(logprefix+" ERROR:", err)
		return
	}
	log.Println("CLIENTS:", logprefix, temp.Table)

	time.Sleep(2 * time.Second)
}

type Subscription struct {
	ClientId string `json:"client_id"`
	User     string `json:"user"`
	Topic    string `json:"topic"`
}

type SubscriptionWrapper struct {
	Table []Subscription `json:"table"`
}

func experimentSetup(basectx context.Context, wg *sync.WaitGroup, webhookhandler http.Handler) (brokerUrl string, mgmtUrl string, err error) {
	ctx, cancel := context.WithCancel(basectx)

	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Println("Could not connect to docker: ", err)
		return brokerUrl, mgmtUrl, err
	}

	s := &httptest.Server{
		Config: &http.Server{Handler: webhookhandler},
	}
	s.Listener, _ = net.Listen("tcp", ":")
	s.Start()

	wg.Add(1)
	go func() {
		<-ctx.Done()
		s.Close()
		wg.Done()
	}()

	network, err := pool.Client.NetworkInfo("bridge")
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		cancel()
		return brokerUrl, mgmtUrl, err
	}
	hostIp := network.IPAM.Config[0].Gateway
	parsedTestserverUrl, err := url.Parse(s.URL)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		cancel()
		return brokerUrl, mgmtUrl, err
	}
	testserverDockerAddress := hostIp + ":" + parsedTestserverUrl.Port()
	log.Println(testserverDockerAddress)

	brokerUrl, mgmtUrl, err = VernemqWithManagementApi(pool, ctx, wg, testserverDockerAddress)
	if err != nil {
		log.Println("ERROR:", err)
		debug.PrintStack()
		cancel()
		return brokerUrl, mgmtUrl, err
	}

	return brokerUrl, mgmtUrl, err
}

func VernemqWithManagementApi(pool *dockertest.Pool, ctx context.Context, wg *sync.WaitGroup, connecorUrl string) (brokerUrl string, managementUrl string, err error) {
	log.Println("start mqtt")
	container, err := pool.Run("erlio/docker-vernemq", "1.9.1-alpine", []string{
		//"DOCKER_VERNEMQ_ALLOW_ANONYMOUS=on",
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

		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLGONE__HOOK=on_client_gone",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLGONE__ENDPOINT=http://" + connecorUrl + "/gone",

		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLONREG__HOOK=on_register",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLONREG__ENDPOINT=http://" + connecorUrl + "/reg",

		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLWAKE__HOOK=on_client_wakeup",
		"DOCKER_VERNEMQ_VMQ_WEBHOOKS__SEPLWAKE__ENDPOINT=http://" + connecorUrl + "/wakeup",
	})
	if err != nil {
		return "", "", err
	}
	go Dockerlog(pool, ctx, container, "VERNEMQ")
	wg.Add(1)
	go func() {
		<-ctx.Done()
		log.Println("DEBUG: remove container " + container.Container.Name)
		container.Close()
		wg.Done()
	}()
	err = pool.Retry(func() error {
		log.Println("DEBUG: try to connection to broker")
		options := paho.NewClientOptions().
			SetAutoReconnect(true).
			SetCleanSession(false).
			SetClientID(uuid.NewV4().String()).
			AddBroker("tcp://" + container.Container.NetworkSettings.IPAddress + ":1883")

		client := paho.NewClient(options)
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			log.Println("Error on Mqtt.Connect(): ", token.Error())
			return token.Error()
		}
		defer client.Disconnect(0)
		return nil
	})
	execLog := &testdocker.LogWriter{Logger: log.New(os.Stdout, "[VERNEMQ-EXEC]", 0)}
	code, err := container.Exec([]string{"vmq-admin", "api-key", "add", "key=testkey"}, dockertest.ExecOptions{StdErr: execLog, StdOut: execLog})
	if err != nil {
		log.Panic("ERROR", code, err)
		return "", "", err
	}
	return "tcp://" + container.Container.NetworkSettings.IPAddress + ":1883", "http://testkey@" + container.Container.NetworkSettings.IPAddress + ":8888", err
}

func Dockerlog(pool *dockertest.Pool, ctx context.Context, repo *dockertest.Resource, name string) {
	out := &testdocker.LogWriter{Logger: log.New(os.Stdout, "["+name+"]", 0)}
	err := pool.Client.Logs(docker.LogsOptions{
		Stdout:       true,
		Stderr:       true,
		Context:      ctx,
		Container:    repo.Container.ID,
		Follow:       true,
		OutputStream: out,
		ErrorStream:  out,
	})
	if err != nil && err != context.Canceled {
		log.Println("DEBUG-ERROR: unable to start docker log", name, err)
	}
}
