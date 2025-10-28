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
	"log"
	"os/exec"
	"regexp"
	"slices"

	"strings"

	"github.com/SENERGY-Platform/mgw-wmbus-dc/pkg/model"
	"github.com/SENERGY-Platform/mgw-wmbus-dc/pkg/util"
	"github.com/SENERGY-Platform/models/go/models"
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
	"github.com/SENERGY-Platform/service-commons/pkg/cache"
)

const (
	wmbusDataProtocolSegment = "data"
	wmbusDecryptedService    = "decrypted"
)

var hexRex = regexp.MustCompile(`0x([0-9a-fA-F]+)`)

// Ensure device type exists.
// If the device does not already exist, a new device is created in the device waiting room.
// If the device exists, decoding (and decrypting) is attempted using wmbusmeters. If decoding is successful,
// the decoded message will be published.
func (this *Handler) handleWmbusEvent(user string, token security.JwtToken, event platform_connector_lib.EventMsg, qos int, nimbus models.Device) (err error) {
	// decode nimbus JSON message
	if this.config.Debug {
		log.Printf("handleWmbusEvent, event: %+v", event)
	}
	eventData, ok := event[wmbusDataProtocolSegment]
	if !ok {
		return errors.New("invalid message: missing protocol segment " + wmbusDataProtocolSegment)
	}
	var msg model.EncryptedMessage
	err = json.Unmarshal([]byte(eventData), &msg)
	if err != nil {
		log.Println("wmbus: unable to unmarshal eventData", err)
		return err
	}

	// ensure device type exists
	deviceTypeId, err := util.DeviceTypeId(msg.Manufacturer, msg.Type, msg.Version, this.wmbusDeviceTypeNamespace)
	if err != nil {
		log.Println("wmbus: unable to calculcate device type id", err)

		return err
	}

	decoded, err := decryptAndDecodeTelegram(this.config.WmbusmetersExecutable, nil, msg.Telegram)
	if err != nil && !errors.Is(err, errorEncrypted) {
		log.Println("wmbus: unable to decryptAndDecodeTelegram 1", err)

		return err
	}
	keyRequired := errors.Is(err, errorEncrypted)
	var deviceType models.DeviceType

	if keyRequired {
		deviceType, err = this.ensureWmbusDeviceType(deviceTypeId, msg, nil)
	} else {
		deviceType, err = this.ensureWmbusDeviceType(deviceTypeId, msg, decoded)
	}
	if err != nil {
		log.Println("wmbus: unable to ensureWmbusDeviceType 1", err)
		return err
	}

	// deduplication
	localDeviceId, err := localDeviceId(msg)
	if err != nil {
		return err
	}
	key := "messages." + user + "." + localDeviceId
	oldTelegram, err := cache.Get(this.connector.IotCache.GetCache(), key, cache.NoValidation[string])
	if err != nil && err != cache.ErrNotFound {
		log.Println("WARN: wmbus: unable to get old telegram from cache", err)
	} else if err == nil {
		if oldTelegram == msg.Telegram {
			log.Println("INFO: handleWmbusEvent: Filtered duplicate message of device " + localDeviceId + " from nimbus " + nimbus.Id)
			return nil // msg is duplicate
		}
	}
	err = this.connector.IotCache.GetCache().Set(key, msg.Telegram, 30) //cache for 30 seconds
	if err != nil {
		log.Println("WARN: wmbus: unable to set old telegram in cache", err)
	}

	// ensure device exists
	device, err := this.connector.IotCache.GetDeviceByLocalId(token, localDeviceId)
	if err != nil && !errors.Is(err, security.ErrorNotFound) {
		log.Println("wmbus: unable to get device", err)
		return err
	} else if errors.Is(err, security.ErrorNotFound) {
		err = nil
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
		stub, err = cache.Use(this.connector.IotCache.GetCache(), "deviceIdentWaitingRoom."+stub.LocalId, func() (DeviceStub, error) { return this.waitingRoom.EnsureWaitingRoom(token, stub) }, cache.NoValidation[DeviceStub], 600) //cache for 10 minutes
		if err != nil {
			log.Println("wmbus: unable to cache deviceIdentWaitingRoom", err)
		}
		return err // done                                                                                                                                                                                                                    // done
	}

	if keyRequired {
		keys := []string{}
		keyOkIdx := -1
		for i, attr := range device.Attributes {
			switch attr.Key {
			case "wmbus/key":
				val := attr.Value
				if len(val) > 0 {
					keys = append(keys, attr.Value)
				}
			case "wmbus/key-ok":
				keyOkIdx = i
			}
		}
		keyOk := models.Attribute{Key: "wmbus/key-ok"}
		if keyOkIdx != -1 {
			keyOk = device.Attributes[keyOkIdx]
		}

		keyOkValue := "false"
		for _, key := range keys {
			decoded, err = decryptAndDecodeTelegram(this.config.WmbusmetersExecutable, &key, msg.Telegram)
			if errors.Is(err, errorWrongKey) {
				continue
			} else if err != nil {
				log.Println("wmbus: unable to decryptAndDecodeTelegram 2", err)

				return err
			}
			keyOkValue = "true"
		}
		if keyOk.Value != keyOkValue || keyOkIdx == -1 {
			keyOk.Value = keyOkValue
			device, err = this.updateDeviceDecryptionStatus(device, keyOk, keyOkIdx, token)
			if err != nil {
				log.Println("wmbus: unable to updateDeviceDecryptionStatus 2", err)

				return err
			}
		}
		if keyOkValue == "false" {
			return nil // no error
		}
		// device type can be updated with (potentially) new fields
		deviceType, err = this.ensureWmbusDeviceType(deviceTypeId, msg, decoded)

		if err != nil {
			log.Println("wmbus: unable to ensureWmbusDeviceType 2", err)

			return err
		}
	}

	reEncodedMsg, err := json.Marshal(decoded)
	if err != nil {
		log.Println("wmbus: unable to marshal decoded message", err)
		return err
	}

	wmbusEvent := platform_connector_lib.EventMsg{
		wmbusDataProtocolSegment: string(reEncodedMsg),
		"timestamp_rfc3339nano":  event["timestamp_rfc3339nano"],
	}
	_, err = this.connector.HandleDeviceRefEventWithAuthToken(token, localDeviceId, wmbusDecryptedService, wmbusEvent, platform_connector_lib.Qos(qos))
	if err != nil {
		log.Println("wmbus: unable to HandleDeviceRefEventWithAuthToken", err)
	}
	return err
}

func (this *Handler) ensureWmbusDeviceType(deviceTypeId string, msg model.EncryptedMessage, decoded map[string]any) (deviceType models.DeviceType, err error) {
	adminToken, err := this.connector.Security().Access()
	if err != nil {
		return deviceType, err
	}

	existingDeviceType, err := this.connector.IotCache.GetDeviceType(adminToken, deviceTypeId)
	if err != nil && !errors.Is(err, security.ErrorNotFound) {
		return deviceType, err
	}

	deviceType = util.DeviceType(deviceTypeId, msg.Manufacturer, msg.Type, msg.Version, this.config.WmbusDeviceClassId, this.config.SenergyProtocolId, this.config.SenergyProtoclSegment, decoded)

	if wmbusDeviceTypeNeedsUpdate(existingDeviceType, deviceType) {
		deviceType, err = this.connector.IotCache.UpdateDeviceType(adminToken, deviceType)
		return deviceType, err
	} else {
		return existingDeviceType, nil
	}

}

func (this *Handler) updateDeviceDecryptionStatus(device models.Device, keyOk models.Attribute, keyOkIdx int, token security.JwtToken) (res models.Device, err error) {
	if keyOkIdx == -1 {
		device.Attributes = append(device.Attributes, keyOk)
	} else {
		device.Attributes[keyOkIdx] = keyOk
	}
	return this.connector.IotCache.UpdateDevice(token, device)

}

var jsonRegex = regexp.MustCompile(`(?s)\{.*\}`)
var errorEncrypted = errors.New("encrypted content")
var errorWrongKey = errors.New("decryption failed")

func decryptAndDecodeTelegram(executable string, key *string, telegram string) (map[string]any, error) {
	analyze := "--analyze"
	if key != nil {
		analyze += "=" + *key
	}
	out, err := exec.Command(executable, analyze, telegram).Output()
	if err != nil {
		return nil, err
	}
	if key == nil && strings.Contains(string(out), "encrypted") {
		return nil, errorEncrypted
	} else if key != nil && strings.Contains(string(out), "failed decryption") {
		return nil, errorWrongKey
	}

	p := jsonRegex.FindSubmatch(out)
	if len(p) != 1 {
		return nil, errors.New("unexpcted output: " + string(out))
	}
	m := map[string]any{}
	err = json.Unmarshal(p[0], &m)
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

func wmbusDeviceTypeNeedsUpdate(existing models.DeviceType, current models.DeviceType) bool {
	if len(existing.Services) != len(current.Services) {
		return true
	}
	if len(existing.Services) == 0 {
		return false
	}
	if len(existing.Services[0].Outputs) != len(current.Services[0].Outputs) {
		return true
	}
	if len(existing.Services[0].Outputs) == 0 {
		return false
	}

	existingCV := existing.Services[0].Outputs[0].ContentVariable.SubContentVariables
	currentCV := current.Services[0].Outputs[0].ContentVariable.SubContentVariables
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
