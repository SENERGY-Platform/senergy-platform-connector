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

package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/marshalling"
	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/msgvalidation"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/platform-connector-lib/statistics"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"github.com/SENERGY-Platform/service-commons/pkg/jwt"
	"github.com/google/uuid"
)

func New(config configuration.Config, connector *platform_connector_lib.Connector, waitingRoom WaitingRoomIf) *Handler {
	return &Handler{
		connector:                connector,
		config:                   config,
		wmbusDeviceTypeNamespace: uuid.MustParse(config.WmbusDeviceTypeNamespace),
		waitingRoom:              waitingRoom,
	}
}

type Handler struct {
	connector                *platform_connector_lib.Connector
	config                   configuration.Config
	wmbusDeviceTypeNamespace uuid.UUID
	waitingRoom              WaitingRoomIf
}

// Subscribe accepts event subscriptions of the form event/<ownerId>/<localDeviceId>/<serviceLocalId>.
// The subscriber must be the owner of the device or have read access to it according to the device-repository.
//
// A rejection never carries an error: handler.HandleTopicSubscribe turns every handler error into handler.Error,
// which aborts the whole subscribe webhook response instead of rejecting only this topic.
func (this *Handler) Subscribe(clientId string, user string, topic string) (result handler.Result, err error) {
	if strings.SplitN(topic, "/", 2)[0] != "event" {
		return handler.Unhandled, nil
	}
	prefix, ownerId, localDeviceId, serviceLocalId, err := handler.ParseTopic(topic)
	if err != nil {
		this.config.GetLogger().Debug("reject event subscription: unexpected topic structure", "topic", topic, "error", err)
		return handler.Rejected, nil
	}
	if prefix != "event" {
		return handler.Unhandled, nil
	}
	parts := subscribeTopicParts{OwnerId: ownerId, LocalDeviceId: localDeviceId, ServiceLocalId: serviceLocalId}
	if reason := parts.invalid(); reason != "" {
		this.config.GetLogger().Debug("reject event subscription", "topic", topic, "reason", reason)
		return handler.Rejected, nil
	}

	token, err := this.connector.Security().GetCachedUserToken(user, model.RemoteInfo{})
	if err != nil {
		this.config.GetLogger().Warn("reject event subscription: unable to get user token", "topic", topic, "error", err)
		return handler.Rejected, nil
	}
	parsedToken, err := jwt.Parse(string(token))
	if err != nil {
		this.config.GetLogger().Warn("reject event subscription: unable to parse user token", "topic", topic, "error", err)
		return handler.Rejected, nil
	}
	return decideEventSubscribe(this.config.GetLogger(), parsedToken.GetUserId(), parts, func(ownerId string, localDeviceId string) (bool, error) {
		return canReadDevice(this.config.DeviceRepoUrl, token, ownerId, localDeviceId)
	}), nil
}

type subscribeTopicParts struct {
	OwnerId        string
	LocalDeviceId  string
	ServiceLocalId string
}

// invalid returns the reason why a subscription to the topic parts must not be considered, or "" if they are fine.
// Wildcards are only allowed as the whole service level ("+"); owner and device must be concrete.
func (this subscribeTopicParts) invalid() string {
	if this.OwnerId == "" || this.LocalDeviceId == "" || this.ServiceLocalId == "" {
		return "empty topic level"
	}
	for _, level := range []string{this.OwnerId, this.LocalDeviceId, this.ServiceLocalId} {
		if level == "." || level == ".." {
			return "dot path level"
		}
	}
	if strings.ContainsAny(this.OwnerId, "+#") {
		return "wildcard in owner id"
	}
	if strings.ContainsAny(this.LocalDeviceId, "+#") {
		return "wildcard in device id"
	}
	if strings.Contains(this.ServiceLocalId, "#") {
		return "multi level wildcard in service id"
	}
	if strings.Contains(this.ServiceLocalId, "+") && this.ServiceLocalId != "+" {
		return "wildcard must be the whole service level"
	}
	return ""
}

// decideEventSubscribe contains the decision of Subscribe without broker or token handling.
// checkAccess is only called if the subscriber is not the owner.
func decideEventSubscribe(logger *slog.Logger, subscriberId string, parts subscribeTopicParts, checkAccess func(ownerId string, localDeviceId string) (bool, error)) handler.Result {
	if reason := parts.invalid(); reason != "" {
		logger.Debug("reject event subscription", "reason", reason)
		return handler.Rejected
	}
	if subscriberId == "" {
		logger.Warn("reject event subscription: token has no user id")
		return handler.Rejected
	}
	if subscriberId == parts.OwnerId {
		return handler.Accepted
	}
	allowed, err := checkAccess(parts.OwnerId, parts.LocalDeviceId)
	if err != nil {
		logger.Warn("reject event subscription: unable to check device access", "ownerId", parts.OwnerId, "localDeviceId", parts.LocalDeviceId, "error", err)
		return handler.Rejected
	}
	if !allowed {
		logger.Debug("reject event subscription: no read access", "subscriber", subscriberId, "ownerId", parts.OwnerId, "localDeviceId", parts.LocalDeviceId)
		return handler.Rejected
	}
	return handler.Accepted
}

// readDeviceClient bounds the whole request, so several access checks of one SUBSCRIBE stay within the
// webhook's write timeout, and never follows a redirect: device-repository answers a cleaned path such as
// "/local-devices/." with a redirect to its list route, which would read as access to the device.
var readDeviceClient = &http.Client{
	Timeout: 3 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// canReadDevice asks the device-repository for the device of the owner with the token of the subscriber.
// The repository checks the read permission; 401, 403 and 404 mean no access.
// Only the device asked for counts: a device-repository that ignores owner_id answers with the
// subscriber's own device of that local id.
func canReadDevice(deviceRepoUrl string, token security.JwtToken, ownerId string, localDeviceId string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, deviceRepoUrl+"/local-devices/"+url.PathEscape(localDeviceId)+"?owner_id="+url.QueryEscape(ownerId), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", string(token))
	resp, err := readDeviceClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status %v from device-repository", resp.StatusCode)
	}
	device := struct {
		LocalId string `json:"local_id"`
		OwnerId string `json:"owner_id"`
	}{}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&device)
	if err != nil {
		return false, err
	}
	return device.OwnerId == ownerId && device.LocalId == localDeviceId, nil
}

func (this *Handler) Publish(clientId string, user string, topic string, payload []byte, qos int, size float64) (result handler.Result, err error) {
	if !strings.HasPrefix(topic, "event") {
		return handler.Unhandled, nil
	}
	prefix, owner, deviceUri, serviceUri, err := handler.ParseTopic(topic)
	if err != nil {
		return handler.Rejected, err
	}
	if this.config.ForceTopicsWithOwner && owner == "" {
		return handler.Rejected, errors.New("expect owner id in topic")
	}
	if prefix != "event" {
		//may happen if topic is something like "eventhandling/foo/bar"
		this.config.GetLogger().Warn("handler.ParseTopic() returned '"+prefix+"' while the topic string prefix is event", "topic", topic)
		return handler.Unhandled, nil
	}

	token, err := this.connector.Security().GetCachedUserToken(user, model.RemoteInfo{})
	if err != nil {
		return handler.Error, err
	}

	if owner != "" {
		parsedToken, err := jwt.Parse(string(token))
		if err != nil {
			return handler.Error, err
		}
		if !parsedToken.IsAdmin() && parsedToken.GetUserId() != owner {
			return handler.Rejected, errors.New("mismatch between client user and owner in topic")
		}
	}
	if this.config.CheckHub {
		err := handler.CheckHub(this.connector, token, clientId, deviceUri)
		if err != nil {
			return handler.Rejected, err
		}
	}
	event := platform_connector_lib.EventMsg{}
	err = json.Unmarshal(payload, &event)
	if err != nil {
		this.config.GetLogger().Debug("error parsing event payload", "error", err, "payload", string(payload))
		return handler.Rejected, err
	}
	if !this.config.CheckHub {
		if err := handler.CheckEvent(this.connector, token, deviceUri, serviceUri); err != nil {
			this.config.GetLogger().Debug("error checking event", "error", err, "deviceLoacalId", deviceUri, "serviceLocalId", serviceUri)

			if errors.Is(err, handler.ServiceNotFound) {
				this.config.GetLogger().Debug("ignoring event for unknown service", "deviceLocalId", deviceUri, "serviceLocalId", serviceUri)
				return handler.Accepted, nil
			}
			if errors.Is(err, security.ErrorNotFound) || errors.Is(err, security.ErrorAccessDenied) {
				return handler.Rejected, err
			}
			return handler.Error, err
		}
	}
	if !this.config.MqttPublishAuthOnly {
		var info platform_connector_lib.HandledDeviceInfo

		device, err := this.connector.IotCache.WithToken(token).GetDeviceByLocalId(deviceUri)
		if err != nil {
			this.config.GetLogger().Error("cant handle device event: GetDeviceByLocalId", "error", err, "deviceLocalId", deviceUri)
			return this.handleErr(err, size, user, info)
		}
		if device.DeviceTypeId == this.config.NimbusDeviceTypeId {
			err, printErr := this.handleWmbusEvent(user, token, event, qos, device)
			if err != nil && printErr {
				this.config.GetLogger().Error("cant handle wmbus device event", "error", err, "deviceLocalId", deviceUri)
				// no return, nimbus event should still be processed
			}
		}

		info, err = this.connector.HandleDeviceRefEventWithAuthToken(token, deviceUri, serviceUri, event, platform_connector_lib.Qos(qos))
		if info.DeviceId != "" && info.DeviceTypeId != "" {
			statistics.DeviceMsgReceive(size, user, info.DeviceId, info.DeviceTypeId, info.ServiceIds)
		}
		if err != nil {
			return this.handleErr(err, size, user, info)
		}
		statistics.DeviceMsgHandled(size, user, info.DeviceId, info.DeviceTypeId, info.ServiceIds)
	}
	return handler.Accepted, nil
}

func (this *Handler) handleErr(err error, size float64, user string, info platform_connector_lib.HandledDeviceInfo) (handler.Result, error) {
	this.config.GetLogger().Debug("cant handle device event", "error", err)
	if errors.Is(err, security.ErrorNotFound) ||
		errors.Is(err, platform_connector_lib.ErrorUnknownLocalServiceId) {
		return handler.Rejected, err
	} else if !this.config.MqttErrorOnEventValidationError &&
		(marshalling.IsMarshallingErr(err) ||
			errors.Is(err, msgvalidation.ErrUnexpectedField) ||
			errors.Is(err, msgvalidation.ErrMissingField) ||
			errors.Is(err, msgvalidation.ErrUnexpectedType)) {
		statistics.DeviceMsgHandled(size, user, info.DeviceId, info.DeviceTypeId, info.ServiceIds)
		return handler.Accepted, nil
	} else {
		return handler.Error, err
	}
}
