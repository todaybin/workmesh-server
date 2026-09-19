// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package resources

import "testing"

func TestCollectCopiesPositiveActiveLeases(t *testing.T) {
	leases := map[string]int{"tasks": 2, "idle": 0}
	snapshot := Collect(leases)
	leases["tasks"] = 9
	if snapshot.ActiveLeases["tasks"] != 2 || snapshot.ActiveLeases["idle"] != 0 {
		t.Fatalf("活动租约快照不正确: %#v", snapshot.ActiveLeases)
	}
	if snapshot.Goroutines < 1 {
		t.Fatal("goroutine 计数必须可用")
	}
}
