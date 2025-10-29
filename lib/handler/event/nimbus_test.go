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
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"testing"

	device_repo "github.com/SENERGY-Platform/device-repository/lib/client"
	"github.com/SENERGY-Platform/mgw-wmbus-dc/pkg/model"
	"github.com/SENERGY-Platform/mgw-wmbus-dc/pkg/util"
	"github.com/SENERGY-Platform/models/go/models"
	platform_connector_lib "github.com/SENERGY-Platform/platform-connector-lib"
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/configuration"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server/mock/auth"
	"github.com/SENERGY-Platform/senergy-platform-connector/test/server/mock/iot"
	"github.com/google/uuid"
)

func TestDecryptAndDecodeTelegram(t *testing.T) {
	key := "0102030405060708090A0B0C0D0E0F11"
	m, err := decryptAndDecodeTelegram("wmbusmeters", &key, "2E44931578563412330333637A2A0020255923C95AAA26D1B2E7493BC2AD013EC4A6F6D3529B520EDFF0EA6DEFC955B29D6D69EBF3EC8A")
	if err != nil {
		if strings.Contains(err.Error(), "executable file not found in $PATH") {
			t.Skip("wmbusmeters not avilable")
		}
		t.Fatal(err)
	}
	log.Printf("%v\n", m)

	m, err = decryptAndDecodeTelegram("wmbusmeters", nil, "32446850411123936980F219A0019F29FA04702FBF02D808DB080000D40D0100000000000000001E348069253E234B472A0000000000000000611B")
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("%v\n", m)

	m, err = decryptAndDecodeTelegram("wmbusmeters", nil, "A944FA120795133002077A02009025D6464C67E51DA564BBF470979ABE832CEE7270F72AE24D3432CCF6B22BB772E8F85ADE5C4506C2F45B7C4BA6031B2A5068438A1DC312481612004C3AA57598BC91E14C68FA043D13B21A92E51660C327A9A7C5E77147BCAD863C0573E41560E1293258F4ECA7E6AFB1E9AB28F36C488EDEA3D3AD2C9A70B40009D44D2AC9D66CAAFB6B4B18C532A72758E2B2390268D103FD0C08000002FD0B0111")
	if !errors.Is(err, errorEncrypted) {
		t.Fatal("no error on encrypted content")
	}

	_, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	log.Printf("%v\n", m)
}

func TestHandleWmbusEvent(t *testing.T) {
	key := ""
	msg := model.EncryptedMessage{
		Telegram:     "23442D2C051623771B168D20EB413C5721EFBAB5600E96E19562497A618E9C945AEB5B3F",
		Manufacturer: "(KAM) Kamstrup Energi (0x2c2d)",
		MeterId:      "77231605",
		Type:         "Cold water meter (0x16) encrypted",
		Version:      "0x1b",
	}

	config, err := configuration.LoadConfig("../../../config.json")
	if err != nil {
		t.Error(err)
		return
	}

	err = auth.Mock(config, context.Background())
	if err != nil {
		t.Error(err)
		return
	}

	err = iot.Mock(config, context.Background(), false)
	if err != nil {
		t.Error(err)
		return
	}

	connector, err := platform_connector_lib.New(platform_connector_lib.Config{
		AuthEndpoint:     config.AuthEndpoint,
		DeviceManagerUrl: config.DeviceManagerUrl,
		DeviceRepoUrl:    config.DeviceRepoUrl,
	})
	if err != nil {
		t.Error(err)
		return
	}

	handler := New(config, connector, NewTestWaitingRoom())

	deviceTypeId, err := util.DeviceTypeId(msg.Manufacturer, msg.Type, msg.Version, uuid.MustParse(config.WmbusDeviceTypeNamespace))
	if err != nil {
		t.Error(err)
		return
	}

	_, err = handler.ensureWmbusDeviceType(deviceTypeId, msg, nil)
	if err != nil {
		t.Error(err)
		return
	}

	localDeviceId, err := localDeviceId(msg)
	if err != nil {
		t.Error(err)
		return
	}

	token, err := connector.Security().Access()
	if err != nil {
		t.Error(err)
		return
	}

	deviceClient := device_repo.NewClient(config.DeviceRepoUrl, nil)

	_, err, _ = deviceClient.SetProtocol(string(token), models.Protocol{
		Id: "urn:infai:ses:protocol:f3a63aeb-187e-4dd9-9ef5-d97a6eb6292b",
	})
	if err != nil {
		t.Error(err)
		return
	}

	_, err = connector.IotCache.CreateDevice(token, models.Device{
		LocalId:      localDeviceId,
		DeviceTypeId: deviceTypeId,
		Attributes: []models.Attribute{{
			Key:   "wmbus/key",
			Value: key,
		}},
	})
	if err != nil {
		t.Error(err)
		return
	}

	bytes, err := json.Marshal(msg)
	if err != nil {
		t.Error(err)
		return
	}

	err = handler.handleWmbusEvent("sepl", token, platform_connector_lib.EventMsg{"data": string(bytes), "timestamp_rfc3339nano": "2025-10-15T11:06:00.269695138Z"}, 2, models.Device{})

	if err != nil && err.Error() != "no matching producer for qos=2 found" {
		t.Error(err)
		return
	}
}
