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
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/SENERGY-Platform/platform-connector-lib/model"
	"github.com/SENERGY-Platform/platform-connector-lib/security"
)

type DeviceStub struct {
	model.Device
	UserId     *string    `json:"user_id,omitempty"`
	Hidden     bool       `json:"hidden"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	LastUpdate *time.Time `json:"last_update,omitempty"`
}

type WaitingRoom struct {
	waitingRoomUrl string
}

type WaitingRoomIf interface {
	EnsureWaitingRoom(token security.JwtToken, stub DeviceStub) (res DeviceStub, err error)
}

func NewWaitingRoom(waitingRoomUrl string) WaitingRoomIf {
	return &WaitingRoom{waitingRoomUrl: waitingRoomUrl}
}

func (this *WaitingRoom) EnsureWaitingRoom(token security.JwtToken, stub DeviceStub) (res DeviceStub, err error) {
	url := this.waitingRoomUrl + "/devices/" + stub.LocalId
	status, err := token.Head(url)
	if err != nil {
		return stub, err
	}
	if status == http.StatusOK {
		return
	}
	err = token.PutJSON(url, stub, &res)
	return res, err
}

type TestWaitingRoom struct{}

func NewTestWaitingRoom() WaitingRoomIf {
	return &TestWaitingRoom{}
}

func (this *TestWaitingRoom) EnsureWaitingRoom(token security.JwtToken, stub DeviceStub) (res DeviceStub, err error) {
	slog.Default().Debug(fmt.Sprintf("TestDeviceWaitingRoom ensured device %+v\n", stub))
	return stub, nil
}
