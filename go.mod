module github.com/SENERGY-Platform/senergy-platform-connector

go 1.13

require (
	github.com/SENERGY-Platform/platform-connector-lib v0.0.0-20200312114332-3744f408d183
	github.com/bradfitz/gomemcache v0.0.0-20180710155616-bc664df96737
	github.com/eclipse/paho.mqtt.golang v1.1.1
	github.com/julienschmidt/httprouter v1.3.0
	github.com/ory/dockertest v3.3.4+incompatible
	github.com/satori/go.uuid v1.2.0
	github.com/segmentio/kafka-go v0.2.5
	github.com/wvanbergen/kazoo-go v0.0.0-20180202103751-f72d8611297a
	go.mongodb.org/mongo-driver v1.1.2
)

//uncomment to test local changes of platform-connector-lib
//replace github.com/SENERGY-Platform/platform-connector-lib => ../platform-connector-lib
