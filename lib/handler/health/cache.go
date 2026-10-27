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
	"sync"
	"time"
)

type hubCheckKey struct {
	user  string
	hubId string
}

// hubCheckCache remembers positive hub access checks until their expiry; it never holds more than maxEntries.
type hubCheckCache struct {
	mux        sync.Mutex
	duration   time.Duration
	maxEntries int
	expiry     map[hubCheckKey]time.Time
	nextPrune  time.Time
}

func newHubCheckCache(duration time.Duration, maxEntries int) *hubCheckCache {
	return &hubCheckCache{
		duration:   duration,
		maxEntries: maxEntries,
		expiry:     map[hubCheckKey]time.Time{},
	}
}

func (this *hubCheckCache) has(key hubCheckKey, now time.Time) bool {
	if this.duration <= 0 {
		return false
	}
	this.mux.Lock()
	defer this.mux.Unlock()
	expiry, ok := this.expiry[key]
	return ok && now.Before(expiry)
}

func (this *hubCheckCache) add(key hubCheckKey, now time.Time) {
	if this.duration <= 0 {
		return
	}
	this.mux.Lock()
	defer this.mux.Unlock()
	// pruning at most once per duration keeps add amortized O(1)
	if !now.Before(this.nextPrune) {
		for k, expiry := range this.expiry {
			if !now.Before(expiry) {
				delete(this.expiry, k)
			}
		}
		this.nextPrune = now.Add(this.duration)
	}
	if _, exists := this.expiry[key]; !exists && len(this.expiry) >= this.maxEntries {
		clear(this.expiry)
	}
	this.expiry[key] = now.Add(this.duration)
}
