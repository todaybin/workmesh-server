// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package taskruntime 提供任务级隔离运行时的受控生命周期适配。
package taskruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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

func defaultPolicy() RuntimePolicy {
	return RuntimePolicy{SandboxType: "forgevm", Backend: "gvisor", IsolationRequired: "container", RiskClass: "untrusted", Environment: "development", AllowRemoteDispatch: true}
}

func (p RuntimePolicy) normalized() RuntimePolicy {
	if p.SandboxType == "" && p.Backend == "" && p.IsolationRequired == "" && p.RiskClass == "" && p.Environment == "" {
		return defaultPolicy()
	}
	return p
}

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

// CLITaskBackend 通过固定 CLI 管理沙盒，不经过 Shell。
type CLITaskBackend struct {
	command     string
	digest      string
	timeout     time.Duration
	outputLimit int
	run         func(context.Context, string, []string, int) ([]byte, error)
}

// NewCLITaskBackend 创建 CLI 后端，并校验可执行文件的固定摘要。
func NewCLITaskBackend(command, digest string, timeout time.Duration, outputLimit int) (*CLITaskBackend, error) {
	command = strings.TrimSpace(command)
	if command == "" || !isAbsolutePath(command) || strings.ContainsAny(command, "\x00 \t\r\n;&|`$<>") {
		return nil, errors.New("任务 CLI 必须是无参数绝对路径")
	}
	if digest == "" {
		return nil, errors.New("任务 CLI 必须配置 sha256 摘要")
	}
	if err := verifyFileDigest(command, digest); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	if outputLimit <= 0 {
		outputLimit = 8 << 20
	}
	if outputLimit > 64<<20 {
		return nil, errors.New("任务 CLI 输出上限不能超过 64 MiB")
	}
	return &CLITaskBackend{command: command, digest: digest, timeout: timeout, outputLimit: outputLimit, run: runCLI}, nil
}

func verifyFileDigest(path, expected string) error {
	expected = strings.TrimSpace(strings.TrimPrefix(expected, "sha256:"))
	if len(expected) != sha256.Size*2 {
		return errors.New("任务 CLI 摘要必须是 64 位十六进制")
	}
	if _, err := hex.DecodeString(expected); err != nil {
		return errors.New("任务 CLI 摘要格式无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开任务 CLI 失败: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, 128<<20)); err != nil {
		return fmt.Errorf("读取任务 CLI 失败: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("任务 CLI 摘要不匹配: expected=%s actual=%s", expected, actual)
	}
	return nil
}

func (b *CLITaskBackend) execute(ctx context.Context, operation string, payload any, result any) error {
	switch operation {
	case "create", "start", "exec", "collect", "cancel", "destroy":
	default:
		return errors.New("任务 CLI 操作不在白名单中")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("编码任务 CLI 请求失败: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	callCtx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	out, err := b.run(callCtx, b.command, []string{"--json", "task", operation, string(body)}, b.outputLimit)
	if err != nil {
		return fmt.Errorf("任务 CLI %s 失败: %w", operation, err)
	}
	var envelope struct {
		OK      *bool           `json:"ok"`
		Error   string          `json:"error"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil || envelope.OK == nil {
		return errors.New("任务 CLI 返回了无效结果")
	}
	if !*envelope.OK {
		message := strings.TrimSpace(envelope.Error)
		if message == "" {
			message = strings.TrimSpace(envelope.Message)
		}
		if message == "" {
			message = "任务 CLI 操作失败"
		}
		return errors.New(sanitizeCLIMessage(message))
	}
	if result != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, result); err != nil {
			return errors.New("任务 CLI data 无效")
		}
	}
	return nil
}

func (b *CLITaskBackend) Create(ctx context.Context, spec TaskSpec) (string, error) {
	var response struct {
		SandboxID string `json:"sandboxId"`
	}
	if err := b.execute(ctx, "create", spec, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.SandboxID) == "" {
		return "", errors.New("任务 CLI create 未返回 sandboxId")
	}
	return response.SandboxID, nil
}
func (b *CLITaskBackend) Start(ctx context.Context, id string) error {
	return b.execute(ctx, "start", map[string]string{"sandboxId": id}, nil)
}
func (b *CLITaskBackend) Exec(ctx context.Context, id string, argv []string) (TaskExecResult, error) {
	if err := validateArgv(argv); err != nil {
		return TaskExecResult{}, err
	}
	var result TaskExecResult
	err := b.execute(ctx, "exec", map[string]any{"sandboxId": id, "argv": argv}, &result)
	return result, err
}
func (b *CLITaskBackend) Cancel(ctx context.Context, id string) error {
	return b.execute(ctx, "cancel", map[string]string{"sandboxId": id}, nil)
}
func (b *CLITaskBackend) Collect(ctx context.Context, id string) (TaskExecResult, error) {
	var result TaskExecResult
	err := b.execute(ctx, "collect", map[string]string{"sandboxId": id}, &result)
	return result, err
}
func (b *CLITaskBackend) Destroy(ctx context.Context, id string) error {
	return b.execute(ctx, "destroy", map[string]string{"sandboxId": id}, nil)
}

func runCLI(ctx context.Context, command string, args []string, outputLimit int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = restrictedEnvironment(os.Environ())
	var stdout, stderr limitedOutput
	stdout.limit, stderr.limit = outputLimit, outputLimit
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("任务 CLI 操作超时")
		}
		if strings.TrimSpace(stderr.String()) != "" {
			return nil, errors.New(sanitizeCLIMessage(stderr.String()))
		}
		return nil, errors.New(sanitizeCLIMessage(err.Error()))
	}
	if stdout.exceeded || stderr.exceeded {
		return nil, errors.New("任务 CLI 输出超过限制")
	}
	return stdout.Bytes(), nil
}

type limitedOutput struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *limitedOutput) Write(data []byte) (int, error) {
	if b.limit > 0 && b.Len()+len(data) > b.limit {
		b.exceeded = true
		return len(data), io.ErrShortBuffer
	}
	return b.Buffer.Write(data)
}

func restrictedEnvironment(source []string) []string {
	allowed := map[string]bool{"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "LANG": true, "TZ": true, "TMP": true, "TEMP": true, "TMPDIR": true, "SystemRoot": true, "WINDIR": true}
	result := make([]string, 0, len(source))
	for _, entry := range source {
		key, _, ok := strings.Cut(entry, "=")
		if ok && (allowed[key] || strings.HasPrefix(key, "LC_")) {
			result = append(result, entry)
		}
	}
	return result
}

var sensitivePattern = regexp.MustCompile(`(?i)(authorization|token|secret|password|private[_ -]?key|api[_ -]?key)\s*[:=]\s*[^\s,;]+`)

func sanitizeCLIMessage(message string) string {
	message = sensitivePattern.ReplaceAllString(message, "$1=[redacted]")
	message = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || (r < 0x20 && r != ' ') {
			return ' '
		}
		return r
	}, message)
	message = strings.TrimSpace(message)
	if len(message) > 512 {
		message = message[:512]
	}
	if message == "" {
		return "任务 CLI 操作失败"
	}
	return message
}
