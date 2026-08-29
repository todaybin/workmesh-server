// SPDX-License-Identifier: LicenseRef-WorkMesh-Pending
// Copyright (c) 2026 WorkMesh contributors

package perf

import (
	"context"
	"testing"

	"github.com/todaybin/workmesh-server/runtime/role"
)

func BenchmarkRoleStateRead(b *testing.B) {
	manager, err := role.New("bench-node", role.Primary)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.State(ctx)
	}
}

func BenchmarkRoleSwitchRoundTrip(b *testing.B) {
	manager, err := role.New("bench-node", role.Primary)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state := manager.State(ctx)
		target := role.Secondary
		if state.Role == role.Secondary {
			target = role.Primary
		}
		if _, err := manager.Switch(ctx, state.RoleEpoch, target); err != nil {
			b.Fatal(err)
		}
	}
}
