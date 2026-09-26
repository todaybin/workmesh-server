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
	TaskCreated       TaskState = "created"
	TaskRunning       TaskState = "running"
	TaskCancelled     TaskState = "cancelled"
	TaskCompleted     TaskState = "completed"
	TaskFailed        TaskState = "failed"
	TaskAwaitingHuman TaskState = "awaiting_human"
	TaskDestroyed     TaskState = "destroyed"
	TaskUnknown       TaskState = "unknown"
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
	TaskID          string         `json:"taskId"`
	ImageDigest     string         `json:"imageDigest"`
	Worktree        string         `json:"worktree"`
	Entrypoint      []string       `json:"entrypoint"`
	Timeout         time.Duration  `json:"timeout"`
	ResourceProfile string         `json:"resourceProfile"`
	ResourceLimits  ResourceLimits `json:"resourceLimits"`
	RuntimePolicy   RuntimePolicy  `json:"runtimePolicy"`
}

// ResourceLimits 是 Sandbox 必须执行的硬资源上限。
// CPUQuotaMicros 使用 cgroup v2 cpu.max 的 quota 单位，周期固定为 100000 微秒。
type ResourceLimits struct {
	CPUQuotaMicros int64 `json:"cpuQuotaMicros"`
	MemoryBytes    int64 `json:"memoryBytes"`
	PIDsMax        int64 `json:"pidsMax"`
	DiskBytes      int64 `json:"diskBytes"`
}

// SandboxCapabilities 描述外部 Sandbox 后端实际能够执行的隔离边界。
// 这些字段是准入证明，不是 Server 根据配置推断出的状态。
type SandboxCapabilities struct {
	ProtocolVersion        string         `json:"protocolVersion"`
	SandboxType            string         `json:"sandboxType"`
	Backend                string         `json:"backend"`
	Isolation              string         `json:"isolation"`
	WorkspaceIsolation     bool           `json:"workspaceIsolation"`
	NetworkIsolation       bool           `json:"networkIsolation"`
	NetworkAllowlist       bool           `json:"networkAllowlist"`
	PreStartEnforcement    bool           `json:"preStartEnforcement"`
	ProcessTreeContainment bool           `json:"processTreeContainment"`
	HardCPU                bool           `json:"hardCpu"`
	HardMemory             bool           `json:"hardMemory"`
	HardPIDs               bool           `json:"hardPids"`
	HardDisk               bool           `json:"hardDisk"`
	MaxResourceLimits      ResourceLimits `json:"maxResourceLimits"`
}

const SandboxProtocolVersion = "workmesh.sandbox.v1"

// SandboxEnforcementReceipt 是 CLI 对本次创建已应用约束的逐任务回执。
type SandboxEnforcementReceipt struct {
	ProtocolVersion          string         `json:"protocolVersion"`
	SandboxType              string         `json:"sandboxType"`
	Backend                  string         `json:"backend"`
	Isolation                string         `json:"isolation"`
	WorkspaceRef             string         `json:"workspaceRef"`
	ResourceLimits           ResourceLimits `json:"resourceLimits"`
	WorkspaceIsolation       bool           `json:"workspaceIsolation"`
	NetworkIsolation         bool           `json:"networkIsolation"`
	NetworkAllowlistEnforced bool           `json:"networkAllowlistEnforced"`
	AllowedHosts             []string       `json:"allowedHosts"`
	PreStartEnforcement      bool           `json:"preStartEnforcement"`
	ProcessTreeContainment   bool           `json:"processTreeContainment"`
	HardCPU                  bool           `json:"hardCpu"`
	HardMemory               bool           `json:"hardMemory"`
	HardPIDs                 bool           `json:"hardPids"`
	HardDisk                 bool           `json:"hardDisk"`
}

// ValidateSandboxEnforcementReceipt 拒绝缺字段、降级或与任务不匹配的执行回执。
func ValidateSandboxEnforcementReceipt(receipt SandboxEnforcementReceipt, spec TaskSpec) error {
	policy := spec.RuntimePolicy.normalized()
	if receipt.ProtocolVersion != SandboxProtocolVersion {
		return errors.New("Sandbox create 回执协议版本不受支持")
	}
	if receipt.SandboxType != policy.SandboxType || receipt.Backend != policy.Backend || receipt.Isolation != policy.IsolationRequired {
		return errors.New("Sandbox create 回执的隔离类型与任务策略不匹配")
	}
	if receipt.WorkspaceRef != policy.WorkspaceRef || receipt.ResourceLimits != spec.ResourceLimits {
		return errors.New("Sandbox create 回执的工作区或资源限制与任务请求不匹配")
	}
	if !receipt.WorkspaceIsolation || !receipt.PreStartEnforcement || !receipt.ProcessTreeContainment || !receipt.HardCPU || !receipt.HardMemory || !receipt.HardPIDs || !receipt.HardDisk {
		return errors.New("Sandbox create 回执未确认启动前资源限制、进程树约束及工作区隔离")
	}
	if (policy.RiskClass == "untrusted" || len(policy.AllowedHosts) > 0) && !receipt.NetworkIsolation {
		return errors.New("Sandbox create 回执未确认网络隔离")
	}
	if len(policy.AllowedHosts) > 0 && !receipt.NetworkAllowlistEnforced {
		return errors.New("Sandbox create 回执未确认网络 allowlist 已执行")
	}
	if !sameStrings(receipt.AllowedHosts, policy.AllowedHosts) {
		return errors.New("Sandbox create 回执的网络 allowlist 与任务策略不匹配")
	}
	return nil
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// SandboxCapabilityError 表示后端无法证明满足任务的硬隔离要求。
type SandboxCapabilityError struct {
	Err error
}

// Error 返回能力错误的上下文信息。
func (e *SandboxCapabilityError) Error() string {
	if e == nil || e.Err == nil {
		return "Sandbox 能力不可用"
	}
	return e.Err.Error()
}

// Unwrap 保留底层能力探测或校验错误，供 HTTP 层识别。
func (e *SandboxCapabilityError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

const (
	resourceProfileSmall  = "small"
	resourceProfileMedium = "medium"
	resourceProfileLarge  = "large"
)

// ResourceLimitsForProfile 返回受支持 profile 的默认硬限制。
func ResourceLimitsForProfile(profile string) (ResourceLimits, error) {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "", resourceProfileSmall:
		return ResourceLimits{CPUQuotaMicros: 100_000, MemoryBytes: 1 << 30, PIDsMax: 256, DiskBytes: 4 << 30}, nil
	case resourceProfileMedium:
		return ResourceLimits{CPUQuotaMicros: 200_000, MemoryBytes: 2 << 30, PIDsMax: 512, DiskBytes: 8 << 30}, nil
	case resourceProfileLarge:
		return ResourceLimits{CPUQuotaMicros: 400_000, MemoryBytes: 4 << 30, PIDsMax: 1024, DiskBytes: 16 << 30}, nil
	default:
		return ResourceLimits{}, fmt.Errorf("resourceProfile 只能是 small、medium 或 large")
	}
}

func normalizeResourceLimits(profile string, limits ResourceLimits) (string, ResourceLimits, error) {
	profile = strings.ToLower(strings.TrimSpace(profile))
	defaults, err := ResourceLimitsForProfile(profile)
	if err != nil {
		return "", ResourceLimits{}, err
	}
	if profile == "" {
		profile = resourceProfileSmall
	}
	if limits == (ResourceLimits{}) {
		return profile, defaults, nil
	}
	if limits.CPUQuotaMicros <= 0 || limits.MemoryBytes <= 0 || limits.PIDsMax <= 0 || limits.DiskBytes <= 0 {
		return "", ResourceLimits{}, errors.New("resourceLimits 必须全部为正数")
	}
	if limits.CPUQuotaMicros > defaults.CPUQuotaMicros || limits.MemoryBytes > defaults.MemoryBytes || limits.PIDsMax > defaults.PIDsMax || limits.DiskBytes > defaults.DiskBytes {
		return "", ResourceLimits{}, fmt.Errorf("resourceLimits 不能超过 %s profile 的默认上限", profile)
	}
	return profile, limits, nil
}

// TaskHandle 是任务沙盒的公开句柄。
type TaskHandle struct {
	TaskID       string    `json:"taskId"`
	SandboxID    string    `json:"sandboxId"`
	ImageDigest  string    `json:"imageDigest"`
	SandboxType  string    `json:"sandboxType"`
	Backend      string    `json:"backend"`
	WorkspaceRef string    `json:"workspaceRef,omitempty"`
	State        TaskState `json:"state"`
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

// TaskBackendCapabilities 是支持硬隔离准入证明的后端扩展接口。
type TaskBackendCapabilities interface {
	Capabilities(context.Context) (SandboxCapabilities, error)
}

// TaskBackendInspector 只读查询已存在沙盒的真实状态，不执行生命周期动作。
type TaskBackendInspector interface {
	Inspect(context.Context, string) (TaskInspection, error)
}

// TaskInspection 是后端对持久化沙盒句柄的只读核验结果。
type TaskInspection struct {
	Found bool      `json:"found"`
	State TaskState `json:"state,omitempty"`
}

// TaskProvider 编排任务生命周期，并保证同一 taskId 只创建一次。
type TaskProvider struct {
	mu                  sync.Mutex
	backend             TaskBackend
	tasks               map[string]*TaskHandle
	requireCapabilities bool
}

// NewTaskProvider 创建任务 Provider。
func NewTaskProvider(backend TaskBackend) (*TaskProvider, error) {
	if backend == nil {
		return nil, errors.New("任务运行时 backend 不能为空")
	}
	return &TaskProvider{backend: backend, tasks: make(map[string]*TaskHandle)}, nil
}

// NewCapabilityCheckedTaskProvider 创建要求后端提供硬隔离能力证明的 Provider。
// 生产任务必须使用该构造器；普通构造器只适合单元测试和非执行型适配器。
func NewCapabilityCheckedTaskProvider(backend TaskBackend) (*TaskProvider, error) {
	if backend == nil {
		return nil, errors.New("任务运行时 backend 不能为空")
	}
	if _, ok := backend.(TaskBackendCapabilities); !ok {
		return nil, errors.New("任务运行时 backend 未提供 Sandbox 能力证明")
	}
	return &TaskProvider{backend: backend, tasks: make(map[string]*TaskHandle), requireCapabilities: true}, nil
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
		case TaskCreated, TaskRunning, TaskCancelled, TaskCompleted, TaskFailed, TaskAwaitingHuman, TaskDestroyed, TaskUnknown:
		default:
			handle.State = TaskCreated
		}
		copy := handle
		p.tasks[handle.TaskID] = &copy
	}
}

// RestoreAndInspect 恢复持久化句柄并尝试从后端确认状态；缺少 inspect 时标记为 unknown。
func (p *TaskProvider) RestoreAndInspect(ctx context.Context, handles []TaskHandle) ([]TaskHandle, error) {
	if p == nil {
		return nil, errors.New("任务运行时 Provider 未配置")
	}
	inspector, supported := p.backend.(TaskBackendInspector)
	results := make([]TaskHandle, 0, len(handles))
	for _, handle := range handles {
		if !taskIDPattern.MatchString(handle.TaskID) || strings.TrimSpace(handle.SandboxID) == "" {
			continue
		}
		if handle.State != TaskDestroyed {
			handle.State = TaskUnknown
		}
		if supported && handle.State == TaskUnknown {
			inspection, err := inspector.Inspect(ctx, handle.SandboxID)
			if err == nil {
				if inspection.Found && validInspectedState(inspection.State) {
					handle.State = inspection.State
				} else {
					handle.State = TaskAwaitingHuman
				}
			}
		}
		p.mu.Lock()
		copy := handle
		p.tasks[handle.TaskID] = &copy
		p.mu.Unlock()
		results = append(results, handle)
	}
	return results, nil
}

// RecoverInspect 再次只读核验一个 unknown 任务，不执行 Sandbox 生命周期动作。
func (p *TaskProvider) RecoverInspect(ctx context.Context, taskID string) (TaskState, error) {
	if p == nil || p.backend == nil {
		return TaskUnknown, errors.New("任务运行时 Provider 未配置")
	}
	inspector, ok := p.backend.(TaskBackendInspector)
	if !ok {
		return TaskUnknown, errors.New("任务后端不支持只读 inspect")
	}
	p.mu.Lock()
	handle := p.tasks[taskID]
	if handle == nil {
		p.mu.Unlock()
		return TaskUnknown, errors.New("任务不存在")
	}
	sandboxID := handle.SandboxID
	if handle.State == TaskDestroyed {
		p.mu.Unlock()
		return handle.State, errors.New("已销毁任务不需要恢复核验")
	}
	p.mu.Unlock()
	inspection, err := inspector.Inspect(ctx, sandboxID)
	if err != nil {
		return TaskUnknown, err
	}
	state := TaskAwaitingHuman
	if inspection.Found && validInspectedState(inspection.State) {
		state = inspection.State
	}
	p.mu.Lock()
	if current := p.tasks[taskID]; current != nil && current.SandboxID == sandboxID {
		current.State = state
	}
	p.mu.Unlock()
	return state, nil
}

func validInspectedState(state TaskState) bool {
	switch state {
	case TaskCreated, TaskRunning, TaskCancelled, TaskCompleted, TaskFailed, TaskAwaitingHuman, TaskDestroyed:
		return true
	default:
		return false
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
		defaults := defaultPolicy()
		p.SandboxType = defaults.SandboxType
		p.Backend = defaults.Backend
		p.IsolationRequired = defaults.IsolationRequired
		p.RiskClass = defaults.RiskClass
		p.Environment = defaults.Environment
		p.AllowRemoteDispatch = defaults.AllowRemoteDispatch
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

// ValidateSandboxCapabilities 检查后端能力是否覆盖任务声明的硬隔离要求。
func ValidateSandboxCapabilities(caps SandboxCapabilities, spec TaskSpec) error {
	if err := validateCapabilitySet(caps); err != nil {
		return err
	}
	policy := spec.RuntimePolicy.normalized()
	if strings.TrimSpace(caps.SandboxType) != policy.SandboxType {
		return fmt.Errorf("Sandbox 类型不匹配: 需要 %s，实际 %s", policy.SandboxType, caps.SandboxType)
	}
	if strings.TrimSpace(caps.Backend) != policy.Backend {
		return fmt.Errorf("Sandbox backend 不匹配: 需要 %s，实际 %s", policy.Backend, caps.Backend)
	}
	if policy.IsolationRequired != "none" && caps.Isolation != policy.IsolationRequired {
		return fmt.Errorf("Sandbox 隔离级别不匹配: 需要 %s，实际 %s", policy.IsolationRequired, caps.Isolation)
	}
	if (policy.RiskClass == "untrusted" || len(policy.AllowedHosts) > 0) && !caps.NetworkIsolation {
		return errors.New("Sandbox 未提供网络隔离能力")
	}
	if len(policy.AllowedHosts) > 0 && !caps.NetworkAllowlist {
		return errors.New("Sandbox 未提供网络 allowlist 能力")
	}
	max := caps.MaxResourceLimits
	limits := spec.ResourceLimits
	if limits.CPUQuotaMicros > max.CPUQuotaMicros || limits.MemoryBytes > max.MemoryBytes || limits.PIDsMax > max.PIDsMax || limits.DiskBytes > max.DiskBytes {
		return errors.New("任务资源限制超过 Sandbox 能力上限")
	}
	return nil
}

func validateCapabilitySet(caps SandboxCapabilities) error {
	if caps.ProtocolVersion != SandboxProtocolVersion {
		return errors.New("Sandbox CLI 未实现受支持的 WorkMesh Sandbox 协议版本")
	}
	if !caps.WorkspaceIsolation {
		return errors.New("Sandbox 未提供工作区隔离能力")
	}
	if !caps.HardCPU || !caps.HardMemory || !caps.HardPIDs || !caps.HardDisk {
		return errors.New("Sandbox 未提供完整 CPU、内存、PID 和磁盘硬限制")
	}
	if !caps.PreStartEnforcement || !caps.ProcessTreeContainment {
		return errors.New("Sandbox 未声明启动前限制和进程树约束")
	}
	max := caps.MaxResourceLimits
	if max.CPUQuotaMicros <= 0 || max.MemoryBytes <= 0 || max.PIDsMax <= 0 || max.DiskBytes <= 0 {
		return errors.New("Sandbox 未声明有效的资源上限")
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
	if _, _, err := normalizeResourceLimits(spec.ResourceProfile, spec.ResourceLimits); err != nil {
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
	root := strings.TrimSpace(os.Getenv("WORKMESH_AGENT_WORKSPACE_ROOT"))
	if root == "" {
		return errors.New("未配置 WORKMESH_AGENT_WORKSPACE_ROOT，拒绝使用未隔离工作区")
	}
	if !isAbsolutePath(root) {
		return errors.New("WORKMESH_AGENT_WORKSPACE_ROOT 必须是绝对路径")
	}
	rootPath := filepath.Clean(root)
	rootSlash := strings.TrimRight(filepath.ToSlash(rootPath), "/")
	rootCompare := strings.ToLower(rootSlash)
	if clean == rootCompare || !strings.HasPrefix(clean, rootCompare+"/") {
		return errors.New("worktree 必须位于 Agent workspace root 的任务子目录")
	}
	if err := rejectSymlinkPath(rootSlash, filepath.ToSlash(filepath.Clean(value))); err != nil {
		return err
	}
	return nil
}

// rejectSymlinkPath 校验已存在的路径组件，防止工作区通过符号链接逃逸。
func rejectSymlinkPath(root, target string) error {
	rootInfo, err := os.Lstat(filepath.FromSlash(root))
	if err != nil {
		return fmt.Errorf("Agent workspace root 不可用: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("Agent workspace root 必须是非符号链接目录")
	}
	rel, err := filepath.Rel(filepath.FromSlash(root), filepath.FromSlash(target))
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return errors.New("worktree 路径解析失败")
	}
	current := filepath.FromSlash(root)
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			break
		}
		if statErr != nil {
			return fmt.Errorf("检查 worktree 路径失败: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("worktree 路径不能包含符号链接")
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
	profile, limits, err := normalizeResourceLimits(spec.ResourceProfile, spec.ResourceLimits)
	if err != nil {
		return TaskHandle{}, err
	}
	spec.ResourceProfile, spec.ResourceLimits = profile, limits
	if p.requireCapabilities {
		capabilityBackend, ok := p.backend.(TaskBackendCapabilities)
		if !ok {
			return TaskHandle{}, &SandboxCapabilityError{Err: errors.New("任务运行时 backend 未提供 Sandbox 能力证明")}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		caps, capabilityErr := capabilityBackend.Capabilities(ctx)
		if capabilityErr != nil {
			return TaskHandle{}, &SandboxCapabilityError{Err: fmt.Errorf("Sandbox 能力探测失败: %w", capabilityErr)}
		}
		if err := ValidateSandboxCapabilities(caps, spec); err != nil {
			return TaskHandle{}, &SandboxCapabilityError{Err: fmt.Errorf("Sandbox 能力不足: %w", err)}
		}
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
	h := &TaskHandle{TaskID: spec.TaskID, SandboxID: sandboxID, ImageDigest: spec.ImageDigest, SandboxType: policy.SandboxType, Backend: policy.Backend, WorkspaceRef: policy.WorkspaceRef, State: TaskCreated}
	p.tasks[spec.TaskID] = h
	return *h, nil
}

// transition 按允许的状态边界执行后端操作；后端失败时保留原状态，允许调用方重试。
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

// State 返回任务当前状态，供 API 在重启和收集结果后持久化真实状态。
func (p *TaskProvider) State(taskID string) (TaskState, error) {
	if p == nil {
		return "", errors.New("任务运行时 Provider 未配置")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	h, ok := p.tasks[taskID]
	if !ok {
		return "", errors.New("任务不存在")
	}
	return h.State, nil
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
	if h.State == TaskRunning {
		return errors.New("运行中的任务必须先取消或收集结果后才能销毁")
	}
	if h.State == TaskDestroyed {
		return errors.New("任务已销毁")
	}
	if h.State == TaskUnknown {
		return errors.New("任务后端状态未知，必须先通过 inspect 核验")
	}
	// workspace 清理失败时后端 Sandbox 已经销毁，人工修复目录后再次调用
	// destroy 只需重试清理，不能重复调用后端销毁操作。
	if h.State != TaskAwaitingHuman {
		if err := p.backend.Destroy(ctx, h.SandboxID); err != nil {
			return fmt.Errorf("销毁任务沙盒失败: %w", err)
		}
	}
	if h.WorkspaceRef != "" {
		layout, layoutErr := NewWorkspaceLayout(os.Getenv("WORKMESH_AGENT_WORKSPACE_ROOT"), h.WorkspaceRef, h.TaskID)
		if layoutErr != nil {
			p.markTaskAwaitingHuman(taskID)
			return &WorkspaceCleanupError{Err: fmt.Errorf("任务 workspace 清理前置校验失败: %w", layoutErr)}
		}
		if cleanupErr := layout.CleanupTransient(ctx); cleanupErr != nil {
			p.markTaskAwaitingHuman(taskID)
			return &WorkspaceCleanupError{Err: cleanupErr}
		}
	}
	p.mu.Lock()
	if current := p.tasks[taskID]; current != nil {
		current.State = TaskDestroyed
	}
	p.mu.Unlock()
	return nil
}

func (p *TaskProvider) markTaskFailed(taskID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if current := p.tasks[taskID]; current != nil {
		current.State = TaskFailed
	}
}

func (p *TaskProvider) markTaskAwaitingHuman(taskID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if current := p.tasks[taskID]; current != nil {
		current.State = TaskAwaitingHuman
	}
}
