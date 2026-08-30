// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDashboardOS(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/dashboard/base/os", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	var envelope struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 200 || envelope.Data["platform"] == nil {
		t.Fatalf("unexpected dashboard envelope: %#v", envelope)
	}
}

func TestDashboardCurrentPathParameters(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/dashboard/current/none/none", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
}

func TestDashboardNetworkAndDisks(t *testing.T) {
	network := dashboardNetwork()
	if _, ok := network["bytesSent"]; !ok {
		t.Fatal("network bytesSent field missing")
	}
	if _, ok := network["bytesRecv"]; !ok {
		t.Fatal("network bytesRecv field missing")
	}
	if _, ok := network["supported"]; !ok {
		t.Fatal("network supported field missing")
	}
	disks := dashboardDisks()
	if disks == nil {
		t.Fatal("disk data must be initialized")
	}
	for _, disk := range disks {
		if disk["mount"] == nil || disk["device"] == nil {
			t.Fatalf("disk entry missing identity: %#v", disk)
		}
	}
}

func TestDashboardAcceleratorsExplainUnavailable(t *testing.T) {
	for _, kind := range []string{"gpu", "npu", "xpu"} {
		items := dashboardAccelerators(kind)
		if len(items) != 1 || items[0]["available"] != false || items[0]["reason"] == "" {
			t.Fatalf("unexpected %s capability: %#v", kind, items)
		}
	}
}
