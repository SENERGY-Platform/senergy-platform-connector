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

package export

import (
	"github.com/SENERGY-Platform/senergy-platform-connector/lib/handler"
	"testing"
)

func TestHandlerPublish(t *testing.T) {
	_handler := New(Sec{})
	testPublish("something/else", "unknown", _handler, handler.Unhandled)
	testPublish("export/else", "unknown", _handler, handler.Unhandled)
	testPublish("export", "unknown", _handler, handler.Unhandled)
	testPublish("export", "unknown", _handler, handler.Unhandled)
}

func TestHandlerSubscribe(t *testing.T) {
	_handler := New(Sec{})

	t.Run(testSubscribe("something/else", "unknown", _handler, handler.Unhandled))
	t.Run(testSubscribe("export/", "user", _handler, handler.Rejected))
	t.Run(testSubscribe("export/#", "user", _handler, handler.Rejected))
	t.Run(testSubscribe("export/differentUser", "user", _handler, handler.Rejected))
	t.Run(testSubscribe("export/user", "user", _handler, handler.Accepted))
	t.Run(testSubscribe("export/user/#", "user", _handler, handler.Accepted))
	t.Run(testSubscribe("export/user/deep", "user", _handler, handler.Accepted))

}

func testSubscribe(topic string, user string, _handler *Handler, expectedResult handler.Result) (string, func(t *testing.T)) {
	return topic, func(t *testing.T) {
		result, err := _handler.Subscribe("", user, topic)
		if err != nil {
			if expectedResult != handler.Error {
				t.Error(result, err)
				return
			}
			if result != handler.Error {
				t.Error("expect Error result with err", result, err)
				return
			}
		}
		if result != expectedResult {
			t.Error(result, expectedResult)
			return
		}
	}
}

func testPublish(topic string, payload string, _handler *Handler, expectedResult handler.Result) (string, func(t *testing.T)) {
	return topic, func(t *testing.T) {
		result, err := _handler.Publish("", "", topic, []byte(payload))
		if err != nil {
			if expectedResult != handler.Error {
				t.Error(result, err)
				return
			}
			if result != handler.Error {
				t.Error("expect Error result with err", result, err)
				return
			}
		}
		if result != expectedResult {
			t.Error(result, expectedResult)
			return
		}
	}
}

type Sec struct {
}

func (_ Sec) GetUserId(username string) (userid string, err error) {
	return username, nil
}
