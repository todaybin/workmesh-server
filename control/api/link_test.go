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

func TestNodeListReturnsCurrentNode(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoleRoutesWithManager(mux, NewRoleManager("test-node", "secondary"))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/core/nodes/list", strings.NewReader(`{"type":"all"}`))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", res.Code)
	}
	var body struct {
		Code int `json:"code"`
		Data []struct {
			NodeID string `json:"nodeId"`
			Role   string `json:"role"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 200 || len(body.Data) != 1 || body.Data[0].NodeID != "test-node" || body.Data[0].Role != "secondary" {
		t.Fatalf("unexpected node list: %+v", body)
	}
}
