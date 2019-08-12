module github.com/SENERGY-Platform/senergy-platform-connector

require (
	github.com/SENERGY-Platform/iot-device-repository v0.0.0-20190411071940-270889a26f60
	github.com/SENERGY-Platform/platform-connector-lib v0.0.0-20190808083051-a098dc2d14ef
	github.com/bradfitz/gomemcache v0.0.0-20180710155616-bc664df96737
	github.com/eclipse/paho.mqtt.golang v1.1.1
	github.com/go-stack/stack v1.8.0 // indirect
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/gotestyourself/gotestyourself v2.2.0+incompatible // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/lib/pq v1.2.0 // indirect
	github.com/onsi/ginkgo v1.8.0 // indirect
	github.com/onsi/gomega v1.5.0 // indirect
	github.com/ory/dockertest v3.3.4+incompatible
	github.com/segmentio/kafka-go v0.2.2
	github.com/streadway/amqp v0.0.0-20180315184602-8e4aba63da9f
	github.com/tidwall/pretty v1.0.0 // indirect
	github.com/wvanbergen/kazoo-go v0.0.0-20180202103751-f72d8611297a
	go.mongodb.org/mongo-driver v1.0.2
	golang.org/x/sync v0.0.0-20190423024810-112230192c58 // indirect
	gopkg.in/airbrake/gobrake.v2 v2.0.9 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/gemnasium/logrus-airbrake-hook.v2 v2.1.2 // indirect
	gotest.tools v2.2.0+incompatible // indirect
)

//uncomment to test local changes of platform-connector-lib
//replace github.com/SENERGY-Platform/platform-connector-lib => ../platform-connector-lib
