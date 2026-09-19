// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/runtime/gateway"
	"github.com/todaybin/workmesh-server/runtime/resources"
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
	limits         RuntimeLimits
	tasks          chan struct{}
	conversions    chan struct{}
	streams        chan struct{}
	aiJobs         chan struct{}
	policy         gateway.ResourcePolicy
	policyRevision int64
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

func tryRuntimeSlotFor(kind string, slots chan struct{}) (func(), bool) {
	if limit := effectiveRuntimeLimit(kind); limit > 0 && len(slots) >= limit {
		return nil, false
	}
	return tryRuntimeSlot(slots)
}

func effectiveRuntimeLimit(kind string) int {
	nodeRuntimeLimits.RLock()
	defer nodeRuntimeLimits.RUnlock()
	local, remote := 0, 0
	switch kind {
	case "tasks":
		local, remote = nodeRuntimeLimits.limits.MaxConcurrentTasks, nodeRuntimeLimits.policy.MaxConcurrentTasks
	case "conversions":
		local, remote = nodeRuntimeLimits.limits.MaxConcurrentConversions, nodeRuntimeLimits.policy.MaxConcurrentConversions
	case "streams":
		local, remote = nodeRuntimeLimits.limits.MaxSSEStreams, nodeRuntimeLimits.policy.MaxSSEStreams
	case "aiJobs":
		local, remote = nodeRuntimeLimits.limits.MaxAIJobs, nodeRuntimeLimits.policy.MaxAIJobs
	}
	if nodeRuntimeLimits.policy.Mode == "enforce" && remote > 0 && (local == 0 || remote < local) {
		return remote
	}
	return local
}

// ApplyRemoteResourcePolicy 保存 Gateway 软策略；enforce 也只能收紧本地硬上限。
func ApplyRemoteResourcePolicy(policy gateway.ResourcePolicy, revision int64) error {
	if policy.Mode == "" {
		policy.Mode = "observe"
	}
	if policy.Mode != "observe" && policy.Mode != "enforce" {
		return errors.New("资源策略 mode 只允许 observe 或 enforce")
	}
	if policy.MaxConcurrentTasks < 0 || policy.MaxConcurrentConversions < 0 || policy.MaxSSEStreams < 0 || policy.MaxAIJobs < 0 || revision < 0 {
		return errors.New("资源策略上限和版本不能为负数")
	}
	nodeRuntimeLimits.Lock()
	if revision >= nodeRuntimeLimits.policyRevision {
		nodeRuntimeLimits.policy = policy
		nodeRuntimeLimits.policyRevision = revision
	}
	nodeRuntimeLimits.Unlock()
	return nil
}

// RuntimeResourceSnapshot 返回当前资源占用和活动租约，不创建独立采样协程。
func RuntimeResourceSnapshot() any {
	managedRuntimeSlots.Lock()
	managedTasks, managedAI := len(managedRuntimeSlots.tasks), len(managedRuntimeSlots.aiJobs)
	managedRuntimeSlots.Unlock()
	return resources.Collect(map[string]int{
		"tasks":       managedTasks,
		"aiJobs":      managedAI,
		"conversions": len(nodeRuntimeLimits.conversions),
		"streams":     len(nodeRuntimeLimits.streams),
	})
}

func runtimeMaxLogBytes() int64        { return nodeRuntimeLimits.limits.MaxLogBytes }
func runtimeCatalogTTL() time.Duration { return nodeRuntimeLimits.limits.CacheTTL }

func acquireManagedSlot(items map[string]func(), id string, kind string, slots chan struct{}) (bool, bool) {
	managedRuntimeSlots.Lock()
	defer managedRuntimeSlots.Unlock()
	if _, exists := items[id]; exists {
		return false, true
	}
	release, ok := tryRuntimeSlotFor(kind, slots)
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
