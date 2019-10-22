module github.com/SENERGY-Platform/senergy-platform-connector

require (
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/SENERGY-Platform/platform-connector-lib v0.0.0-20190910084656-7e809c6a4989
	github.com/bradfitz/gomemcache v0.0.0-20180710155616-bc664df96737
	github.com/cenkalti/backoff v2.1.1+incompatible // indirect
	github.com/containerd/continuity v0.0.0-20190426062206-aaeac12a7ffc // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/eclipse/paho.mqtt.golang v1.1.1
	github.com/go-stack/stack v1.8.0 // indirect
	github.com/google/go-cmp v0.3.0 // indirect
	github.com/gotestyourself/gotestyourself v2.2.0+incompatible // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/lib/pq v1.2.0 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/ory/dockertest v3.3.4+incompatible
	github.com/segmentio/kafka-go v0.2.5
	github.com/sirupsen/logrus v1.4.2 // indirect
	github.com/streadway/amqp v0.0.0-20180315184602-8e4aba63da9f
	github.com/tidwall/pretty v1.0.0 // indirect
	github.com/wvanbergen/kazoo-go v0.0.0-20180202103751-f72d8611297a
	go.mongodb.org/mongo-driver v1.0.3
	golang.org/x/sync v0.0.0-20190423024810-112230192c58 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gotest.tools v2.2.0+incompatible // indirect
)

//uncomment to test local changes of platform-connector-lib
//replace github.com/SENERGY-Platform/platform-connector-lib => ../platform-connector-lib
