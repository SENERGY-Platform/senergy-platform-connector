module github.com/SENERGY-Platform/senergy-platform-connector

go 1.17

require (
	github.com/SENERGY-Platform/platform-connector-lib v0.0.0-20210928101452-22b3fa0c424b
	github.com/bradfitz/gomemcache v0.0.0-20180710155616-bc664df96737
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/eclipse/paho.mqtt.golang v1.1.1
	github.com/gotestyourself/gotestyourself v2.2.0+incompatible // indirect
	github.com/julienschmidt/httprouter v1.3.0
	github.com/lib/pq v1.4.0
	github.com/ory/dockertest v3.3.4+incompatible
	github.com/satori/go.uuid v1.2.0
	github.com/segmentio/kafka-go v0.4.15
	github.com/wvanbergen/kazoo-go v0.0.0-20180202103751-f72d8611297a
	gotest.tools v2.2.0+incompatible // indirect
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/DataDog/zstd v1.4.0 // indirect
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/Shopify/sarama v1.22.0 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/clbanning/mxj v1.8.4 // indirect
	github.com/containerd/continuity v0.0.0-20200413184840-d3ef23f19fbb // indirect
	github.com/coocood/freecache v1.1.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/eapache/go-resiliency v1.1.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20180814174437-776d5712da21 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/golang-jwt/jwt v3.2.1+incompatible // indirect
	github.com/golang/snappy v0.0.1 // indirect
	github.com/klauspost/compress v1.9.8 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v1.0.0-rc95 // indirect
	github.com/pierrec/lz4 v2.6.0+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20181016184325-3113b8401b8a // indirect
	github.com/samuel/go-zookeeper v0.0.0-20190923202752-2cc03de413da // indirect
	github.com/sirupsen/logrus v1.7.0 // indirect
	golang.org/x/net v0.0.0-20201224014010-6772e930b67b // indirect
	golang.org/x/sys v0.0.0-20210426230700-d19ff857e887 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
)

//uncomment to test local changes of platform-connector-lib
//replace github.com/SENERGY-Platform/platform-connector-lib => ../platform-connector-lib
