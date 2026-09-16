// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"github.com/todaybin/workmesh-server/runtime/role"
)

// RoleController 提供本机主节点与次节点的安全切换接口。
type RoleController struct {
	mu         sync.Mutex
	manager    *role.Manager
	state      role.Transition
	nodesMu    sync.RWMutex
	nodes      map[string]nodeListItem
	nodesPath  string
	db         *sql.DB
	repository storage.Transactional
}

// nodeListItem 是前端节点选择器使用的兼容字段集合。
type nodeListItem struct {
	ID                int     `json:"id"`
	NodeID            string  `json:"nodeId"`
	Name              string  `json:"name"`
	Addr              string  `json:"addr"`
	Version           string  `json:"version"`
	IsXpack           bool    `json:"isXpack"`
	IsBound           bool    `json:"isBound"`
	DisplayName       string  `json:"displayName"`
	Role              string  `json:"role"`
	Status            string  `json:"status"`
	Endpoint          string  `json:"endpoint,omitempty"`
	Description       string  `json:"description,omitempty"`
	SystemVersion     string  `json:"systemVersion,omitempty"`
	SecurityEntrance  string  `json:"securityEntrance,omitempty"`
	CPUUsedPercent    float64 `json:"cpuUsedPercent,omitempty"`
	CPUTotal          int     `json:"cpuTotal,omitempty"`
	MemoryTotal       int64   `json:"memoryTotal,omitempty"`
	MemoryUsedPercent float64 `json:"memoryUsedPercent,omitempty"`
	IsFavorite        bool    `json:"isFavorite,omitempty"`
	IsCurrent         bool    `json:"isCurrent"`
}

// RegisterRoleRoutes 注册角色状态、预检查、准备、提交、取消接口。
func RegisterRoleRoutes(mux *http.ServeMux, nodeID, initialRole string) {
	RegisterRoleRoutesWithManager(mux, NewRoleManager(nodeID, initialRole))
}

// NewRoleManager 创建控制面和节点链路共同使用的角色管理器。
// 初始化失败时沿用旧接口的兼容行为，回退到次节点角色。
func NewRoleManager(nodeID, initialRole string) *role.Manager {
	// 生产环境通过 WORKMESH_DATA_DIR 持久化角色 epoch；未设置时保持纯内存模式，
	// 避免开发测试在仓库目录产生状态文件。
	dataDir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	var manager *role.Manager
	var err error
	if dataDir != "" {
		if db := controlDB; db != nil {
			manager, err = role.NewSQLite(db, nodeID, initialRole)
		} else {
			statePath := filepath.Join(dataDir, "role-state.json")
			if absolute, absErr := filepath.Abs(statePath); absErr == nil {
				statePath = absolute
			}
			manager, err = role.NewPersistent(nodeID, initialRole, statePath)
		}
	} else {
		manager, err = role.New(nodeID, initialRole)
	}
	if err != nil {
		manager, _ = role.New(nodeID, role.Secondary)
	}
	return manager
}

// RegisterRoleRoutesWithManager 将角色及节点路由绑定到共享 epoch 管理器和可选鉴权器。
func RegisterRoleRoutesWithManager(mux *http.ServeMux, manager *role.Manager, authorizers ...RequestAuthorizer) {
	if manager == nil {
		return
	}
	controller := newRoleController(manager)
	var authorize RequestAuthorizer
	if len(authorizers) > 0 {
		authorize = authorizers[0]
	}
	write := func(handler http.HandlerFunc) http.HandlerFunc {
		if authorize == nil {
			return handler
		}
		return func(w http.ResponseWriter, r *http.Request) {
			if !authorize(r) {
				wmhttp.JSON(w, http.StatusUnauthorized, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "LOCAL_AUTH_REQUIRED"}, "message": "需要有效的本地登录会话"})
				return
			}
			handler(w, r)
		}
	}
	// 节点列表是前端切换主/次节点的基础接口，必须返回真实的当前节点而非占位响应。
	mux.HandleFunc("POST /api/v2/core/nodes/list", controller.list)
	mux.HandleFunc("GET /api/v2/core/nodes/list", controller.list)
	mux.HandleFunc("GET /api/v2/core/nodes/simple/all", controller.list)
	mux.HandleFunc("POST /api/v2/core/nodes/add", write(controller.addNode))
	mux.HandleFunc("POST /api/v2/core/nodes/update", write(controller.updateNode))
	mux.HandleFunc("POST /api/v2/core/nodes/del", write(controller.deleteNode))
	mux.HandleFunc("POST /api/v2/core/nodes/delete", write(controller.deleteNode))
	// 商业版前端曾使用 core/xpack 前缀，保留同一真实节点注册实现。
	mux.HandleFunc("POST /api/v2/core/xpack/nodes/add", write(controller.addNode))
	mux.HandleFunc("POST /api/v2/core/xpack/nodes/update", write(controller.updateNode))
	mux.HandleFunc("POST /api/v2/core/xpack/nodes/del", write(controller.deleteNode))
	mux.HandleFunc("POST /api/v2/core/xpack/nodes/delete", write(controller.deleteNode))
	mux.HandleFunc("POST /api/v2/core/xpack/nodes/search", controller.list)
	mux.HandleFunc("GET /api/v2/core/xpack/nodes/list", controller.list)
	mux.HandleFunc("POST /api/v2/core/xpack/nodes/favorite", write(controller.favorite))
	mux.HandleFunc("GET /api/v2/core/nodes/role", controller.current)
	mux.HandleFunc("POST /api/v2/core/nodes/role/check", controller.check)
	mux.HandleFunc("POST /api/v2/core/nodes/role/prepare", write(controller.prepare))
	mux.HandleFunc("POST /api/v2/core/nodes/role/commit", write(controller.commit))
	mux.HandleFunc("POST /api/v2/core/nodes/role/abort", write(controller.abort))
}

// newRoleController 创建控制器并从 SQLite 或兼容节点文件恢复节点列表。
func newRoleController(manager *role.Manager) *RoleController {
	dataDir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dataDir == "" {
		dataDir = "./data"
	}
	c := &RoleController{manager: manager, nodes: make(map[string]nodeListItem), nodesPath: filepath.Join(dataDir, "nodes.json"), db: controlDB}
	state := manager.State(nil)
	current := c.nodeItem(state.NodeID, state.Role, true)
	c.nodes[state.NodeID] = current
	if c.db != nil {
		_, _ = c.db.Exec(`CREATE TABLE IF NOT EXISTS role_nodes (node_id TEXT PRIMARY KEY, id INTEGER NOT NULL, name TEXT NOT NULL, addr TEXT NOT NULL, endpoint TEXT NOT NULL DEFAULT '', role TEXT NOT NULL, status TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', is_favorite INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL)`)
		c.repository, _ = storage.NewSQLiteRepository(c.db)
		rows, _ := c.repository.Query(`SELECT node_id,id,name,addr,endpoint,role,status,description,is_favorite FROM role_nodes`)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var item nodeListItem
				var favorite int
				if rows.Scan(&item.NodeID, &item.ID, &item.Name, &item.Addr, &item.Endpoint, &item.Role, &item.Status, &item.Description, &favorite) == nil {
					item.IsFavorite = favorite != 0
					item.DisplayName = item.Name
					c.nodes[item.NodeID] = item
				}
			}
		}
	} else if b, err := os.ReadFile(c.nodesPath); err == nil {
		var saved []nodeListItem
		if json.Unmarshal(b, &saved) == nil {
			for _, item := range saved {
				if strings.TrimSpace(item.NodeID) != "" {
					c.nodes[item.NodeID] = item
				}
			}
		}
	}
	return c
}

// persistNodes 在 SQLite 可用时更新关系表，否则原子写入兼容节点文件。
func (c *RoleController) persistNodes() error {
	items := make([]nodeListItem, 0, len(c.nodes))
	for _, item := range c.nodes {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	if c.repository != nil {
		return c.repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
			if _, err := tx.Exec(`DELETE FROM role_nodes`); err != nil {
				return err
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			for _, item := range items {
				if _, err := tx.Exec(`INSERT INTO role_nodes(node_id,id,name,addr,endpoint,role,status,description,is_favorite,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, item.NodeID, item.ID, item.Name, item.Addr, item.Endpoint, item.Role, item.Status, item.Description, boolIntRole(item.IsFavorite), now); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := os.MkdirAll(filepath.Dir(c.nodesPath), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(items)
	if err != nil {
		return err
	}
	tmp := c.nodesPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.nodesPath)
}

// list 返回真实节点列表并按请求搜索条件过滤当前节点状态。
func (c *RoleController) list(w http.ResponseWriter, r *http.Request) {
	state := c.manager.State(r.Context())
	c.nodesMu.Lock()
	if item, ok := c.nodes[state.NodeID]; ok {
		item.Role, item.IsCurrent, item.Status = state.Role, true, "online"
		c.nodes[state.NodeID] = item
	} else {
		c.nodes[state.NodeID] = c.nodeItem(state.NodeID, state.Role, true)
	}
	items := make([]nodeListItem, 0, len(c.nodes))
	for _, item := range c.nodes {
		item.IsCurrent = item.NodeID == state.NodeID
		items = append(items, item)
	}
	c.nodesMu.Unlock()
	// 支持旧接口的 type/search 参数，未知类型不改变结果，保证客户端向前兼容。
	var filter map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&filter)
	}
	if query, ok := filter["search"].(string); ok && strings.TrimSpace(query) != "" {
		query = strings.ToLower(strings.TrimSpace(query))
		filtered := items[:0]
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Name), query) || strings.Contains(strings.ToLower(item.Addr), query) || strings.Contains(strings.ToLower(item.NodeID), query) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	sort.Slice(items, func(i, j int) bool { return items[i].IsCurrent && !items[j].IsCurrent })
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": items})
}

// addNode 校验并登记远端节点地址、角色和展示信息。
func (c *RoleController) addNode(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID          string `json:"id"`
		NodeID      string `json:"nodeId"`
		Name        string `json:"name"`
		Addr        string `json:"addr"`
		Endpoint    string `json:"endpoint"`
		Role        string `json:"role"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	nodeID := strings.TrimSpace(request.NodeID)
	if nodeID == "" {
		nodeID = strings.TrimSpace(request.ID)
	}
	if nodeID == "" {
		nodeID = strings.TrimSpace(request.Name)
	}
	if nodeID == "" || strings.TrimSpace(request.Addr) == "" && strings.TrimSpace(request.Endpoint) == "" {
		writeError(w, http.StatusBadRequest, errors.New("节点 ID 和地址不能为空"))
		return
	}
	if request.Role == "" {
		request.Role = role.Secondary
	}
	if request.Role != role.Primary && request.Role != role.Secondary {
		writeError(w, http.StatusBadRequest, errors.New("节点角色必须为 primary 或 secondary"))
		return
	}
	addr := strings.TrimSpace(request.Addr)
	if addr == "" {
		addr = strings.TrimSpace(request.Endpoint)
	}
	parsed, parseErr := url.Parse(addr)
	if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || strings.ContainsAny(addr, "\r\n") {
		writeError(w, http.StatusBadRequest, errors.New("节点地址必须是无用户信息的 HTTP(S) URL"))
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = nodeID
	}
	item := nodeListItem{ID: nodeNumericID(nodeID), NodeID: nodeID, Name: name, DisplayName: name, Addr: addr, Endpoint: addr, Role: request.Role, Version: "workmesh-server", Status: "registered", IsBound: false, IsCurrent: false}
	c.nodesMu.Lock()
	if _, exists := c.nodes[nodeID]; exists {
		c.nodesMu.Unlock()
		writeError(w, http.StatusConflict, errors.New("节点已存在"))
		return
	}
	c.nodes[nodeID] = item
	err := c.persistNodes()
	c.nodesMu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
}

// updateNode 更新已登记节点的名称、地址、状态或角色后持久化变更。
func (c *RoleController) updateNode(w http.ResponseWriter, r *http.Request) {
	var request nodeListItem
	if err := decodeJSON(r, &request); err != nil || strings.TrimSpace(request.NodeID) == "" && strings.TrimSpace(request.Name) == "" {
		writeError(w, http.StatusBadRequest, errors.New("节点 ID 不能为空"))
		return
	}
	nodeID := request.NodeID
	if nodeID == "" {
		nodeID = request.Name
	}
	c.nodesMu.Lock()
	item, ok := c.nodes[nodeID]
	if !ok {
		c.nodesMu.Unlock()
		writeError(w, http.StatusNotFound, errors.New("节点不存在"))
		return
	}
	if request.Name != "" {
		item.Name, item.DisplayName = request.Name, request.Name
	}
	if request.Addr != "" {
		item.Addr, item.Endpoint = request.Addr, request.Addr
	}
	if request.Status != "" {
		item.Status = request.Status
	}
	if request.Role == role.Primary || request.Role == role.Secondary {
		item.Role = request.Role
	}
	c.nodes[nodeID] = item
	err := c.persistNodes()
	c.nodesMu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
}

// deleteNode 删除非当前节点的登记信息，避免误删正在提供服务的节点。
func (c *RoleController) deleteNode(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID     int    `json:"id"`
		NodeID string `json:"nodeId"`
		Name   string `json:"name"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	nodeID := strings.TrimSpace(request.NodeID)
	if nodeID == "" {
		nodeID = strings.TrimSpace(request.Name)
	}
	state := c.manager.State(r.Context())
	if nodeID == "" || nodeID == state.NodeID {
		writeError(w, http.StatusBadRequest, errors.New("不能删除当前节点"))
		return
	}
	c.nodesMu.Lock()
	if _, ok := c.nodes[nodeID]; !ok {
		c.nodesMu.Unlock()
		writeError(w, http.StatusNotFound, errors.New("节点不存在"))
		return
	}
	delete(c.nodes, nodeID)
	err := c.persistNodes()
	c.nodesMu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}

// favorite 按节点 ID 或数值 ID 更新节点收藏标记。
func (c *RoleController) favorite(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ID         int    `json:"id"`
		NodeID     string `json:"nodeId"`
		IsFavorite bool   `json:"isFavorite"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	c.nodesMu.Lock()
	var found string
	for id, item := range c.nodes {
		if (request.NodeID != "" && id == request.NodeID) || (request.ID != 0 && item.ID == request.ID) {
			item.IsFavorite = request.IsFavorite
			c.nodes[id] = item
			found = id
			break
		}
	}
	if found == "" {
		c.nodesMu.Unlock()
		writeError(w, http.StatusNotFound, errors.New("节点不存在"))
		return
	}
	err := c.persistNodes()
	c.nodesMu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}

// current 返回角色管理器当前节点、角色和 epoch 状态。
func (c *RoleController) current(w http.ResponseWriter, r *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": c.manager.State(r.Context())})
}

// check 校验角色切换请求的目标角色、节点身份和预期 epoch。
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

// prepare 校验并暂存一次带操作 ID 的角色切换请求。
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

// commit 使用暂存请求执行角色切换，并在成功后清理过渡状态。
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

// abort 清理尚未提交的角色切换过渡状态。
func (c *RoleController) abort(w http.ResponseWriter, _ *http.Request) {
	c.mu.Lock()
	c.state = role.Transition{}
	c.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}
