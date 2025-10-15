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
	"log"
	"strings"
	"testing"
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
}
