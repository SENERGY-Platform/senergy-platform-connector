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

package configuration

import (
	"encoding/json"
	"log"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/segmentio/kafka-go"
)

type ConfigStruct struct {
	KafkaUrl           string
	KafkaResponseTopic string
	KafkaGroupName     string
	FatalKafkaError    bool // "true" || "false"; "" -> "true", else -> "false"
	Protocol           string

	DeviceManagerUrl string
	DeviceRepoUrl    string

	AuthClientId             string `config:"secret"` //keycloak-client
	AuthClientSecret         string `config:"secret"` //keycloak-secret
	AuthExpirationTimeBuffer float64
	AuthEndpoint             string

	JwtPrivateKey string `config:"secret"`
	JwtExpiration int64
	JwtIssuer     string `config:"secret"`

	WebhookPort string

	GatewayLogTopic  string
	DeviceLogTopic   string
	ConnectionLogQos int

	MemcachedUrl string

	MqttBroker  string
	MqttVersion string

	CheckHub bool

	Debug     bool
	MqttDebug bool

	CorrelationExpiration int64

	IotCacheUrls             string
	DeviceExpiration         int64
	DeviceTypeExpiration     int64
	CharacteristicExpiration int64

	TokenCacheUrls       string `config:"secret"`
	TokenCacheExpiration int64

	MqttPublishAuthOnly bool

	StartupDelay int64

	Log            string
	WebhookTimeout int64

	Validate                  bool
	ValidateAllowUnknownField bool
	ValidateAllowMissingField bool

	FogHandlerTopicPrefix string

	KafkaPartitionNum      int
	KafkaReplicationFactor int

	ForceCleanSession         bool
	CleanSessionAllowUserList []string

	PublishToPostgres bool
	PostgresHost      string
	PostgresPort      int
	PostgresUser      string `config:"secret"`
	PostgresPw        string `config:"secret"`
	PostgresDb        string

	HttpCommandConsumerPort string

	AsyncPgThreadMax    int64
	AsyncFlushMessages  int64
	AsyncFlushFrequency string
	AsyncCompression    string
	SyncCompression     string

	KafkaConsumerMaxWait  string
	KafkaConsumerMinBytes int64
	KafkaConsumerMaxBytes int64

	IotCacheMaxIdleConns    int64
	CorrelationMaxIdleConns int64
	IotCacheTimeout         string
	CorrelationTimeout      string

	CommandWorkerCount int64

	DeviceTypeTopic string

	NotificationUrl  string
	PermissionsV2Url string

	NotificationsIgnoreDuplicatesWithinS int
	NotificationUserOverwrite            string
	DeveloperNotificationUrl             string

	MqttErrorOnEventValidationError bool

	KafkaTopicConfigs map[string][]kafka.ConfigEntry

	MqttAuthMethod string // Whether the MQTT broker uses a username/password or client certificate authetication

	ConnectionLimitCount             int
	ConnectionLimitDurationInSeconds int

	ForceCommandSubscriptionServiceSingleLevelWildcard bool

	SecRemoteProtocol string

	ForceTopicsWithOwner bool

	MutedUserNotificationTitles []string

	ConnectionCheckUrl         string
	ConnectionCheckHttpTimeout string

	ApiDocsProviderBaseUrl string

	VmqAdminApiUrl         string
	DisconnectCommandDelay string

	InitTopics bool
}

type Config = *ConfigStruct

func LoadConfig(location string) (config Config, err error) {
	file, error := os.Open(location)
	if error != nil {
		log.Println("error on config load: ", error)
		return config, error
	}
	decoder := json.NewDecoder(file)
	error = decoder.Decode(&config)
	if error != nil {
		log.Println("invalid config json: ", error)
		return config, error
	}
	HandleEnvironmentVars(config)
	return config, nil
}

var camel = regexp.MustCompile("(^[^A-Z]*|[A-Z]*)([A-Z][^A-Z]+|$)")

func fieldNameToEnvName(s string) string {
	var a []string
	for _, sub := range camel.FindAllStringSubmatch(s, -1) {
		if sub[1] != "" {
			a = append(a, sub[1])
		}
		if sub[2] != "" {
			a = append(a, sub[2])
		}
	}
	return strings.ToUpper(strings.Join(a, "_"))
}

// preparations for docker
func HandleEnvironmentVars(config Config) {
	configValue := reflect.Indirect(reflect.ValueOf(config))
	configType := configValue.Type()
	for index := 0; index < configType.NumField(); index++ {
		fieldName := configType.Field(index).Name
		fieldConfig := configType.Field(index).Tag.Get("config")
		envName := fieldNameToEnvName(fieldName)
		envValue := os.Getenv(envName)
		if envValue != "" {
			if !strings.Contains(fieldConfig, "secret") {
				log.Println("use environment variable: ", envName, " = ", envValue)
			}
			if configValue.FieldByName(fieldName).Kind() == reflect.Int64 {
				i, _ := strconv.ParseInt(envValue, 10, 64)
				configValue.FieldByName(fieldName).SetInt(i)
			}
			if configValue.FieldByName(fieldName).Kind() == reflect.Float64 {
				f, _ := strconv.ParseFloat(envValue, 64)
				configValue.FieldByName(fieldName).SetFloat(f)
			}
			if configValue.FieldByName(fieldName).Kind() == reflect.String {
				configValue.FieldByName(fieldName).SetString(envValue)
			}
			if configValue.FieldByName(fieldName).Kind() == reflect.Bool {
				b, _ := strconv.ParseBool(envValue)
				configValue.FieldByName(fieldName).SetBool(b)
			}
			if configValue.FieldByName(fieldName).Kind() == reflect.Slice {
				val := []string{}
				for _, element := range strings.Split(envValue, ",") {
					val = append(val, strings.TrimSpace(element))
				}
				configValue.FieldByName(fieldName).Set(reflect.ValueOf(val))
			}
			if configValue.FieldByName(fieldName).Kind() == reflect.Map {
				value := map[string]string{}
				for _, element := range strings.Split(envValue, ",") {
					keyVal := strings.Split(element, ":")
					key := strings.TrimSpace(keyVal[0])
					val := strings.TrimSpace(keyVal[1])
					value[key] = val
				}
				configValue.FieldByName(fieldName).Set(reflect.ValueOf(value))
			}
		}
	}
}
