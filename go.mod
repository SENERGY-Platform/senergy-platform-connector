module github.com/SENERGY-Platform/senergy-platform-connector

require (
	github.com/SENERGY-Platform/platform-connector-lib v0.0.0-20191104133852-4fa377bf7e4f
	github.com/bradfitz/gomemcache v0.0.0-20180710155616-bc664df96737
	github.com/eclipse/paho.mqtt.golang v1.1.1
	github.com/ory/dockertest v3.3.4+incompatible
	github.com/segmentio/kafka-go v0.2.5
	github.com/wvanbergen/kazoo-go v0.0.0-20180202103751-f72d8611297a
	go.mongodb.org/mongo-driver v1.1.2
)

//uncomment to test local changes of platform-connector-lib
//replace github.com/SENERGY-Platform/platform-connector-lib => ../platform-connector-lib
