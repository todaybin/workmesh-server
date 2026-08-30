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

// nodeListItem 是前端节点选择器使用的兼容字段集合。
type nodeListItem struct {
	ID          string `json:"id"`
	NodeID      string `json:"nodeId"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	Endpoint    string `json:"endpoint,omitempty"`
	IsCurrent   bool   `json:"isCurrent"`
}

// RegisterRoleRoutes 注册角色状态、预检查、准备、提交、取消接口。
func RegisterRoleRoutes(mux *http.ServeMux, nodeID, initialRole string) {
	RegisterRoleRoutesWithManager(mux, NewRoleManager(nodeID, initialRole))
}

// NewRoleManager 创建控制面和节点链路共同使用的角色管理器。
// 初始化失败时沿用旧接口的兼容行为，回退到次节点角色。
func NewRoleManager(nodeID, initialRole string) *role.Manager {
	manager, err := role.New(nodeID, initialRole)
	if err != nil {
		manager, _ = role.New(nodeID, role.Secondary)
	}
	return manager
}

// RegisterRoleRoutesWithManager 使用指定管理器注册角色接口，确保 fencing 与角色查询共享 epoch。
func RegisterRoleRoutesWithManager(mux *http.ServeMux, manager *role.Manager) {
	if manager == nil {
		return
	}
	controller := &RoleController{manager: manager}
	// 节点列表是前端切换主/次节点的基础接口，必须返回真实的当前节点而非占位响应。
	mux.HandleFunc("POST /api/v2/core/nodes/list", controller.list)
	mux.HandleFunc("GET /api/v2/core/nodes/simple/all", controller.list)
	mux.HandleFunc("GET /api/v2/core/nodes/role", controller.current)
	mux.HandleFunc("POST /api/v2/core/nodes/role/check", controller.check)
	mux.HandleFunc("POST /api/v2/core/nodes/role/prepare", controller.prepare)
	mux.HandleFunc("POST /api/v2/core/nodes/role/commit", controller.commit)
	mux.HandleFunc("POST /api/v2/core/nodes/role/abort", controller.abort)
}

func (c *RoleController) list(w http.ResponseWriter, r *http.Request) {
	state := c.manager.State(r.Context())
	item := nodeListItem{
		ID: state.NodeID, NodeID: state.NodeID, Name: state.NodeID,
		DisplayName: state.NodeID, Role: state.Role, Status: "online", IsCurrent: true,
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": []nodeListItem{item}})
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
