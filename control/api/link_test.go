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

func TestNodeAddAndFavoriteRoundTrip(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	RegisterRoleRoutesWithManager(mux, NewRoleManager("primary", "primary"))
	add := httptest.NewRecorder()
	mux.ServeHTTP(add, httptest.NewRequest(http.MethodPost, "/api/v2/core/nodes/add", strings.NewReader(`{"nodeId":"secondary","addr":"http://127.0.0.1:9999","role":"secondary"}`)))
	if add.Code != http.StatusOK {
		t.Fatalf("add status=%d body=%s", add.Code, add.Body.String())
	}
	favorite := httptest.NewRecorder()
	mux.ServeHTTP(favorite, httptest.NewRequest(http.MethodPost, "/api/v2/core/xpack/nodes/favorite", strings.NewReader(`{"nodeId":"secondary","isFavorite":true}`)))
	if favorite.Code != http.StatusOK {
		t.Fatalf("favorite status=%d body=%s", favorite.Code, favorite.Body.String())
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodPost, "/api/v2/core/nodes/list", strings.NewReader(`{"type":"all"}`)))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"isFavorite":true`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
}

func TestNodeAddUpdateFavoriteDelete(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	RegisterRoleRoutesWithManager(mux, NewRoleManager("local", "primary"))
	add := httptest.NewRecorder()
	mux.ServeHTTP(add, httptest.NewRequest(http.MethodPost, "/api/v2/core/nodes/add", strings.NewReader(`{"nodeId":"secondary-1","name":"测试节点","addr":"http://127.0.0.1:9999","role":"secondary"}`)))
	if add.Code != http.StatusOK {
		t.Fatalf("add status = %d body=%s", add.Code, add.Body.String())
	}
	favorite := httptest.NewRecorder()
	mux.ServeHTTP(favorite, httptest.NewRequest(http.MethodPost, "/api/v2/core/xpack/nodes/favorite", strings.NewReader(`{"nodeId":"secondary-1","isFavorite":true}`)))
	if favorite.Code != http.StatusOK {
		t.Fatalf("favorite status = %d", favorite.Code)
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodPost, "/api/v2/core/nodes/list", strings.NewReader(`{"search":"测试"}`)))
	var response struct {
		Data []nodeListItem `json:"data"`
	}
	if err := json.NewDecoder(list.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if list.Code != http.StatusOK || len(response.Data) != 1 || !response.Data[0].IsFavorite {
		t.Fatalf("节点列表异常: %#v", response.Data)
	}
	del := httptest.NewRecorder()
	mux.ServeHTTP(del, httptest.NewRequest(http.MethodPost, "/api/v2/core/nodes/del", strings.NewReader(`{"nodeId":"secondary-1"}`)))
	if del.Code != http.StatusOK {
		t.Fatalf("delete status = %d", del.Code)
	}
}

func TestNodeAddRejectsUnsafeEndpoint(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	RegisterRoleRoutesWithManager(mux, NewRoleManager("local", "primary"))
	for _, addr := range []string{"/etc/passwd", "file:///tmp/node", "http://user:pass@example.com:9999"} {
		response := httptest.NewRecorder()
		body := strings.NewReader(`{"nodeId":"unsafe","addr":"` + addr + `","role":"secondary"}`)
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v2/core/nodes/add", body))
		if response.Code != http.StatusBadRequest {
			t.Errorf("addr %q status=%d, want %d", addr, response.Code, http.StatusBadRequest)
		}
	}
}

func TestRoleControllerRestoresPersistentEpoch(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	first := NewRoleManager("persistent-node", "primary")
	mux := http.NewServeMux()
	RegisterRoleRoutesWithManager(mux, first)
	transition := `{"operationId":"persist-switch","nodeId":"persistent-node","from":"primary","to":"secondary","expectedEpoch":1}`
	prepare := httptest.NewRecorder()
	mux.ServeHTTP(prepare, httptest.NewRequest(http.MethodPost, "/api/v2/core/nodes/role/prepare", strings.NewReader(transition)))
	if prepare.Code != http.StatusOK {
		t.Fatalf("prepare status=%d body=%s", prepare.Code, prepare.Body.String())
	}
	commit := httptest.NewRecorder()
	mux.ServeHTTP(commit, httptest.NewRequest(http.MethodPost, "/api/v2/core/nodes/role/commit", strings.NewReader(`{}`)))
	if commit.Code != http.StatusOK {
		t.Fatalf("commit status=%d body=%s", commit.Code, commit.Body.String())
	}
	second := NewRoleManager("persistent-node", "primary")
	state := second.State(nil)
	if state.Role != "secondary" || state.RoleEpoch != 2 {
		t.Fatalf("角色状态未跨重启恢复: %+v", state)
	}
}
