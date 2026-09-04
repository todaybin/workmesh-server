// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/todaybin/workmesh-server/runtime/link"
	"github.com/todaybin/workmesh-server/runtime/role"
)

// RegisterLinkRoutes 注册节点握手、同步和 fencing 接口。
// 控制面和链路面复用同一个 role.Manager，确保所有写操作校验同一 roleEpoch。
func RegisterLinkRoutes(mux *http.ServeMux, nodeID, initialRole string, manager *role.Manager) *link.Server {
	if manager == nil {
		manager = NewRoleManager(nodeID, initialRole)
	}
	var syncStore link.SyncStore
	if dataDir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR")); dataDir != "" {
		if persisted, storeErr := link.NewFileSyncStore(filepath.Join(dataDir, "link-sync.json")); storeErr == nil {
			syncStore = persisted
		}
	}
	if syncStore == nil {
		syncStore = link.NewMemorySyncStore()
	}
	server, err := link.NewServer(link.ServerOptions{
		NodeID:          nodeID,
		Role:            initialRole,
		ProtocolVersion: "v2",
		Capabilities:    []string{"system", "containers", "files", "sync", "fencing"},
		Secret:          []byte(os.Getenv("WORKMESH_LINK_SECRET")),
		RoleManager:     manager,
		SyncStore:       syncStore,
	})
	if err != nil {
		return nil
	}
	server.Register(mux)
	return server
}
