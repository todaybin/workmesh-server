// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// validateSignedNode 防止持有有效签名的节点替其他节点写入握手或心跳状态。
func (s *Server) validateSignedNode(r *http.Request, bodyNodeID string) error {
	if len(s.options.Secret) == 0 {
		return nil
	}
	if signedNodeID := firstHeader(r, HeaderNodeID, "X-Node-ID"); signedNodeID != bodyNodeID {
		return errors.New("链路签名节点与请求节点不一致")
	}
	return nil
}

// validateSyncRoleEpoch 对任务流强制 fencing，并拒绝所有显式携带的旧 epoch。
// 普通控制面同步暂时允许省略 epoch，保持现有 v2 客户端兼容。
func validateSyncRoleEpoch(stream string, requestEpoch, currentEpoch uint64) error {
	if requestEpoch == 0 {
		if isTaskSyncStream(stream) {
			return fmt.Errorf("%w: 任务同步缺少 roleEpoch", ErrSyncConflict)
		}
		return nil
	}
	if requestEpoch != currentEpoch {
		return fmt.Errorf("%w: roleEpoch=%d current=%d", ErrSyncConflict, requestEpoch, currentEpoch)
	}
	return nil
}

// isTaskSyncStream 识别跨节点任务及其结果/日志快照流。
func isTaskSyncStream(stream string) bool {
	stream = strings.ToLower(strings.TrimSpace(stream))
	return stream == "task" || stream == "tasks" || strings.HasPrefix(stream, "task.") || strings.HasPrefix(stream, "tasks.")
}
