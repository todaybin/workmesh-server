// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import "context"

// contextError 返回任务操作的取消原因，供所有平台共用。
func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// CgroupV2Handle 标识一个任务对应的 cgroup v2 子组。
type CgroupV2Handle struct {
	TaskID string `json:"taskId"`
	Path   string `json:"path"`
}

// CgroupV2Controller 管理任务级 CPU、内存和 PID 硬限制。
// 磁盘容量需要文件系统 quota 或 MicroVM 磁盘配额，不能由 cgroup v2 单独提供。
type CgroupV2Controller struct {
	root   string
	parent string
}

// NewCgroupV2Controller 创建 cgroup v2 控制器并检查节点能力。
func NewCgroupV2Controller(root, parent string) (*CgroupV2Controller, error) {
	return newCgroupV2Controller(root, parent)
}

// Create 创建任务 cgroup 并写入 CPU、内存和 PID 硬限制。
func (c *CgroupV2Controller) Create(ctx context.Context, taskID string, limits ResourceLimits) (CgroupV2Handle, error) {
	return c.create(ctx, taskID, limits)
}

// Attach 将进程加入任务 cgroup。
func (c *CgroupV2Controller) Attach(ctx context.Context, handle CgroupV2Handle, pid int) error {
	return c.attach(ctx, handle, pid)
}

// Destroy 在确认没有进程残留后删除任务 cgroup。
func (c *CgroupV2Controller) Destroy(ctx context.Context, handle CgroupV2Handle) error {
	return c.destroy(ctx, handle)
}
