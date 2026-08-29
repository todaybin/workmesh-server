// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLinkFencingSharesRoleManagerWithCoreRoutes(t *testing.T) {
	t.Setenv("WORKMESH_LINK_SECRET", "")
	mux := http.NewServeMux()
	Register(mux, "node-test", "primary")

	transition := `{"operationId":"switch-1","nodeId":"node-test","from":"primary","to":"secondary","expectedEpoch":1}`
	prepare := httptest.NewRecorder()
	mux.ServeHTTP(prepare, httptest.NewRequest(http.MethodPost, "/api/v2/link/fencing/prepare", strings.NewReader(transition)))
	if prepare.Code != http.StatusOK {
		t.Fatalf("链路 prepare status=%d body=%s", prepare.Code, prepare.Body.String())
	}
	commit := httptest.NewRecorder()
	mux.ServeHTTP(commit, httptest.NewRequest(http.MethodPost, "/api/v2/link/fencing/commit", strings.NewReader(transition)))
	if commit.Code != http.StatusOK {
		t.Fatalf("链路 commit status=%d body=%s", commit.Code, commit.Body.String())
	}

	roleResponse := httptest.NewRecorder()
	mux.ServeHTTP(roleResponse, httptest.NewRequest(http.MethodGet, "/api/v2/core/nodes/role", nil))
	if roleResponse.Code != http.StatusOK {
		t.Fatalf("角色查询 status=%d body=%s", roleResponse.Code, roleResponse.Body.String())
	}
	var envelope struct {
		Data struct {
			Role      string `json:"role"`
			RoleEpoch uint64 `json:"role_epoch"`
		} `json:"data"`
	}
	if err := json.Unmarshal(roleResponse.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Role != "secondary" || envelope.Data.RoleEpoch != 2 {
		t.Fatalf("角色管理器未共享: %+v", envelope.Data)
	}
}
