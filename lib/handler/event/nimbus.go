/*
 * Copyright 2025 InfAI (CC SES)
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
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"strings"

	"github.com/SENERGY-Platform/mgw-wmbus-dc/pkg/model"
	"github.com/SENERGY-Platform/mgw-wmbus-dc/pkg/util"
	"github.com/SENERGY-Platform/models/go/models"
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/service-commons/pkg/cache"
)

const (
	wmbusDataProtocolSegment     = "data"
	wmbusDecryptedService        = "decrypted"
	wmbusDriverAttribute         = "wmbus/driver"
	wmbusKeyOkAttribute          = "wmbus/key-ok"
	connectorOrigin              = "senergy-platform-connector"
	defaultAttributeOrigin       = "default" // account defaults, merged into the device on read but not stored with it
	wmbusLastSeenAttributePrefix = "wmbus/last_seen/"
	wmbusLastSeenInterval        = time.Minute
)

var hexRex = regexp.MustCompile(`0x([0-9a-fA-F]+)`)

// Ensure device type exists.
// If the device does not already exist, a new device is created in the device waiting room.
// If the device exists, decoding (and decrypting) is attempted using wmbusmeters. If decoding is successful,
// the decoded message will be published.
func (this *Handler) handleWmbusEvent(user string, token security.JwtToken, event platform_connector_lib.EventMsg, qos int, nimbus models.Device) (err error, shouldPrintErr bool) {
	// decode nimbus JSON message
	this.config.GetLogger().Debug("handleWmbusEvent", "event", event)
	eventData, ok := event[wmbusDataProtocolSegment]
	if !ok {
		return errors.New("invalid message: missing protocol segment " + wmbusDataProtocolSegment), true
	}
	var msg model.EncryptedMessage
	err = json.Unmarshal([]byte(eventData), &msg)
	if err != nil {
		this.config.GetLogger().Error("wmbus: unable to unmarshal eventData", "error", err)
		return err, false
	}

	// ensure device type exists
	deviceTypeId, err := util.DeviceTypeId(msg.Manufacturer, msg.Type, msg.Version, this.wmbusDeviceTypeNamespace)
	if err != nil {
		this.config.GetLogger().Error("wmbus: unable to calculcate device type id", "error", err)
		return err, false
	}

	localDeviceId, err := localDeviceId(msg)
	if err != nil {
		return err, true
	}

	// the forced driver may be set on the device or on the device type, both of which may not exist yet
	device, err := this.connector.IotCache.GetDeviceByLocalId(token, localDeviceId)
	deviceFound := true
	if errors.Is(err, security.ErrorNotFound) {
		device = models.Device{}
		deviceFound = false
	} else if err != nil {
		this.config.GetLogger().Error("wmbus: unable to get device", "error", err)
		return err, false
	}
	// recorded before any filtering, so duplicates and undecodable telegrams count as received
	if deviceFound {
		if err := this.recordWmbusReceiver(token, device, nimbus); err != nil {
			this.config.GetLogger().Warn("wmbus: unable to record receiving nimbus", "error", err, "deviceLocalId", localDeviceId, "nimbus", nimbus.Id)
		}
	}
	existingDeviceType, err := this.getWmbusDeviceType(deviceTypeId)
	if err != nil {
		this.config.GetLogger().Error("wmbus: unable to get device type", "error", err)
		return err, false
	}
	driver := wmbusDriver(device, existingDeviceType)

	decoded, err := decryptAndDecodeTelegram(this.config.WmbusmetersExecutable, this.config.WmbusmetersDriversDir, driver, nil, msg.Telegram)
	if err != nil && !errors.Is(err, errorEncrypted) {
		this.config.GetLogger().Warn("wmbus: unable to decryptAndDecodeTelegram #1", "error", err, "telegram", msg.Telegram)
		return err, false
	}
	keyRequired := errors.Is(err, errorEncrypted)
	var deviceType models.DeviceType

	if keyRequired {
		deviceType, err = this.ensureWmbusDeviceType(deviceTypeId, msg, nil)
	} else {
		deviceType, err = this.ensureWmbusDeviceType(deviceTypeId, msg, decoded)
	}
	if err != nil {
		this.config.GetLogger().Error("wmbus: unable to ensureWmbusDeviceType #1", "error", err)
		return err, false
	}

	// deduplication
	key := "messages." + user + "." + localDeviceId
	oldTelegram, err := cache.Get(this.connector.IotCache.GetCache(), key, cache.NoValidation[string])
	if err != nil && !errors.Is(err, cache.ErrNotFound) {
		this.config.GetLogger().Warn("wmbus: unable to get old telegram from cache", "error", err)
	} else if err == nil {
		if oldTelegram == msg.Telegram {
			this.config.GetLogger().Info("wmbus: duplicate message of device " + localDeviceId + " from nimbus " + nimbus.Id)
			return nil, false // msg is duplicate
		}
	}
	err = this.connector.IotCache.GetCache().Set(key, msg.Telegram, 30*time.Second) //cache for 30 seconds
	if err != nil {
		this.config.GetLogger().Warn("wmbus: unable to set old telegram in cache", "error", err)
	}

	// ensure device exists
	if !deviceFound {
		attr := []models.Attribute{}
		if keyRequired {
			attr = append(attr, models.Attribute{
				Key:   "wmbus/key",
				Value: "",
			})
		}
		stub := DeviceStub{
			Device: models.Device{
				LocalId:      localDeviceId,
				Name:         msg.Manufacturer + " " + msg.Type + " " + msg.MeterId,
				Attributes:   attr,
				DeviceTypeId: deviceType.Id,
			},
		}
		stub, err = cache.Use(this.connector.IotCache.GetCache(), "deviceIdentWaitingRoom."+stub.LocalId, func() (DeviceStub, error) { return this.waitingRoom.EnsureWaitingRoom(token, stub) }, cache.NoValidation[DeviceStub], 60*time.Second) //cache for 1 minutes
		if err != nil {
			this.config.GetLogger().Error("wmbus: unable to cache deviceIdentWaitingRoom", "error", err)
		}
		return err, false // done                                                                                                                                                                                                                    // done
	}

	if keyRequired {
		keys := []string{}
		for _, attr := range device.Attributes {
			if attr.Key == "wmbus/key" && len(attr.Value) > 0 {
				keys = append(keys, attr.Value)
			}
		}

		keyOkValue := "false"
		for _, key := range keys {
			decoded, err = decryptAndDecodeTelegram(this.config.WmbusmetersExecutable, this.config.WmbusmetersDriversDir, driver, &key, msg.Telegram)
			if errors.Is(err, errorWrongKey) {
				continue
			} else if err != nil {
				this.config.GetLogger().Warn("wmbus: unable to decryptAndDecodeTelegram #2", "error", err, "telegram", msg.Telegram)
				return err, false
			}
			keyOkValue = "true"
		}
		err = this.updateDeviceDecryptionStatus(token, device, keyOkValue)
		if err != nil {
			this.config.GetLogger().Error("wmbus: unable to updateDeviceDecryptionStatus #1", "error", err)
			return err, false
		}
		if keyOkValue == "false" {
			return nil, false // no error
		}
		// device type can be updated with (potentially) new fields
		deviceType, err = this.ensureWmbusDeviceType(deviceTypeId, msg, decoded)

		if err != nil {
			this.config.GetLogger().Error("wmbus: unable to ensureWmbusDeviceType #2", "error", err)
			return err, false
		}
	}

	reEncodedMsg, err := json.Marshal(decoded)
	if err != nil {
		this.config.GetLogger().Error("wmbus: unable to marshal decoded message", "error", err)
		return err, false
	}

	wmbusEvent := platform_connector_lib.EventMsg{
		wmbusDataProtocolSegment: string(reEncodedMsg),
		"timestamp_rfc3339nano":  event["timestamp_rfc3339nano"],
	}
	_, err = this.connector.HandleDeviceRefEventWithAuthToken(token, localDeviceId, wmbusDecryptedService, wmbusEvent, platform_connector_lib.Qos(qos))
	if err != nil {
		this.config.GetLogger().Error("wmbus: unable to HandleDeviceRefEventWithAuthToken", "error", err)
	}
	return err, false
}

func (this *Handler) ensureWmbusDeviceType(deviceTypeId string, msg model.EncryptedMessage, decoded map[string]any) (deviceType models.DeviceType, err error) {
	adminToken, err := this.connector.Security().Access()
	if err != nil {
		return deviceType, err
	}

	existingDeviceType, err := this.getWmbusDeviceType(deviceTypeId)
	if err != nil {
		return deviceType, err
	}

	deviceType = util.DeviceType(deviceTypeId, msg.Manufacturer, msg.Type, msg.Version, this.config.WmbusDeviceClassId, this.config.SenergyProtocolId, this.config.SenergyProtoclSegment, decoded)
	deviceType.Attributes = keepUnmanagedAttributes(existingDeviceType.Attributes, deviceType.Attributes)

	if wmbusDeviceTypeNeedsUpdate(existingDeviceType, deviceType) {
		this.config.GetLogger().Info("Updating wmbus device type " + deviceType.Id)
		deviceType, err = this.connector.IotCache.UpdateDeviceType(adminToken, deviceType)
		return deviceType, err
	} else {
		return existingDeviceType, nil
	}

}

// getWmbusDeviceType returns the stored wmbus device type or an empty device type, if it does not exist yet.
func (this *Handler) getWmbusDeviceType(deviceTypeId string) (deviceType models.DeviceType, err error) {
	adminToken, err := this.connector.Security().Access()
	if err != nil {
		return deviceType, err
	}
	deviceType, err = this.connector.IotCache.GetDeviceType(adminToken, deviceTypeId)
	if errors.Is(err, security.ErrorNotFound) {
		return models.DeviceType{}, nil
	}
	if err != nil {
		return models.DeviceType{}, err
	}
	return deviceType, nil
}

// wmbusDriver returns the wmbusmeters driver forced by attribute, or an empty string, if none is forced.
// A driver set on the device takes precedence over a driver set on the device type.
func wmbusDriver(device models.Device, deviceType models.DeviceType) string {
	for _, attr := range device.Attributes {
		if attr.Key == wmbusDriverAttribute && len(attr.Value) > 0 {
			return attr.Value
		}
	}
	for _, attr := range deviceType.Attributes {
		if attr.Key == wmbusDriverAttribute && len(attr.Value) > 0 {
			return attr.Value
		}
	}
	return ""
}

// keepUnmanagedAttributes appends those existing attributes to the generated ones, which the generator does not manage.
// util.DeviceType() builds the attribute list from the telegram only, so attributes set by others,
// like wmbus/driver, would be lost with the next device type update.
func keepUnmanagedAttributes(existing []models.Attribute, generated []models.Attribute) []models.Attribute {
	result := slices.Clone(generated)
	for _, attr := range existing {
		if !slices.ContainsFunc(generated, func(g models.Attribute) bool { return g.Key == attr.Key }) {
			result = append(result, attr)
		}
	}
	return result
}

// recordWmbusReceiver stores the time the nimbus received a telegram of the device, in one attribute per nimbus.
// The attribute has the nimbus id as origin, so the update replaces only this attribute and keeps every other one,
// including those changed since the device was cached. Each nimbus and device pair is written at most once per wmbusLastSeenInterval.
func (this *Handler) recordWmbusReceiver(token security.JwtToken, device models.Device, nimbus models.Device) error {
	if nimbus.Id == "" {
		// an empty origin would replace all attributes without origin, like wmbus/key
		return errors.New("missing nimbus id")
	}
	key := "wmbus.last_seen." + nimbus.Id + "." + device.Id
	_, err := cache.Get(this.connector.IotCache.GetCache(), key, cache.NoValidation[string])
	if err == nil {
		return nil // recorded recently
	}
	if !errors.Is(err, cache.ErrNotFound) {
		return err
	}
	// set before the update, so a failing update is not repeated with every telegram
	err = this.connector.IotCache.GetCache().Set(key, nimbus.Id, wmbusLastSeenInterval)
	if err != nil {
		return err
	}
	return this.setSameOriginAttributes(token, device.Id, []string{nimbus.Id}, []models.Attribute{{
		Key:    wmbusLastSeenAttributePrefix + nimbus.Id,
		Value:  time.Now().UTC().Format(time.RFC3339),
		Origin: nimbus.Id,
	}})
}

// setSameOriginAttributes replaces the attributes of the given origins on the device and keeps all others.
// An empty origin addresses the attributes without origin.
func (this *Handler) setSameOriginAttributes(token security.JwtToken, deviceId string, origins []string, attributes []models.Attribute) error {
	endpoint := this.config.DeviceManagerUrl + "/devices/" + url.PathEscape(deviceId) + "/attributes?update-only-same-origin-attributes=" + url.QueryEscape(strings.Join(origins, ","))
	return token.PutJSON(endpoint, attributes, nil)
}

// updateDeviceDecryptionStatus sets wmbus/key-ok with the connector origin, so the update replaces no other attribute,
// however old the cached device is. A wmbus/key-ok of another origin, as set by earlier versions, is replaced once per device.
func (this *Handler) updateDeviceDecryptionStatus(token security.JwtToken, device models.Device, keyOkValue string) error {
	current := slices.IndexFunc(device.Attributes, func(attr models.Attribute) bool {
		return attr.Key == wmbusKeyOkAttribute && attr.Origin == connectorOrigin
	})
	legacy := slices.ContainsFunc(device.Attributes, isLegacyKeyOk)
	if current != -1 && device.Attributes[current].Value == keyOkValue && !legacy {
		return nil
	}
	// the cached device shows the old state until it expires, which would repeat the write with every telegram
	cacheKey := "wmbus.key-ok." + device.Id
	written, err := cache.Get(this.connector.IotCache.GetCache(), cacheKey, cache.NoValidation[string])
	if err == nil && written == keyOkValue {
		return nil
	}
	keyOk := models.Attribute{Key: wmbusKeyOkAttribute, Value: keyOkValue, Origin: connectorOrigin}
	if legacy {
		err = this.replaceLegacyKeyOk(token, device.Id, keyOk)
	} else {
		err = this.setSameOriginAttributes(token, device.Id, []string{connectorOrigin}, []models.Attribute{keyOk})
	}
	if err != nil {
		return err
	}
	if this.config.DeviceExpiration > 0 {
		err = this.connector.IotCache.GetCache().Set(cacheKey, keyOkValue, time.Duration(this.config.DeviceExpiration)*time.Second)
		if err != nil {
			this.config.GetLogger().Warn("wmbus: unable to cache written decryption status", "error", err)
		}
	}
	return nil
}

// replaceLegacyKeyOk removes wmbus/key-ok of other origins and sets keyOk. The request has to carry all other attributes
// of those origins, for the empty origin among them wmbus/key, so they are read from the device-manager instead of the
// device cache. That leaves the delay of the device-manager in reading its own updates as window for overwriting a change.
func (this *Handler) replaceLegacyKeyOk(token security.JwtToken, deviceId string, keyOk models.Attribute) error {
	device := models.Device{}
	err := token.GetJSON(this.config.DeviceManagerUrl+"/devices/"+url.PathEscape(deviceId), &device)
	if err != nil {
		return err
	}
	origins := []string{keyOk.Origin}
	for _, attr := range device.Attributes {
		if isLegacyKeyOk(attr) && !slices.Contains(origins, attr.Origin) {
			origins = append(origins, attr.Origin)
		}
	}
	attributes := []models.Attribute{keyOk}
	for _, attr := range device.Attributes {
		if attr.Key != keyOk.Key && slices.Contains(origins, attr.Origin) {
			attributes = append(attributes, attr)
		}
	}
	return this.setSameOriginAttributes(token, deviceId, origins, attributes)
}

// isLegacyKeyOk reports a stored wmbus/key-ok not written with the connector origin.
func isLegacyKeyOk(attr models.Attribute) bool {
	return attr.Key == wmbusKeyOkAttribute && attr.Origin != connectorOrigin && attr.Origin != defaultAttributeOrigin
}

// wmbusmeters prints the decoded json as the last brace block of its output. Since version 3.0.0
// the analysis of manufacturer specific data contains braced blocks of its own, which precede
// the json, so the first block is not the json.
var jsonRegex = regexp.MustCompile(`(?ms)^\{$.*?^\}$`)
var errorEncrypted = errors.New("encrypted content")
var errorWrongKey = errors.New("decryption failed")
var errorUnknownDriver = errors.New("unknown wmbusmeters driver")
var errorInvalidDriversDir = errors.New("wmbusmeters drivers dir must be an absolute path")

// An empty driver lets wmbusmeters detect the driver, any other value forces that driver.
// An empty driversDir leaves wmbusmeters with its built in drivers, any other value must be an
// absolute path to a directory containing nothing but loadable driver files. wmbusmeters exits with
// EXIT_DRIVER_ERROR on any other directory entry, and it ignores only ".", ".." and files ending in "~".
func decryptAndDecodeTelegram(executable string, driversDir string, driver string, key *string, telegram string) (map[string]any, error) {
	telegram = strings.ToLower(regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(telegram, "")) // sanitize telegram for command line usage
	analyze := "--analyze"
	sanitizedDriver := regexp.MustCompile(`[^a-zA-Z0-9_]`).ReplaceAllString(driver, "") // sanitize driver for command line usage, must not contain the : separating driver and key
	if len(sanitizedDriver) > 0 || key != nil {
		analyze += "="
	}
	analyze += sanitizedDriver
	if key != nil {
		sanitizedKey := regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(*key, "") // sanitize key for command line usage
		if len(sanitizedDriver) > 0 {
			analyze += ":"
		}
		analyze += sanitizedKey
	}
	args := []string{}
	if len(driversDir) > 0 {
		if !filepath.IsAbs(driversDir) {
			return nil, fmt.Errorf("%w: %s", errorInvalidDriversDir, driversDir)
		}
		args = append(args, "--driversdir="+driversDir) // dynamically loaded drivers take part in the driver detection of --analyze
	}
	args = append(args, analyze, telegram)
	out, err := exec.Command(executable, args...).CombinedOutput()
	if strings.Contains(string(out), "No such driver ") { // wmbusmeters exits with EXIT_DRIVER_ERROR, so check the output before the exit code
		return nil, fmt.Errorf("%w: %s", errorUnknownDriver, sanitizedDriver)
	}
	if err != nil {
		return nil, err
	}
	if key == nil && (strings.Contains(string(out), "encrypted") || strings.Contains(string(out), "failed decryption")) {
		return nil, errorEncrypted
	} else if key != nil && strings.Contains(string(out), "failed decryption") {
		return nil, errorWrongKey
	}

	blocks := jsonRegex.FindAll(out, -1)
	if len(blocks) == 0 {
		return nil, errors.New("unexpcted output: " + string(out))
	}
	m := map[string]any{}
	err = json.Unmarshal(blocks[len(blocks)-1], &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func localDeviceId(msg model.EncryptedMessage) (string, error) {
	localDeviceId := "wmbus"

	p := hexRex.FindStringSubmatch(msg.Manufacturer)
	if len(p) != 2 {
		return "", errors.New("invalid manufacturer data")
	}
	if len(p[1])%2 != 0 {
		p[1] = "0" + p[1]
	}
	localDeviceId += "_" + p[1]

	p = hexRex.FindStringSubmatch(msg.Version)
	if len(p) != 2 {
		return "", errors.New("invalid version data")
	}
	localDeviceId += "_" + p[1]

	p = hexRex.FindStringSubmatch(msg.Type)
	if len(p) != 2 {
		return "", errors.New("invalid type data")
	}
	localDeviceId += "_" + p[1]
	localDeviceId += "_" + msg.MeterId

	return localDeviceId, nil
}

func wmbusDeviceTypeNeedsUpdate(existing models.DeviceType, new models.DeviceType) bool {
	if len(existing.Services) != len(new.Services) {
		return true
	}
	if len(existing.Services) == 0 { // both are len 0
		return false
	}
	if len(existing.Services[0].Outputs) < len(new.Services[0].Outputs) {
		return true
	}
	if len(existing.Services[0].Outputs) > len(new.Services[0].Outputs) {
		return false
	}
	if len(existing.Services[0].Outputs) == 0 {
		return false
	}

	existingCV := existing.Services[0].Outputs[0].ContentVariable.SubContentVariables
	currentCV := new.Services[0].Outputs[0].ContentVariable.SubContentVariables
	if len(existingCV) > len(currentCV) {
		return false
	}
	if len(existingCV) < len(currentCV) {
		return true
	}
	cmpCVName := func(a models.ContentVariable, b models.ContentVariable) int {
		return cmp.Compare(a.Name, b.Name)
	}
	slices.SortFunc(existingCV, cmpCVName)
	slices.SortFunc(currentCV, cmpCVName)
	for i := range len(existingCV) {
		if existingCV[i].Name != currentCV[i].Name {
			return true
		}
		if existingCV[i].Type != currentCV[i].Type {
			return true
		}
	}
	return false
}
