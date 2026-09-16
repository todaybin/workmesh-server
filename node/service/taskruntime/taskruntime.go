// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package taskruntime 提供任务级隔离运行时的受控生命周期适配。
package taskruntime

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// TaskState 是任务沙盒的生命周期状态。
type TaskState string

const (
	TaskCreated   TaskState = "created"
	TaskRunning   TaskState = "running"
	TaskCancelled TaskState = "cancelled"
	TaskCompleted TaskState = "completed"
	TaskFailed    TaskState = "failed"
	TaskDestroyed TaskState = "destroyed"
)

// RuntimePolicy 描述任务运行时必须满足的隔离约束。
type RuntimePolicy struct {
	SandboxType                 string   `json:"sandboxType"`
	Backend                     string   `json:"backend"`
	IsolationRequired           string   `json:"isolationRequired"`
	RiskClass                   string   `json:"riskClass"`
	Environment                 string   `json:"environment"`
	AllowRemoteDispatch         bool     `json:"allowRemoteDispatch"`
	AllowedHosts                []string `json:"allowedHosts,omitempty"`
	WorkspaceRef                string   `json:"workspaceRef,omitempty"`
	ProductionReleaseApprovalID string   `json:"productionReleaseApprovalId,omitempty"`
}

// TaskSpec 只描述经 HTTP 层校验的任务，不包含宿主 Shell 字符串。
type TaskSpec struct {
	TaskID        string        `json:"taskId"`
	ImageDigest   string        `json:"imageDigest"`
	Worktree      string        `json:"worktree"`
	Entrypoint    []string      `json:"entrypoint"`
	Timeout       time.Duration `json:"timeout"`
	RuntimePolicy RuntimePolicy `json:"runtimePolicy"`
}

// TaskHandle 是任务沙盒的公开句柄。
type TaskHandle struct {
	TaskID      string    `json:"taskId"`
	SandboxID   string    `json:"sandboxId"`
	ImageDigest string    `json:"imageDigest"`
	SandboxType string    `json:"sandboxType"`
	Backend     string    `json:"backend"`
	State       TaskState `json:"state"`
}

// TaskExecResult 是沙盒内受控入口的执行结果。
type TaskExecResult struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// TaskBackend 是任务运行时的最小适配面；实现不得提供宿主 Shell。
type TaskBackend interface {
	Create(context.Context, TaskSpec) (string, error)
	Start(context.Context, string) error
	Exec(context.Context, string, []string) (TaskExecResult, error)
	Cancel(context.Context, string) error
	Collect(context.Context, string) (TaskExecResult, error)
	Destroy(context.Context, string) error
}

// TaskProvider 编排任务生命周期，并保证同一 taskId 只创建一次。
type TaskProvider struct {
	mu      sync.Mutex
	backend TaskBackend
	tasks   map[string]*TaskHandle
}

// NewTaskProvider 创建任务 Provider。
func NewTaskProvider(backend TaskBackend) (*TaskProvider, error) {
	if backend == nil {
		return nil, errors.New("任务运行时 backend 不能为空")
	}
	return &TaskProvider{backend: backend, tasks: make(map[string]*TaskHandle)}, nil
}

// Restore 恢复进程重启前已知的沙盒句柄；后端负责确认句柄仍然有效。
// 仅恢复合法状态，未知状态按 created 处理，避免通过状态文件绕过状态机。
func (p *TaskProvider) Restore(handles []TaskHandle) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, handle := range handles {
		if !taskIDPattern.MatchString(handle.TaskID) || strings.TrimSpace(handle.SandboxID) == "" {
			continue
		}
		switch handle.State {
		case TaskCreated, TaskRunning, TaskCancelled, TaskCompleted, TaskFailed, TaskDestroyed:
		default:
			handle.State = TaskCreated
		}
		copy := handle
		p.tasks[handle.TaskID] = &copy
	}
}

var taskIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,119}$`)

// defaultPolicy 返回任务沙箱的默认资源上限，避免调用方获得无界执行能力。
func defaultPolicy() RuntimePolicy {
	return RuntimePolicy{SandboxType: "forgevm", Backend: "gvisor", IsolationRequired: "container", RiskClass: "untrusted", Environment: "development", AllowRemoteDispatch: true}
}

// normalized 将零值策略补齐为默认值，并裁剪到服务端允许的范围。
func (p RuntimePolicy) normalized() RuntimePolicy {
	if p.SandboxType == "" && p.Backend == "" && p.IsolationRequired == "" && p.RiskClass == "" && p.Environment == "" {
		return defaultPolicy()
	}
	return p
}

// validate 检查任务策略的时间、输出和并发限制，拒绝不安全配置。
func (p RuntimePolicy) validate() error {
	p = p.normalized()
	if p.SandboxType != "forgevm" {
		return errors.New("当前节点仅支持 forgevm 沙盒")
	}
	if p.IsolationRequired != "vm" && p.IsolationRequired != "container" && p.IsolationRequired != "none" {
		return errors.New("runtimePolicy isolationRequired 无效")
	}
	if p.RiskClass != "untrusted" && p.RiskClass != "trusted" && p.RiskClass != "read_only" {
		return errors.New("runtimePolicy riskClass 无效")
	}
	if p.Backend != "" && p.Backend != "firecracker" && p.Backend != "gvisor" && p.Backend != "docker" && p.Backend != "managed" {
		return errors.New("runtimePolicy backend 无效")
	}
	if p.Environment != "development" && p.Environment != "test" && p.Environment != "staging" && p.Environment != "production" {
		return errors.New("runtimePolicy environment 无效")
	}
	if p.Environment == "production" && strings.TrimSpace(p.ProductionReleaseApprovalID) == "" {
		return errors.New("生产环境必须提供 productionReleaseApprovalId")
	}
	if p.RiskClass == "untrusted" && p.IsolationRequired == "none" {
		return errors.New("不可信 Agent 必须声明容器或 VM 隔离")
	}
	if p.Backend == "docker" && p.RiskClass == "untrusted" {
		return errors.New("普通 Docker 不得执行不可信 Agent")
	}
	if p.Backend == "firecracker" && p.IsolationRequired != "vm" {
		return errors.New("Firecracker 任务必须声明 vm 隔离")
	}
	for _, host := range p.AllowedHosts {
		if strings.TrimSpace(host) == "" || strings.ContainsAny(host, "\r\n") {
			return errors.New("runtimePolicy allowedHosts 包含无效主机")
		}
	}
	return nil
}

// validateSpec 校验任务标识、工作目录、镜像摘要和入口参数白名单。
func validateSpec(spec TaskSpec) error {
	if strings.TrimSpace(spec.TaskID) != spec.TaskID || !taskIDPattern.MatchString(spec.TaskID) {
		return errors.New("taskId 格式无效")
	}
	if len(spec.ImageDigest) != len("sha256:")+64 || !strings.HasPrefix(spec.ImageDigest, "sha256:") {
		return errors.New("Agent image digest 必须是 sha256 加 64 位摘要")
	}
	for _, value := range spec.ImageDigest[len("sha256:"):] {
		if !((value >= '0' && value <= '9') || (value >= 'a' && value <= 'f')) {
			return errors.New("Agent image digest 不是有效的小写十六进制")
		}
	}
	if _, err := hex.DecodeString(spec.ImageDigest[len("sha256:"):]); err != nil {
		return errors.New("Agent image digest 不是有效的小写十六进制")
	}
	if err := validateWorktree(spec.Worktree); err != nil {
		return err
	}
	if len(spec.Entrypoint) == 0 || strings.TrimSpace(spec.Entrypoint[0]) == "" {
		return errors.New("Agent bootstrap entrypoint 不能为空")
	}
	entry := filepath.ToSlash(filepath.Clean(strings.TrimSpace(spec.Entrypoint[0])))
	if !strings.HasPrefix(entry, "/opt/workmesh/") || filepath.Base(entry) == "sh" || filepath.Base(entry) == "bash" {
		return errors.New("Agent bootstrap entrypoint 必须位于 /opt/workmesh")
	}
	for _, arg := range spec.Entrypoint {
		if strings.IndexByte(arg, 0) >= 0 || len(arg) > 16*1024 {
			return errors.New("entrypoint 含有非法或过长参数")
		}
	}
	return spec.RuntimePolicy.validate()
}

// validateWorktree 确保工作目录为绝对路径且不包含路径穿越片段。
func validateWorktree(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.IndexByte(value, 0) >= 0 || !isAbsolutePath(value) || filepath.ToSlash(filepath.Clean(value)) == "/" {
		return errors.New("worktree 必须是非根绝对路径")
	}
	clean := strings.ToLower(filepath.ToSlash(filepath.Clean(value)))
	for _, denied := range []string{"/etc", "/root", "/proc", "/sys", "/dev", "/var/run", "/var/lib/docker", "/www", "/home"} {
		if clean == denied || strings.HasPrefix(clean, denied+"/") {
			return fmt.Errorf("worktree 位于禁止访问的宿主目录: %s", denied)
		}
	}
	if root := strings.TrimSpace(os.Getenv("WORKMESH_AGENT_WORKSPACE_ROOT")); root != "" {
		if !isAbsolutePath(root) {
			return errors.New("WORKMESH_AGENT_WORKSPACE_ROOT 必须是绝对路径")
		}
		root = strings.TrimRight(filepath.ToSlash(filepath.Clean(root)), "/")
		if clean != root && !strings.HasPrefix(clean, root+"/") {
			return errors.New("worktree 超出 Agent workspace root")
		}
	}
	return nil
}

// isAbsolutePath 同时接受 Linux 节点路径和 Windows 本地测试路径。
func isAbsolutePath(value string) bool {
	return filepath.IsAbs(value) || strings.HasPrefix(filepath.ToSlash(value), "/") || (len(value) >= 3 && value[1] == ':' && (value[2] == '/' || value[2] == '\\'))
}

// validateArgv 限制任务入口参数数量和长度，防止命令注入及资源滥用。
func validateArgv(argv []string) error {
	if len(argv) == 0 || len(argv) > 128 || strings.TrimSpace(argv[0]) == "" {
		return errors.New("Agent argv 不能为空且不能超过 128 项")
	}
	for _, arg := range argv {
		if strings.IndexByte(arg, 0) >= 0 || len(arg) > 16*1024 {
			return errors.New("Agent argv 含有非法或过长参数")
		}
	}
	return nil
}

// Create 创建任务沙盒，但不启动 Agent。
// Create 创建并持久化一个待执行任务，初始状态固定为 pending。
func (p *TaskProvider) Create(ctx context.Context, spec TaskSpec) (TaskHandle, error) {
	if p == nil || p.backend == nil {
		return TaskHandle{}, errors.New("任务运行时 Provider 未配置")
	}
	if err := validateSpec(spec); err != nil {
		return TaskHandle{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.tasks[spec.TaskID]; exists {
		return TaskHandle{}, fmt.Errorf("taskId 已存在: %s", spec.TaskID)
	}
	sandboxID, err := p.backend.Create(ctx, spec)
	if err != nil {
		return TaskHandle{}, fmt.Errorf("创建任务沙盒失败: %w", err)
	}
	policy := spec.RuntimePolicy.normalized()
	h := &TaskHandle{TaskID: spec.TaskID, SandboxID: sandboxID, ImageDigest: spec.ImageDigest, SandboxType: policy.SandboxType, Backend: policy.Backend, State: TaskCreated}
	p.tasks[spec.TaskID] = h
	return *h, nil
}

// transition 按允许的状态边界执行后端操作，失败时保留可恢复的原状态。
func (p *TaskProvider) transition(ctx context.Context, taskID string, from, to TaskState, operation func(string) error) error {
	p.mu.Lock()
	h, ok := p.tasks[taskID]
	if !ok {
		p.mu.Unlock()
		return errors.New("任务不存在")
	}
	if h.State != from {
		p.mu.Unlock()
		return fmt.Errorf("任务状态 %s 不允许转换到 %s", h.State, to)
	}
	sandboxID := h.SandboxID
	p.mu.Unlock()
	if err := operation(sandboxID); err != nil {
		p.mu.Lock()
		h.State = TaskFailed
		p.mu.Unlock()
		return err
	}
	p.mu.Lock()
	h.State = to
	p.mu.Unlock()
	return nil
}

// Start 启动已创建的任务沙盒。
func (p *TaskProvider) Start(ctx context.Context, taskID string) error {
	return p.transition(ctx, taskID, TaskCreated, TaskRunning, func(id string) error { return p.backend.Start(ctx, id) })
}

// Exec 在运行中的沙盒内执行受控 argv。
func (p *TaskProvider) Exec(ctx context.Context, taskID string, argv []string) (TaskExecResult, error) {
	if err := validateArgv(argv); err != nil {
		return TaskExecResult{}, err
	}
	p.mu.Lock()
	h, ok := p.tasks[taskID]
	if ok {
		copy := *h
		h = &copy
	}
	p.mu.Unlock()
	if !ok {
		return TaskExecResult{}, errors.New("任务不存在")
	}
	if h.State != TaskRunning {
		return TaskExecResult{}, errors.New("任务沙盒未运行")
	}
	return p.backend.Exec(ctx, h.SandboxID, argv)
}

// Cancel 请求取消任务但保留沙盒句柄，便于收集诊断结果。
func (p *TaskProvider) Cancel(ctx context.Context, taskID string) error {
	return p.transition(ctx, taskID, TaskRunning, TaskCancelled, func(id string) error { return p.backend.Cancel(ctx, id) })
}

// Collect 收集任务执行结果。
func (p *TaskProvider) Collect(ctx context.Context, taskID string) (TaskExecResult, error) {
	p.mu.Lock()
	h, ok := p.tasks[taskID]
	if ok {
		copy := *h
		h = &copy
	}
	p.mu.Unlock()
	if !ok {
		return TaskExecResult{}, errors.New("任务不存在")
	}
	result, err := p.backend.Collect(ctx, h.SandboxID)
	if err == nil && h.State == TaskRunning {
		p.mu.Lock()
		if current := p.tasks[taskID]; current != nil && current.State == TaskRunning {
			current.State = TaskCompleted
		}
		p.mu.Unlock()
	}
	return result, err
}

// Destroy 销毁任务沙盒；销毁后句柄不可再次执行。
func (p *TaskProvider) Destroy(ctx context.Context, taskID string) error {
	p.mu.Lock()
	h, ok := p.tasks[taskID]
	if ok {
		copy := *h
		h = &copy
	}
	p.mu.Unlock()
	if !ok {
		return errors.New("任务不存在")
	}
	if err := p.backend.Destroy(ctx, h.SandboxID); err != nil {
		return fmt.Errorf("销毁任务沙盒失败: %w", err)
	}
	p.mu.Lock()
	if current := p.tasks[taskID]; current != nil {
		current.State = TaskDestroyed
	}
	p.mu.Unlock()
	return nil
}
