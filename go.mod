module gilhub.com/SENERGY-Platform/senergy-platform-connector

require (
	github.com/SENERGY-Platform/iot-device-repository v0.0.0-20190411071940-270889a26f60
	github.com/SENERGY-Platform/platform-connector-lib v0.0.0-20190425131945-df4f43b12c4f
	github.com/bradfitz/gomemcache v0.0.0-20180710155616-bc664df96737
	github.com/eclipse/paho.mqtt.golang v1.1.1
	github.com/ory/dockertest v3.3.4+incompatible
	github.com/satori/go.uuid v1.2.0
	github.com/segmentio/kafka-go v0.2.2
	github.com/streadway/amqp v0.0.0-20180315184602-8e4aba63da9f
	github.com/wvanbergen/kazoo-go v0.0.0-20180202103751-f72d8611297a
)

//uncomment to test local changes of platform-connector-lib
//replace github.com/SENERGY-Platform/platform-connector-lib => ../platform-connector-lib
