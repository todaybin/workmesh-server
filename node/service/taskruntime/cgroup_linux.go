//go:build linux

// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const cgroupV2PeriodMicros int64 = 100_000

var cgroupNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func newCgroupV2Controller(root, parent string) (*CgroupV2Controller, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "/sys/fs/cgroup"
	}
	if !isAbsolutePath(root) {
		return nil, errors.New("cgroup v2 root 必须是绝对路径")
	}
	root = filepath.Clean(root)
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("读取 cgroup v2 root 失败: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("cgroup v2 root 必须是非符号链接目录")
	}
	controllers, err := os.ReadFile(filepath.Join(root, "cgroup.controllers"))
	if err != nil {
		return nil, fmt.Errorf("读取 cgroup v2 controllers 失败: %w", err)
	}
	available := map[string]bool{}
	for _, name := range strings.Fields(string(controllers)) {
		available[name] = true
	}
	for _, name := range []string{"cpu", "memory", "pids"} {
		if !available[name] {
			return nil, fmt.Errorf("cgroup v2 缺少 %s controller", name)
		}
	}
	parent = strings.TrimSpace(parent)
	if parent == "" {
		parent = "workmesh-agent"
	}
	if !cgroupNamePattern.MatchString(parent) {
		return nil, errors.New("cgroup v2 parent 名称无效")
	}
	return &CgroupV2Controller{root: root, parent: parent}, nil
}

func (c *CgroupV2Controller) create(ctx context.Context, taskID string, limits ResourceLimits) (CgroupV2Handle, error) {
	if err := contextError(ctx); err != nil {
		return CgroupV2Handle{}, err
	}
	if !taskIDPattern.MatchString(taskID) {
		return CgroupV2Handle{}, errors.New("cgroup v2 taskId 格式无效")
	}
	if limits.CPUQuotaMicros <= 0 || limits.MemoryBytes <= 0 || limits.PIDsMax <= 0 {
		return CgroupV2Handle{}, errors.New("cgroup v2 CPU、内存和 PID 限制必须为正数")
	}
	if limits.DiskBytes <= 0 {
		return CgroupV2Handle{}, errors.New("cgroup v2 控制器不能单独提供磁盘容量硬限制")
	}
	parentPath := filepath.Join(c.root, c.parent)
	if err := os.MkdirAll(parentPath, 0o750); err != nil {
		return CgroupV2Handle{}, fmt.Errorf("创建 cgroup v2 parent 失败: %w", err)
	}
	if err := rejectSymlinkPath(filepath.ToSlash(c.root), filepath.ToSlash(parentPath)); err != nil {
		return CgroupV2Handle{}, fmt.Errorf("cgroup v2 parent 路径不安全: %w", err)
	}
	if err := ensureCgroupControllers(parentPath); err != nil {
		return CgroupV2Handle{}, err
	}
	taskPath := filepath.Join(parentPath, taskID)
	if err := os.Mkdir(taskPath, 0o750); err != nil {
		return CgroupV2Handle{}, fmt.Errorf("创建任务 cgroup 失败: %w", err)
	}
	if info, statErr := os.Lstat(taskPath); statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		_ = os.Remove(taskPath)
		if statErr != nil {
			return CgroupV2Handle{}, fmt.Errorf("检查任务 cgroup 失败: %w", statErr)
		}
		return CgroupV2Handle{}, errors.New("任务 cgroup 不能是符号链接")
	}
	cleanup := func() { _ = os.Remove(taskPath) }
	if err := writeCgroupValue(taskPath, "cpu.max", fmt.Sprintf("%d %d", limits.CPUQuotaMicros, cgroupV2PeriodMicros)); err != nil {
		cleanup()
		return CgroupV2Handle{}, err
	}
	if err := writeCgroupValue(taskPath, "memory.max", strconv.FormatInt(limits.MemoryBytes, 10)); err != nil {
		cleanup()
		return CgroupV2Handle{}, err
	}
	if err := writeCgroupValue(taskPath, "memory.swap.max", "0"); err != nil {
		cleanup()
		return CgroupV2Handle{}, err
	}
	if err := writeCgroupValue(taskPath, "pids.max", strconv.FormatInt(limits.PIDsMax, 10)); err != nil {
		cleanup()
		return CgroupV2Handle{}, err
	}
	return CgroupV2Handle{TaskID: taskID, Path: taskPath}, nil
}

func (c *CgroupV2Controller) attach(ctx context.Context, handle CgroupV2Handle, pid int) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if pid <= 0 {
		return errors.New("cgroup v2 进程 PID 必须为正数")
	}
	path, err := c.resolveHandle(handle)
	if err != nil {
		return err
	}
	return writeCgroupValue(path, "cgroup.procs", strconv.Itoa(pid))
}

func (c *CgroupV2Controller) destroy(ctx context.Context, handle CgroupV2Handle) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	path, err := c.resolveHandle(handle)
	if err != nil {
		return err
	}
	procs, err := os.ReadFile(filepath.Join(path, "cgroup.procs"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("读取 cgroup v2 进程列表失败: %w", err)
	}
	if strings.TrimSpace(string(procs)) != "" {
		return errors.New("任务 cgroup 仍有进程，拒绝删除")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除任务 cgroup 失败: %w", err)
	}
	return nil
}

func (c *CgroupV2Controller) resolveHandle(handle CgroupV2Handle) (string, error) {
	if c == nil || !taskIDPattern.MatchString(handle.TaskID) {
		return "", errors.New("cgroup v2 句柄无效")
	}
	expected := filepath.Join(c.root, c.parent, handle.TaskID)
	if filepath.Clean(handle.Path) != expected {
		return "", errors.New("cgroup v2 句柄路径不匹配")
	}
	info, err := os.Lstat(expected)
	if err != nil {
		return "", fmt.Errorf("读取任务 cgroup 失败: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("任务 cgroup 必须是非符号链接目录")
	}
	return expected, nil
}

func ensureCgroupControllers(parentPath string) error {
	path := filepath.Join(parentPath, "cgroup.subtree_control")
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 cgroup v2 subtree_control 失败: %w", err)
	}
	fields := strings.Fields(string(content))
	set := map[string]bool{}
	for _, field := range fields {
		set[strings.TrimPrefix(field, "+")] = true
	}
	missing := make([]string, 0, 3)
	for _, name := range []string{"cpu", "memory", "pids"} {
		if !set[name] {
			missing = append(missing, "+"+name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if err := os.WriteFile(path, []byte(strings.Join(missing, " ")), 0o640); err != nil {
		return fmt.Errorf("启用 cgroup v2 controllers 失败: %w", err)
	}
	return nil
}

func writeCgroupValue(dir, name, value string) error {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(value), 0o640); err != nil {
		return fmt.Errorf("写入 cgroup v2 %s 失败: %w", name, err)
	}
	return nil
}
