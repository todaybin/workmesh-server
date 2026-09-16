// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostOperationalRoutesExposeLocalState(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", filepath.Join(".tmp", "host-operational-test"))
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	for _, path := range []string{"/api/v2/hosts/monitor/netoptions", "/api/v2/hosts/monitor/iooptions", "/api/v2/hosts/monitor/setting", "/api/v2/hosts/disks", "/api/v2/hosts/firewall/settings"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"code":200`) {
			t.Fatalf("%s returned %d: %s", path, res.Code, res.Body.String())
		}
	}
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/hosts/disks", nil))
	var envelope struct {
		Data struct {
			Disks      []map[string]any `json:"disks"`
			TotalDisks int              `json:"totalDisks"`
			TotalBytes uint64           `json:"totalCapacity"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid disk response: %v", err)
	}
	if envelope.Data.Disks == nil || envelope.Data.TotalDisks != len(envelope.Data.Disks) {
		t.Fatalf("disk response does not match CompleteDiskInfo contract: %s", res.Body.String())
	}
	if envelope.Data.TotalBytes == 0 && envelope.Data.TotalDisks > 0 {
		t.Fatalf("disk capacity was not collected: %s", res.Body.String())
	}
}

func TestHostMonitorSettingsPersistAndValidate(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", filepath.Join(".tmp", "host-operational-persist"))
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	body := bytes.NewBufferString(`{"enabled":false,"interval":30}`)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/monitor/setting/update", body))
	if res.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", res.Code, res.Body.String())
	}
	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v2/hosts/monitor/setting", nil))
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &envelope); err != nil || envelope.Data["enabled"] != false {
		t.Fatalf("settings were not persisted: %s", get.Body.String())
	}
	bad := httptest.NewRecorder()
	mux.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/monitor/setting/update", bytes.NewBufferString(`{"interval":0}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid interval status=%d body=%s", bad.Code, bad.Body.String())
	}
}

func TestFirewallSettingsExposeBackendGroups(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/hosts/firewall/settings", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("firewall settings status=%d body=%s", res.Code, res.Body.String())
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"system", "forwarding", "docker"} {
		if _, ok := envelope.Data[key]; !ok {
			t.Fatalf("missing firewall backend group %q: %s", key, res.Body.String())
		}
	}
}

func TestFirewallRuleCheckRejectsUnsafeRule(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/firewall/rules/check", bytes.NewBufferString(`{"items":[{"rule":{"scope":{"provider":"iptables","family":"ipv4","chain":"INPUT"},"protocol":"tcp","destinationPort":"22","action":"accept"}}]}`)))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"decision":"blocked"`) {
		t.Fatalf("unsafe firewall rule should be blocked: status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestContainerImageLoadRejectsUnsafeArchivePath(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/containers/image/load", bytes.NewBufferString(`{"path":"/tmp/../image.tar"}`)))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("unsafe path status=%d body=%s", res.Code, res.Body.String())
	}
}
