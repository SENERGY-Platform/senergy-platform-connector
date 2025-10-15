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
	"encoding/json"
	"errors"
	"log"
	"os/exec"
	"reflect"
	"regexp"

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

// Ensure device type exists.
// If the device does not already exist, a new device is created in the device waiting room.
// If the device exists, decoding (and decrypting) is attempted using wmbusmeters. If decoding is successful,
// the decoded message will be published.
func (this *Handler) handleWmbusEvent(user string, token security.JwtToken, event platform_connector_lib.EventMsg, qos int, nimbus models.Device) (err error) {
	// decode nimbus JSON message
	eventData, ok := event[wmbusDataProtocolSegment]
	if !ok {
		return errors.New("invalid message: missing protocol segment " + wmbusDataProtocolSegment)
	}
	var msg model.EncryptedMessage
	err = json.Unmarshal([]byte(eventData), &msg)
	if err != nil {
		return err
	}

	// ensure device type exists
	deviceTypeId, err := util.DeviceTypeId(msg.Manufacturer, msg.Type, msg.Version, this.wmbusDeviceTypeNamespace)
	if err != nil {
		return err
	}

	decoded, err := decryptAndDecodeTelegram(this.config.WmbusmetersExecutable, nil, msg.Telegram)
	if err != nil && !errors.Is(err, errorEncrypted) {
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
		return err
	}

	// deduplication
	localDeviceId := strings.ReplaceAll(msg.Manufacturer+"_"+msg.Type+"_"+msg.MeterId+"_"+msg.Version, " ", "_")
	localDeviceId = strings.ReplaceAll(localDeviceId, "(", "")
	localDeviceId = strings.ReplaceAll(localDeviceId, ")", "")
	key := "messages." + user + "." + localDeviceId
	oldMsg, err := cache.Get(this.connector.IotCache.GetCache(), key, cache.NoValidation[[]byte])
	if err != nil && err != cache.ErrNotFound {
		return err
	} else if err == nil {
		var oldTelegram string
		err = json.Unmarshal(oldMsg, &oldTelegram)
		if err != nil {
			return err
		}
		if reflect.DeepEqual(oldTelegram, msg.Telegram) {
			log.Println("INFO: handleWmbusEvent: Filtered duplicate message of device " + localDeviceId + " from nimbus " + nimbus.Id)
			return nil // msg is duplicate
		}
	}
	this.connector.IotCache.GetCache().Set(key, msg.Telegram, 30) //cache for 30 seconds

	// ensure device exists
	device, err := this.connector.IotCache.GetDeviceByLocalId(token, localDeviceId)
	if err != nil && !errors.Is(err, security.ErrorNotFound) {
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
		return err                                                                                                                                                                                                                    // done                                                                                                                                                                                                                    // done
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
				return err
			}
			keyOkValue = "true"
		}
		if keyOk.Value != keyOkValue {
			keyOk.Value = keyOkValue
			device, err = this.updateDeviceDecryptionStatus(device, keyOk, keyOkIdx, token)
			if err != nil {
				return err
			}
		}
		if keyOkValue == "false" {
			return nil // no error
		}
		// device type can be updated with (potentially) new fields
		deviceType, err = this.ensureWmbusDeviceType(deviceTypeId, msg, decoded)
		if err != nil {
			return err
		}
	}

	reEncodedMsg, err := json.Marshal(decoded)
	if err != nil {
		return err
	}

	event[wmbusDataProtocolSegment] = string(reEncodedMsg)
	_, err = this.connector.HandleDeviceRefEventWithAuthToken(token, localDeviceId, wmbusDecryptedService, event, platform_connector_lib.Qos(qos))
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

	deviceType = util.DeviceType(deviceTypeId, msg.Manufacturer, msg.Type, msg.Version, this.config.WmbusDeviceClassId, this.config.SenergyProtocolId, decoded)

	if len(existingDeviceType.Services) == 0 || len(existingDeviceType.Services[0].Outputs) == 0 || len(existingDeviceType.Services[0].Outputs[0].ContentVariable.SubContentVariables) < len(deviceType.Services[0].Outputs[0].ContentVariable.SubContentVariables) {
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
