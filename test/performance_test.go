package test

import (
	"github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/cache"
	"github.com/SENERGY-Platform/platform-connector-lib/kafka"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/client"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server"
	"log"
	"os"
	"path/filepath"
	"runtime/trace"
	"sync"
	"testing"
	"time"
)

var testPerformance = false

func Test100p(t *testing.T) {
	if !testPerformance {
		t.Skip("set testPerformance = true")
	}
	log.Println(test_n(100, true, t, true, true, "./trace/trace_100p.out"))
}

func Test100px2(t *testing.T) {
	if !testPerformance {
		t.Skip("set testPerformance = true")
	}
	log.Println(test_n(100, true, t, true, true, "./trace/trace_100px2_1.out", "./trace/trace_100px2_2.out"))
}

func Test1000p(t *testing.T) {
	if !testPerformance {
		t.Skip("set testPerformance = true")
	}
	log.Println(test_n(1000, true, t, true, true, "./trace/trace_1000p.out"))
}

func Test1000pConf(t *testing.T) {
	if !testPerformance {
		t.Skip("set testPerformance = true")
	}
	results := [][]map[string]time.Duration{}
	results = append(results, test_n(1000, true, t, true, true, "./trace/trace_1000pConf_tt.out"))
	results = append(results, test_n(1000, true, t, true, false, "./trace/trace_1000pConf_tf.out"))
	results = append(results, test_n(1000, true, t, false, false, "./trace/trace_1000pConf_ff.out"))
	log.Println(results)
}

func Test1000px2(t *testing.T) {
	if !testPerformance {
		t.Skip("set testPerformance = true")
	}
	log.Println(test_n(1000, true, t, true, true, "./trace/trace_1000px2_1.out", "./trace/trace_1000px2_2.out"))
}

func Test1000(t *testing.T) {
	if !testPerformance {
		t.Skip("set testPerformance = true")
	}
	log.Println(test_n(1000, false, t, true, true, "./trace/trace_1000.out"))
}

func Test10000(t *testing.T) {
	if !testPerformance {
		t.Skip("set testPerformance = true")
	}
	log.Println(test_n(10000, false, t, true, true, "./trace/trace_10000.out"))
}

func Test10000p(t *testing.T) {
	if !testPerformance {
		t.Skip("set testPerformance = true")
	}
	log.Println(test_n(10000, true, t, true, true, "./trace/trace_10000p.out"))
}

func test_n(n int, parallel bool, t *testing.T, syncProd bool, idempotent bool, tracefiles ...string) (times []map[string]time.Duration) {
	config, err := lib.LoadConfig("../config.json")
	if err != nil {
		t.Error(err)
		return times
	}
	cache.Debug = true
	config.KafkaEventTopic = ""
	config.Debug = true
	config.MqttPublishAuthOnly = false
	config.SyncKafka = syncProd
	config.SyncKafkaIdempotent = idempotent
	config, shutdown, err := server.New(config)
	if err != nil {
		t.Error(err)
		return times
	}
	if true {
		defer shutdown()
	}

	time.Sleep(2 * time.Second)

	c, err := client.New(config.MqttBroker, config.IotRepoUrl, config.DeviceRepoUrl, config.AuthEndpoint, "sepl", "sepl", "", "testname", []client.DeviceRepresentation{
		{
			Name:    "test1",
			Uri:     "test1",
			IotType: "iot#80550847-a151-4de4-806a-50503b2fdf62",
			Tags:    []string{},
		},
	})
	if err != nil {
		t.Error(err)
		return times
	}

	defer c.Stop()

	for _, tracefile := range tracefiles {
		var sendTime time.Duration
		var firstConsumeTime time.Duration
		var allConsumedTime time.Duration
		wait := sync.WaitGroup{}
		wait.Add(3)
		err = os.MkdirAll(filepath.Dir(tracefile), os.ModePerm)
		if err != nil {
			t.Error(err)
			return times
		}

		file, err := os.Create(tracefile)
		if err != nil {
			t.Error(err)
			return times
		}
		defer file.Close()

		first := make(chan bool, n+1)
		if config.MqttPublishAuthOnly {
			first <- true
		}
		all := sync.WaitGroup{}
		if !config.MqttPublishAuthOnly {
			all.Add(n)
		}

		consumer, err := kafka.NewConsumer(config.ZookeeperUrl, "stress-test-consumer", "iot_dc3c326c-8420-4af1-be0d-dcabfdacc90e", func(topic string, msg []byte) error {
			log.Println("consumed: ", topic)
			first <- true
			all.Done()
			return nil
		}, func(err error, consumer *kafka.Consumer) {
			log.Println(err)
			t.Error(err)
		})
		if err != nil {
			t.Error(err)
			return
		}

		starttime := time.Now()

		go func() {
			<-first
			firstConsumeTime = time.Now().Sub(starttime)
			wait.Done()
		}()

		go func() {
			all.Wait()
			allConsumedTime = time.Now().Sub(starttime)
			wait.Done()
		}()

		go func() {
			err = trace.Start(file)
			if err != nil {
				t.Error(err)
				return
			}
			waitAllSend := sync.WaitGroup{}
			for i := 0; i < n; i++ {
				if parallel {
					waitAllSend.Add(1)
					go func() {
						err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})
						if err != nil {
							t.Error(err)
						}
						waitAllSend.Done()
					}()
				} else {
					err = c.SendEvent("test1", "sepl_get", map[platform_connector_lib.ProtocolSegmentName]string{"metrics": `{"level": 42, "title": "event", "updateTime": 0}`})
					if err != nil {
						t.Error(err)
						waitAllSend.Done()
						return
					}
				}
			}
			if parallel {
				waitAllSend.Wait()
			}
			trace.Stop()
			sendTime = time.Now().Sub(starttime)
			wait.Done()
		}()

		wait.Wait()
		consumer.Stop()
		times = append(times, map[string]time.Duration{"sendTime": sendTime, "firstConsumeTime": firstConsumeTime, "allConsumedTime": allConsumedTime})
		time.Sleep(2 * time.Second)
	}

	return times
}
