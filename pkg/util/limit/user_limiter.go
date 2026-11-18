// Copyright 2026 The frp Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package limit

import (
	"sync"

	"golang.org/x/time/rate"
)

// UserLimiterManager keeps one bandwidth limiter per user. All server-side
// limited proxies belonging to the same user share that limiter, so a user's
// limit is aggregate across every tunnel instead of per proxy.
type UserLimiterManager struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func NewUserLimiterManager() *UserLimiterManager {
	return &UserLimiterManager{limiters: make(map[string]*rate.Limiter)}
}

// GetOrCreate returns the shared limiter for user, creating it if needed and
// updating its rate to bytesPerSecond.
func (m *UserLimiterManager) GetOrCreate(user string, bytesPerSecond int64) *rate.Limiter {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limiter, ok := m.limiters[user]; ok {
		applyUserLimit(limiter, bytesPerSecond)
		return limiter
	}
	limiter := newUserLimiter(bytesPerSecond)
	m.limiters[user] = limiter
	return limiter
}

// Set updates an existing user's shared limiter. It creates the limiter when
// it does not exist yet, so a later proxy creation picks up the same value.
func (m *UserLimiterManager) Set(user string, bytesPerSecond int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limiter, ok := m.limiters[user]; ok {
		applyUserLimit(limiter, bytesPerSecond)
		return
	}
	m.limiters[user] = newUserLimiter(bytesPerSecond)
}

func newUserLimiter(bytesPerSecond int64) *rate.Limiter {
	maxBurst := int(^uint(0) >> 1)
	if bytesPerSecond <= 0 {
		return rate.NewLimiter(rate.Inf, maxBurst)
	}
	burst := bytesPerSecond
	if burst > int64(maxBurst) {
		burst = int64(maxBurst)
	}
	return rate.NewLimiter(rate.Limit(float64(bytesPerSecond)), int(burst))
}

func applyUserLimit(limiter *rate.Limiter, bytesPerSecond int64) {
	maxBurst := int(^uint(0) >> 1)
	if bytesPerSecond <= 0 {
		limiter.SetLimit(rate.Inf)
		limiter.SetBurst(maxBurst)
		return
	}
	burst := bytesPerSecond
	if burst > int64(maxBurst) {
		burst = int64(maxBurst)
	}
	limiter.SetLimit(rate.Limit(float64(bytesPerSecond)))
	limiter.SetBurst(int(burst))
}
