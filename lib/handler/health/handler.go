/*
 * Copyright 2026 InfAI (CC SES)
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

package health

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/kafka"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
)

// TimeFormat is RFC3339Nano with a fixed-width fraction, so that the string order matches the time order.
const TimeFormat = "2006-01-02T15:04:05.000000000Z07:00"

const maxHubCheckCacheEntries = 100000

var ErrReportTooLarge = errors.New("health report too large")
var ErrInvalidReport = errors.New("health report is not a json object")
var ErrHubAccessDenied = errors.New("client id is not a hub accessible by the user")
var ErrMissingUserId = errors.New("user token without user id")

type ProducerProvider interface {
	GetProducer(qos platform_connector_lib.Qos) (producer kafka.ProducerInterface, err error)
}

type TokenProvider interface {
	GetCachedUserToken(username string, remoteInfo model.RemoteInfo) (token security.JwtToken, err error)
}

type HubChecker interface {
	ExistsHub(id string, cred security.JwtToken) (exists bool, err error)
}

// Message is the Kafka message on GatewayHealthTopic, keyed by HubId.
type Message struct {
	HubId  string          `json:"hub_id"`
	UserId string          `json:"user_id"`
	Time   string          `json:"time"`
	Report json.RawMessage `json:"report,omitempty"`
}

type Config struct {
	Topic                 string
	KafkaTopic            string
	MaxPayloadSize        int64
	HubCheckCacheDuration time.Duration // <= 0 disables the cache
}

type Handler struct {
	config           Config
	producerProvider ProducerProvider
	tokens           TokenProvider
	hubs             HubChecker
	cache            *hubCheckCache
	now              func() time.Time
	logger           *slog.Logger
}

func Enabled(topic string) bool {
	return topic != "" && topic != "-"
}

func NewFromConfig(config configuration.Config, producerProvider ProducerProvider, tokens TokenProvider, hubs HubChecker) (*Handler, error) {
	cacheDuration, err := time.ParseDuration(config.HealthHubCheckCacheDuration)
	if err != nil {
		return nil, fmt.Errorf("invalid HealthHubCheckCacheDuration: %w", err)
	}
	return New(Config{
		Topic:                 config.HealthTopic,
		KafkaTopic:            config.GatewayHealthTopic,
		MaxPayloadSize:        config.HealthMaxPayloadSize,
		HubCheckCacheDuration: cacheDuration,
	}, producerProvider, tokens, hubs, config.GetLogger())
}

func New(config Config, producerProvider ProducerProvider, tokens TokenProvider, hubs HubChecker, logger *slog.Logger) (*Handler, error) {
	if !Enabled(config.Topic) {
		return nil, errors.New("missing health mqtt topic")
	}
	if !Enabled(config.KafkaTopic) {
		return nil, errors.New("missing gateway health kafka topic")
	}
	if config.MaxPayloadSize <= 0 {
		return nil, fmt.Errorf("invalid health max payload size %v", config.MaxPayloadSize)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		config:           config,
		producerProvider: producerProvider,
		tokens:           tokens,
		hubs:             hubs,
		cache:            newHubCheckCache(config.HubCheckCacheDuration, maxHubCheckCacheEntries),
		now:              time.Now,
		logger:           logger,
	}, nil
}

// Subscribe rejects without error; HandleTopicSubscribe would turn a non-nil error into handler.Error.
func (this *Handler) Subscribe(clientId string, user string, topic string) (result handler.Result, err error) {
	if topic != this.config.Topic {
		return handler.Unhandled, nil
	}
	return handler.Rejected, nil
}

// Publish returns a non-nil error with every Rejected or Error result; the vernemqtt webhook reads err.Error() for both.
func (this *Handler) Publish(clientId string, user string, topic string, payload []byte, qos int, size float64) (result handler.Result, err error) {
	if topic != this.config.Topic {
		return handler.Unhandled, nil
	}
	if int64(len(payload)) > this.config.MaxPayloadSize {
		return handler.Rejected, fmt.Errorf("%w: %v bytes, at most %v allowed", ErrReportTooLarge, len(payload), this.config.MaxPayloadSize)
	}
	if !isJsonObject(payload) {
		return handler.Rejected, ErrInvalidReport
	}
	token, err := this.tokens.GetCachedUserToken(user, model.RemoteInfo{})
	if err != nil {
		return handler.Error, err
	}
	accessible, err := this.isAccessibleHub(user, clientId, token)
	if err != nil {
		return handler.Error, err
	}
	if !accessible {
		this.logger.Info("reject health report for inaccessible hub", "clientId", clientId, "user", user)
		return handler.Rejected, ErrHubAccessDenied
	}
	jwtPayload, err := token.GetPayload()
	if err != nil {
		return handler.Error, err
	}
	if jwtPayload.UserId == "" {
		return handler.Error, ErrMissingUserId
	}
	msg, err := encodeMessage(clientId, jwtPayload.UserId, this.now(), payload)
	if err != nil {
		return handler.Error, err
	}
	producer, err := this.producerProvider.GetProducer(platform_connector_lib.Sync)
	if err != nil {
		return handler.Error, err
	}
	err = producer.ProduceWithKey(this.config.KafkaTopic, string(msg), clientId)
	if err != nil {
		return handler.Error, err
	}
	return handler.Accepted, nil
}

// isAccessibleHub caches only positive results, so that a newly shared or created hub works immediately.
func (this *Handler) isAccessibleHub(user string, hubId string, token security.JwtToken) (bool, error) {
	// An empty or dot-segment id would make the HEAD request address another resource than a single hub.
	if hubId == "" || hubId == "." || hubId == ".." {
		return false, nil
	}
	key := hubCheckKey{user: user, hubId: hubId}
	if this.cache.has(key, this.now()) {
		return true, nil
	}
	// ExistsHub reports exists=true together with a transport error, so the error is checked first.
	exists, err := this.hubs.ExistsHub(hubId, token)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	this.cache.add(key, this.now())
	return true, nil
}

func isJsonObject(payload []byte) bool {
	if !utf8.Valid(payload) || !json.Valid(payload) {
		return false
	}
	trimmed := bytes.TrimLeft(payload, " \t\r\n")
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// encodeMessage splices the report in, because json.Marshal would compact and HTML-escape a json.RawMessage.
// report must be a valid json value.
func encodeMessage(hubId string, userId string, received time.Time, report []byte) ([]byte, error) {
	header, err := json.Marshal(Message{HubId: hubId, UserId: userId, Time: received.UTC().Format(TimeFormat)})
	if err != nil {
		return nil, err
	}
	const reportField = `,"report":`
	result := make([]byte, 0, len(header)+len(reportField)+len(report))
	result = append(result, header[:len(header)-1]...)
	result = append(result, reportField...)
	result = append(result, report...)
	result = append(result, '}')
	return result, nil
}
