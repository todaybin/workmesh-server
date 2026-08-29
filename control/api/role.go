// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"net/http"
	"sync"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"github.com/todaybin/workmesh-server/runtime/role"
)

// RoleController 提供本机主节点与次节点的安全切换接口。
type RoleController struct {
	mu      sync.Mutex
	manager *role.Manager
	state   role.Transition
}

// RegisterRoleRoutes 注册角色状态、预检查、准备、提交、取消接口。
func RegisterRoleRoutes(mux *http.ServeMux, nodeID, initialRole string) {
	manager, err := role.New(nodeID, initialRole)
	if err != nil {
		manager, _ = role.New(nodeID, role.Secondary)
	}
	controller := &RoleController{manager: manager}
	mux.HandleFunc("GET /api/v2/core/nodes/role", controller.current)
	mux.HandleFunc("POST /api/v2/core/nodes/role/check", controller.check)
	mux.HandleFunc("POST /api/v2/core/nodes/role/prepare", controller.prepare)
	mux.HandleFunc("POST /api/v2/core/nodes/role/commit", controller.commit)
	mux.HandleFunc("POST /api/v2/core/nodes/role/abort", controller.abort)
}

func (c *RoleController) current(w http.ResponseWriter, r *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": c.manager.State(r.Context())})
}

func (c *RoleController) check(w http.ResponseWriter, r *http.Request) {
	var request role.Transition
	if err := decodeJSON(r, &request); err != nil || request.To == "" {
		writeError(w, http.StatusBadRequest, errors.New("目标角色不能为空"))
		return
	}
	state := c.manager.State(r.Context())
	if request.ExpectedEpoch != state.RoleEpoch || request.NodeID != "" && request.NodeID != state.NodeID {
		writeError(w, http.StatusConflict, errors.New("角色版本或节点身份冲突"))
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"ready": true, "current": state}})
}

func (c *RoleController) prepare(w http.ResponseWriter, r *http.Request) {
	var request role.Transition
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	state := c.manager.State(r.Context())
	if request.OperationID == "" {
		writeError(w, http.StatusBadRequest, errors.New("操作 ID 不能为空"))
		return
	}
	if request.ExpectedEpoch != state.RoleEpoch {
		writeError(w, http.StatusConflict, errors.New("角色版本冲突"))
		return
	}
	c.mu.Lock()
	c.state = request
	c.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"status": "prepared"}})
}

func (c *RoleController) commit(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	request := c.state
	c.mu.Unlock()
	if request.OperationID == "" {
		writeError(w, http.StatusConflict, errors.New("没有待提交的角色切换"))
		return
	}
	state, err := c.manager.Switch(r.Context(), request.ExpectedEpoch, request.To)
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	c.mu.Lock()
	c.state = role.Transition{}
	c.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": state})
}

func (c *RoleController) abort(w http.ResponseWriter, _ *http.Request) {
	c.mu.Lock()
	c.state = role.Transition{}
	c.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}
