module github.com/SENERGY-Platform/senergy-platform-connector

require (
	github.com/SENERGY-Platform/iot-device-repository v0.0.0-20190411071940-270889a26f60
	github.com/SENERGY-Platform/platform-connector-lib v0.0.0-20190724063317-f63f04f4269d
	github.com/SmartEnergyPlatform/formatter-lib v0.0.0-20181018082014-b45c9317bb5e // indirect
	github.com/bradfitz/gomemcache v0.0.0-20180710155616-bc664df96737
	github.com/eclipse/paho.mqtt.golang v1.1.1
	github.com/go-stack/stack v1.8.0 // indirect
	github.com/ory/dockertest v3.3.4+incompatible
	github.com/satori/go.uuid v1.2.0
	github.com/segmentio/kafka-go v0.2.2
	github.com/streadway/amqp v0.0.0-20180315184602-8e4aba63da9f
	github.com/wvanbergen/kafka v0.0.0-20171203153745-e2edea948ddf // indirect
	github.com/wvanbergen/kazoo-go v0.0.0-20180202103751-f72d8611297a
	go.mongodb.org/mongo-driver v1.0.2
	golang.org/x/sync v0.0.0-20190423024810-112230192c58 // indirect
)

//uncomment to test local changes of platform-connector-lib
replace github.com/SENERGY-Platform/platform-connector-lib => ../platform-connector-lib
