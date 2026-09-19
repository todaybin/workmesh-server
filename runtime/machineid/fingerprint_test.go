// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package machineid

import (
	"errors"
	"testing"
)

func TestFromComponentsStableAndSensitiveToHardware(t *testing.T) {
	base := Components{ProductUUID: " UUID-A ", BoardSerial: "BOARD-A", PhysicalMACs: []string{"AA-BB-CC-DD-EE-02", "aa:bb:cc:dd:ee:01"}, CPUIdentity: "Vendor | Model"}
	first, err := FromComponents(base)
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := FromComponents(Components{ProductUUID: "uuid-a", BoardSerial: "board-a", PhysicalMACs: []string{"aa:bb:cc:dd:ee:01", "aa:bb:cc:dd:ee:02"}, CPUIdentity: "vendor | model"})
	if err != nil {
		t.Fatal(err)
	}
	if first != reordered {
		t.Fatalf("规范化后指纹不稳定: %+v != %+v", first, reordered)
	}
	changed := base
	changed.PhysicalMACs = []string{"aa:bb:cc:dd:ee:03"}
	second, err := FromComponents(changed)
	if err != nil {
		t.Fatal(err)
	}
	if first.Code == second.Code {
		t.Fatal("物理网卡变化后必须生成新机器码")
	}
}

func TestFromComponentsRequiresTwoIndependentSignals(t *testing.T) {
	_, err := FromComponents(Components{ProductUUID: "uuid-only", CPUIdentity: "generic-x86"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err=%v, want ErrUnavailable", err)
	}
}
