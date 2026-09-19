// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"sync"
	"time"
)

// RuntimeLimits 是节点执行面的进程级资源预算。
type RuntimeLimits struct {
	MaxConcurrentTasks       int
	MaxConcurrentConversions int
	MaxSSEStreams            int
	MaxAIJobs                int
	MaxLogBytes              int64
	CacheTTL                 time.Duration
}

type runtimeLimitState struct {
	sync.RWMutex
	limits      RuntimeLimits
	tasks       chan struct{}
	conversions chan struct{}
	streams     chan struct{}
	aiJobs      chan struct{}
}

var nodeRuntimeLimits = newRuntimeLimitState(RuntimeLimits{
	MaxConcurrentTasks: 4, MaxConcurrentConversions: 2, MaxSSEStreams: 64,
	MaxAIJobs: 4, MaxLogBytes: 8 << 20, CacheTTL: 5 * time.Minute,
})

var managedRuntimeSlots = struct {
	sync.Mutex
	tasks  map[string]func()
	aiJobs map[string]func()
}{tasks: make(map[string]func()), aiJobs: make(map[string]func())}

func newRuntimeLimitState(limits RuntimeLimits) *runtimeLimitState {
	return &runtimeLimitState{limits: limits, tasks: make(chan struct{}, limits.MaxConcurrentTasks), conversions: make(chan struct{}, limits.MaxConcurrentConversions), streams: make(chan struct{}, limits.MaxSSEStreams), aiJobs: make(chan struct{}, limits.MaxAIJobs)}
}

// SetRuntimeLimits 在路由注册前注入统一配置。运行期间不支持热修改，避免替换仍有占用者的信号量。
func SetRuntimeLimits(limits RuntimeLimits) error {
	if limits.MaxConcurrentTasks < 1 || limits.MaxConcurrentConversions < 1 || limits.MaxSSEStreams < 1 || limits.MaxAIJobs < 1 || limits.MaxLogBytes < 1 || limits.CacheTTL <= 0 {
		return errors.New("运行时资源上限必须大于 0")
	}
	nodeRuntimeLimits = newRuntimeLimitState(limits)
	return nil
}

func tryRuntimeSlot(slots chan struct{}) (func(), bool) {
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, true
	default:
		return nil, false
	}
}

func runtimeMaxLogBytes() int64        { return nodeRuntimeLimits.limits.MaxLogBytes }
func runtimeCatalogTTL() time.Duration { return nodeRuntimeLimits.limits.CacheTTL }

func acquireManagedSlot(items map[string]func(), id string, slots chan struct{}) (bool, bool) {
	managedRuntimeSlots.Lock()
	defer managedRuntimeSlots.Unlock()
	if _, exists := items[id]; exists {
		return false, true
	}
	release, ok := tryRuntimeSlot(slots)
	if !ok {
		return false, false
	}
	items[id] = release
	return true, true
}

func releaseManagedSlot(items map[string]func(), id string) {
	managedRuntimeSlots.Lock()
	release := items[id]
	delete(items, id)
	managedRuntimeSlots.Unlock()
	if release != nil {
		release()
	}
}
