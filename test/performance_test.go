package test

import (
	"gilhub.com/SENERGY-Platform/senergy-platform-connector/lib"
	"gilhub.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"gilhub.com/SENERGY-Platform/senergy-platform-connector/test/server"
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/kafka"
	"log"
	"os"
	"path/filepath"
	"runtime/trace"
	"sync"
	"testing"
	"time"
)

func Test100p(t *testing.T) {
	log.Println(test_n(100, true, t, "./trace/trace_100p.out"))
}

func Test1000p(t *testing.T) {
	log.Println(test_n(1000, true, t, "./trace/trace_1000p.out"))
}

func Test1000(t *testing.T) {
	log.Println(test_n(1000, false, t, "./trace/trace_1000.out"))
}

func Test10000(t *testing.T) {
	log.Println(test_n(10000, false, t, "./trace/trace_10000.out"))
}

func test_n(n int, parallel bool, t *testing.T, tracefile string) time.Duration {
	config, err := lib.LoadConfig("../config.json")
	if err != nil {
		t.Error(err)
		return 0
	}
	config, shutdown, err := server.New(config)
	if err != nil {
		t.Error(err)
		return 0
	}
	if true {
		defer shutdown()
	}

	err = kafka.InitTopic(config.ZookeeperUrl, config.KafkaEventTopic)
	if err != nil {
		t.Error(err)
		return 0
	}

	err = kafka.InitTopic(config.ZookeeperUrl, "iot_dc3c326c-8420-4af1-be0d-dcabfdacc90e")
	if err != nil {
		t.Error(err)
		return 0
	}

	time.Sleep(2 * time.Second)

	c, err := client.New(config.MqttBroker, config.IotRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{
		{
			Name:    "test1",
			Uri:     "test1",
			IotType: "iot#80550847-a151-4de4-806a-50503b2fdf62",
			Tags:    []string{},
		},
	})
	if err != nil {
		t.Error(err)
		return 0
	}

	defer c.Stop()

	err = os.MkdirAll(filepath.Dir(tracefile), os.ModePerm)
	if err != nil {
		t.Error(err)
		return 0
	}

	file, err := os.Create(tracefile)
	if err != nil {
		t.Error(err)
		return 0
	}
	defer file.Close()

	starttime := time.Now()
	err = trace.Start(file)
	if err != nil {
		t.Error(err)
		return 0
	}
	wait := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		if parallel {
			wait.Add(1)
			go func() {
				err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})
				if err != nil {
					t.Error(err)
				}
				wait.Done()
			}()
		} else {
			err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})
			if err != nil {
				t.Error(err)
				return time.Now().Sub(starttime)
			}
		}
	}
	if parallel {
		wait.Wait()
	}
	trace.Stop()
	return time.Now().Sub(starttime)
}