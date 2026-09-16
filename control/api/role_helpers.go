// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"hash/fnv"
	"os"
	"strings"
)

// boolIntRole 将节点收藏布尔值转换为 SQLite 使用的整数值。
func boolIntRole(value bool) int {
	if value {
		return 1
	}
	return 0
}

// nodeNumericID 将节点字符串 ID 稳定映射为前端兼容的正整数 ID。
func nodeNumericID(nodeID string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(nodeID))
	id := int(h.Sum32() & 0x7fffffff)
	if id == 0 {
		return 1
	}
	return id
}

// roleNodeItem 根据角色状态生成前端节点选择器所需的本机节点条目。
func (c *RoleController) nodeItem(nodeID, nodeRole string, current bool) nodeListItem {
	name := nodeID
	if name == "" {
		name = "local"
	}
	addr := strings.TrimSpace(os.Getenv("WORKMESH_NODE_ENDPOINT_URL"))
	if addr == "" {
		addr = "127.0.0.1"
	}
	return nodeListItem{ID: nodeNumericID(nodeID), NodeID: nodeID, Name: name, Addr: addr, Version: "workmesh-server", SystemVersion: "workmesh-server", IsBound: true, DisplayName: name, Role: nodeRole, Status: "online", IsCurrent: current}
}
