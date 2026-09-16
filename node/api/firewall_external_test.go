// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// TestExternalFirewallIptablesLifecycle runs inside an isolated network
// namespace (invoke the test with `unshare -n`). It exercises real kernel
// iptables state without touching the host namespace.
func TestExternalFirewallIptablesLifecycle(t *testing.T) {
	if os.Getenv("WORKMESH_FIREWALL_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_FIREWALL_EXTERNAL_TEST=1 to run isolated iptables acceptance")
	}
	t.Setenv("WORKMESH_ALLOW_FIREWALL_MUTATION", "1")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/hosts/firewall/settings/operate", handleFirewallBackendOperate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules", handleFirewallRuleCreate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/search", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Scope map[string]any `json:"scope"`
		}
		if err := decodeJSON(r, &request); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		inventory, err := firewallInventory(r.Context(), request.Scope)
		if err != nil {
			wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": inventory})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/delete", handleFirewallRuleDelete)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/reset", handleFirewallRuleReset)
	call := func(path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(res, req)
		return res
	}
	initialized := call("/api/v2/hosts/firewall/settings/operate", `{"backend":"iptables","operation":"initialize"}`)
	if initialized.Code != http.StatusOK {
		t.Fatalf("initialize status=%d body=%s", initialized.Code, initialized.Body.String())
	}
	t.Cleanup(func() {
		_ = call("/api/v2/hosts/firewall/rules/reset", `{"provider":"iptables"}`)
		_ = call("/api/v2/hosts/firewall/settings/operate", `{"backend":"iptables","operation":"cleanup"}`)
	})
	created := call("/api/v2/hosts/firewall/rules", `{"items":[{"rule":{"scope":{"provider":"iptables","family":"ipv4","chain":"WORKMESH_BASIC","direction":"input"},"protocol":"tcp","destinationPort":"18080","action":"accept"}}]}`)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"succeeded":1`) {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	search := call("/api/v2/hosts/firewall/rules/search", `{"scope":{"provider":"iptables","family":"ipv4","chain":"WORKMESH_BASIC"}}`)
	if search.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%s", search.Code, search.Body.String())
	}
	var payload struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &payload); err != nil || len(payload.Data.Items) != 1 {
		t.Fatalf("unexpected inventory: %d %v %s", len(payload.Data.Items), err, search.Body.String())
	}
	rule, _ := payload.Data.Items[0]["rule"].(map[string]any)
	uuid, _ := rule["uuid"].(string)
	if uuid == "" {
		t.Fatalf("inventory rule missing uuid: %#v", rule)
	}
	deleted := call("/api/v2/hosts/firewall/rules/delete", `{"uuids":["`+uuid+`"]}`)
	if deleted.Code != http.StatusOK || !strings.Contains(deleted.Body.String(), `"succeeded":1`) {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
}

func TestExternalFirewallNftablesLifecycle(t *testing.T) {
	if os.Getenv("WORKMESH_FIREWALL_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_FIREWALL_EXTERNAL_TEST=1 to run isolated nftables acceptance")
	}
	t.Setenv("WORKMESH_ALLOW_FIREWALL_MUTATION", "1")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/hosts/firewall/settings/operate", handleFirewallBackendOperate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules", handleFirewallRuleCreate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/search", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Scope map[string]any `json:"scope"`
		}
		if err := decodeJSON(r, &request); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		inventory, err := firewallInventory(r.Context(), request.Scope)
		if err != nil {
			wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": inventory})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/delete", handleFirewallRuleDelete)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/reset", handleFirewallRuleReset)
	call := func(path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(res, req)
		return res
	}
	initialized := call("/api/v2/hosts/firewall/settings/operate", `{"backend":"nftables","operation":"initialize"}`)
	if initialized.Code != http.StatusOK {
		t.Fatalf("initialize status=%d body=%s", initialized.Code, initialized.Body.String())
	}
	t.Cleanup(func() {
		_ = call("/api/v2/hosts/firewall/rules/reset", `{"provider":"nftables"}`)
		_ = call("/api/v2/hosts/firewall/settings/operate", `{"backend":"nftables","operation":"cleanup"}`)
	})
	created := call("/api/v2/hosts/firewall/rules", `{"items":[{"rule":{"scope":{"provider":"nftables","family":"inet","table":"workmesh","chain":"input","direction":"input"},"protocol":"tcp","destinationPort":"18081","action":"accept"}}]}`)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"succeeded":1`) {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	search := call("/api/v2/hosts/firewall/rules/search", `{"scope":{"provider":"nftables","family":"inet","table":"workmesh","chain":"input"}}`)
	if search.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%s", search.Code, search.Body.String())
	}
	var payload struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &payload); err != nil || len(payload.Data.Items) != 1 {
		t.Fatalf("unexpected inventory: %d %v %s", len(payload.Data.Items), err, search.Body.String())
	}
	rule, _ := payload.Data.Items[0]["rule"].(map[string]any)
	uuid, _ := rule["uuid"].(string)
	if uuid == "" {
		t.Fatalf("inventory rule missing uuid: %#v", rule)
	}
	deleted := call("/api/v2/hosts/firewall/rules/delete", `{"uuids":["`+uuid+`"]}`)
	if deleted.Code != http.StatusOK || !strings.Contains(deleted.Body.String(), `"succeeded":1`) {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
}

func TestExternalFirewallCrossBackendSync(t *testing.T) {
	if os.Getenv("WORKMESH_FIREWALL_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_FIREWALL_EXTERNAL_TEST=1 to run isolated firewall sync acceptance")
	}
	t.Setenv("WORKMESH_ALLOW_FIREWALL_MUTATION", "1")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/hosts/firewall/settings/operate", handleFirewallBackendOperate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules", handleFirewallRuleCreate)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/search", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Scope map[string]any `json:"scope"`
		}
		if err := decodeJSON(r, &request); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		inventory, err := firewallInventory(r.Context(), request.Scope)
		if err != nil {
			wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": inventory})
	})
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/sync/preview", handleFirewallSyncPreview)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/sync", handleFirewallSync)
	mux.HandleFunc("POST /api/v2/hosts/firewall/rules/reset", handleFirewallRuleReset)
	call := func(path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(res, req)
		return res
	}
	for _, body := range []string{`{"backend":"iptables","operation":"initialize"}`, `{"backend":"nftables","operation":"initialize"}`} {
		if res := call("/api/v2/hosts/firewall/settings/operate", body); res.Code != http.StatusOK {
			t.Fatalf("initialize status=%d body=%s", res.Code, res.Body.String())
		}
	}
	t.Cleanup(func() {
		_ = call("/api/v2/hosts/firewall/rules/reset", `{"provider":"iptables"}`)
		_ = call("/api/v2/hosts/firewall/rules/reset", `{"provider":"nftables"}`)
		_ = call("/api/v2/hosts/firewall/settings/operate", `{"backend":"iptables","operation":"cleanup"}`)
		_ = call("/api/v2/hosts/firewall/settings/operate", `{"backend":"nftables","operation":"cleanup"}`)
	})
	created := call("/api/v2/hosts/firewall/rules", `{"items":[{"rule":{"scope":{"provider":"iptables","family":"ipv4","chain":"WORKMESH_BASIC","direction":"input"},"protocol":"tcp","destinationPort":"18082","action":"accept"}}]}`)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"succeeded":1`) {
		t.Fatalf("source create status=%d body=%s", created.Code, created.Body.String())
	}
	preview := call("/api/v2/hosts/firewall/rules/sync/preview", `{"subsystem":"system","sourceProvider":"iptables","targetProvider":"nftables"}`)
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), `"ready":1`) {
		t.Fatalf("sync preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	synced := call("/api/v2/hosts/firewall/rules/sync", `{"subsystem":"system","sourceProvider":"iptables","targetProvider":"nftables","resetSource":true}`)
	if synced.Code != http.StatusOK || !strings.Contains(synced.Body.String(), `"succeeded":1`) {
		t.Fatalf("sync status=%d body=%s", synced.Code, synced.Body.String())
	}
	source := call("/api/v2/hosts/firewall/rules/search", `{"scope":{"provider":"iptables","family":"ipv4","chain":"WORKMESH_BASIC"}}`)
	if source.Code != http.StatusOK || strings.Contains(source.Body.String(), "18082") {
		t.Fatalf("source rule was not removed: %d %s", source.Code, source.Body.String())
	}
	target := call("/api/v2/hosts/firewall/rules/search", `{"scope":{"provider":"nftables","family":"inet","table":"workmesh","chain":"input"}}`)
	if target.Code != http.StatusOK || !strings.Contains(target.Body.String(), "18082") {
		t.Fatalf("target rule missing: %d %s", target.Code, target.Body.String())
	}
	reversePreview := call("/api/v2/hosts/firewall/rules/sync/preview", `{"subsystem":"system","sourceProvider":"nftables","targetProvider":"iptables"}`)
	if reversePreview.Code != http.StatusOK || !strings.Contains(reversePreview.Body.String(), `"ready":1`) {
		t.Fatalf("reverse sync preview status=%d body=%s", reversePreview.Code, reversePreview.Body.String())
	}
	reversed := call("/api/v2/hosts/firewall/rules/sync", `{"subsystem":"system","sourceProvider":"nftables","targetProvider":"iptables","resetSource":true}`)
	if reversed.Code != http.StatusOK || !strings.Contains(reversed.Body.String(), `"succeeded":1`) {
		t.Fatalf("reverse sync status=%d body=%s", reversed.Code, reversed.Body.String())
	}
	restored := call("/api/v2/hosts/firewall/rules/search", `{"scope":{"provider":"iptables","family":"ipv4","chain":"WORKMESH_BASIC"}}`)
	if restored.Code != http.StatusOK || !strings.Contains(restored.Body.String(), "18082") {
		t.Fatalf("reverse target rule missing: %d %s", restored.Code, restored.Body.String())
	}
}
