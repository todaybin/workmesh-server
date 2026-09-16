// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"strings"
	"sync"
)

// runtimeOperationLocks 按运行时 ID 串行化外部操作，不阻塞其他运行时的读写。
// 锁顺序固定为资源锁后状态锁；最后一个等待者退出后回收条目。
type runtimeOperationLocks struct {
	mu      sync.Mutex
	entries map[string]*runtimeOperationLock
}

// runtimeOperationLock 保存单个资源锁及持有者、等待者总数。
type runtimeOperationLock struct {
	mu    sync.Mutex
	users int
}

// lock 获取资源锁并返回释放方法；等待期间不持有运行时状态锁。
func (locks *runtimeOperationLocks) lock(id string) func() {
	locks.mu.Lock()
	if locks.entries == nil {
		locks.entries = make(map[string]*runtimeOperationLock)
	}
	entry := locks.entries[id]
	if entry == nil {
		entry = &runtimeOperationLock{}
		locks.entries[id] = entry
	}
	entry.users++
	locks.mu.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		locks.mu.Lock()
		entry.users--
		if entry.users == 0 {
			delete(locks.entries, id)
		}
		locks.mu.Unlock()
	}
}

// validRuntimeOperation 拒绝把任意用户输入当作 Docker 子命令执行。
// up/down/restart 是原前端契约，start/stop 仅保留当前客户端兼容。
func validRuntimeOperation(operation string) bool {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "up", "down", "restart", "start", "stop":
		return true
	default:
		return false
	}
}
