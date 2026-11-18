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
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestUserLimiterManagerSharesPerUser(t *testing.T) {
	manager := NewUserLimiterManager()

	aliceA := manager.GetOrCreate("alice", 1024)
	aliceB := manager.GetOrCreate("alice", 1024)
	require.Same(t, aliceA, aliceB)

	bob := manager.GetOrCreate("bob", 1024)
	require.NotSame(t, aliceA, bob)

	manager.Set("alice", 2048)
	require.Equal(t, rate.Limit(2048), aliceA.Limit())
	require.Equal(t, rate.Limit(1024), bob.Limit())

	manager.Set("alice", 0)
	require.Equal(t, rate.Inf, aliceA.Limit())
}
