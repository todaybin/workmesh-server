// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	firecrackerGuestFrameLimit = 1 << 20
	firecrackerGuestArgLimit   = 128
	firecrackerGuestArgBytes   = 4096
)

// FirecrackerGuestRequest 是 Server 与 guest agent 之间的最小受控消息。
// 请求通过长度前缀帧传输，guest agent 不接收宿主 Shell 字符串。
type FirecrackerGuestRequest struct {
	RequestID string   `json:"requestId"`
	Operation string   `json:"operation"`
	Argv      []string `json:"argv,omitempty"`
}

// FirecrackerGuestResponse 是 guest agent 返回的有界结果摘要。
type FirecrackerGuestResponse struct {
	RequestID string `json:"requestId"`
	ExitCode  int    `json:"exitCode"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Error     string `json:"error,omitempty"`
}

// EncodeFirecrackerGuestRequest 将请求编码为 4 字节大端长度前缀帧。
func EncodeFirecrackerGuestRequest(request FirecrackerGuestRequest) ([]byte, error) {
	if err := validateFirecrackerGuestRequest(request); err != nil {
		return nil, err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("编码 guest 请求失败: %w", err)
	}
	if len(body) > firecrackerGuestFrameLimit {
		return nil, errors.New("guest 请求超过 1 MiB 限制")
	}
	frame := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(body)))
	copy(frame[4:], body)
	return frame, nil
}

// DecodeFirecrackerGuestFrame 解码单个完整帧，并拒绝尾随数据或超限输入。
func DecodeFirecrackerGuestFrame(frame []byte, response *FirecrackerGuestResponse) error {
	if response == nil {
		return errors.New("guest 响应目标不能为空")
	}
	if len(frame) < 4 {
		return errors.New("guest 帧长度前缀缺失")
	}
	bodyLen := int(binary.BigEndian.Uint32(frame[:4]))
	if bodyLen <= 0 || bodyLen > firecrackerGuestFrameLimit {
		return errors.New("guest 帧长度无效")
	}
	if len(frame) != bodyLen+4 {
		return errors.New("guest 帧存在截断或尾随数据")
	}
	decoder := json.NewDecoder(bytes.NewReader(frame[4:]))
	if err := decoder.Decode(response); err != nil {
		return fmt.Errorf("解码 guest 响应失败: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("guest 响应包含尾随 JSON")
	}
	if strings.TrimSpace(response.RequestID) == "" || len(response.RequestID) > 128 {
		return errors.New("guest response requestId 无效")
	}
	if len(response.Stdout) > firecrackerGuestFrameLimit || len(response.Stderr) > firecrackerGuestFrameLimit || len(response.Error) > 8192 {
		return errors.New("guest 响应输出超过限制")
	}
	return nil
}

func validateFirecrackerGuestRequest(request FirecrackerGuestRequest) error {
	if strings.TrimSpace(request.RequestID) == "" || len(request.RequestID) > 128 {
		return errors.New("guest requestId 无效")
	}
	switch request.Operation {
	case "exec", "collect", "cancel", "inspect":
	default:
		return errors.New("guest operation 不在白名单中")
	}
	if len(request.Argv) > firecrackerGuestArgLimit {
		return errors.New("guest argv 数量超过限制")
	}
	for _, arg := range request.Argv {
		if strings.ContainsAny(arg, "\x00\r\n") || len(arg) > firecrackerGuestArgBytes {
			return errors.New("guest argv 含无效字符或超过长度限制")
		}
	}
	return nil
}

// FirecrackerRuntimeConfig 描述 Firecracker 运行器启动所需的固定资产。
// 该配置只负责准入和启动计划，不包含宿主 Shell 或用户可控参数。
type FirecrackerRuntimeConfig struct {
	BinaryPath    string
	KernelPath    string
	RootFSPath    string
	KVMPath       string
	WorkspaceRoot string
	CgroupRoot    string
	VMMemoryBytes int64
	VCPUCount     int
	DiskBytes     int64
	NetworkDevice string
	VsockDevice   string
}

// FirecrackerBootPlan 是经白名单校验后的 VMM 启动计划。
// Args 可直接传给 exec.Command，不得经过 Shell 拼接或再次追加用户输入。
type FirecrackerBootPlan struct {
	BinaryPath string
	Args       []string
}

// FirecrackerMachineConfig 是 Firecracker 官方 machine-config 的受控子集。
// workspace image 必须由更高层在受控目录中预先创建；本函数不会创建镜像或宣称磁盘 quota 已生效。
type FirecrackerMachineConfig struct {
	BootSource        FirecrackerBootSource         `json:"boot-source"`
	Drives            []FirecrackerDrive            `json:"drives"`
	MachineConfig     FirecrackerMachineParameters  `json:"machine-config"`
	NetworkInterfaces []FirecrackerNetworkInterface `json:"network-interfaces,omitempty"`
	Vsock             *FirecrackerVsock             `json:"vsock,omitempty"`
}

// FirecrackerBootSource 指定固定 kernel 和 guest 启动参数。
type FirecrackerBootSource struct {
	KernelImagePath string `json:"kernel_image_path"`
	BootArgs        string `json:"boot_args"`
}

// FirecrackerDrive 描述 guest 可见的磁盘镜像。
type FirecrackerDrive struct {
	DriveID      string `json:"drive_id"`
	PathOnHost   string `json:"path_on_host"`
	IsRootDevice bool   `json:"is_root_device"`
	IsReadOnly   bool   `json:"is_read_only"`
}

// FirecrackerMachineParameters 限制 VMM 的 CPU 和内存。
type FirecrackerMachineParameters struct {
	VCPUCount  int  `json:"vcpu_count"`
	MemSizeMiB int  `json:"mem_size_mib"`
	HTEnabled  bool `json:"ht_enabled"`
}

// FirecrackerNetworkInterface 描述预先创建的宿主网络设备。
type FirecrackerNetworkInterface struct {
	IfaceID     string `json:"iface_id"`
	HostDevName string `json:"host_dev_name"`
	GuestMAC    string `json:"guest_mac"`
}

// FirecrackerVsock 描述 guest agent 使用的 host UDS。
type FirecrackerVsock struct {
	GuestCID uint32 `json:"guest_cid"`
	UDSPath  string `json:"uds_path"`
}

var firecrackerDevicePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

// FirecrackerUnavailableError 表示当前节点不能安全执行 Firecracker 任务。
type FirecrackerUnavailableError struct {
	Reason string
}

func (e *FirecrackerUnavailableError) Error() string {
	if e == nil || strings.TrimSpace(e.Reason) == "" {
		return "Firecracker 运行器不可用"
	}
	return "Firecracker 运行器不可用: " + e.Reason
}

// ValidateFirecrackerConfig 校验 Firecracker 的固定资产和资源边界。
// 任何缺失资产都会失败关闭，不会退回宿主进程或普通 Docker。
func ValidateFirecrackerConfig(config FirecrackerRuntimeConfig, fileExists func(string) (bool, error), kvmExists func(string) (bool, error)) error {
	if fileExists == nil || kvmExists == nil {
		return errors.New("Firecracker 能力探测器未配置")
	}
	for name, value := range map[string]string{
		"binaryPath":    config.BinaryPath,
		"kernelPath":    config.KernelPath,
		"rootFSPath":    config.RootFSPath,
		"kvmPath":       config.KVMPath,
		"workspaceRoot": config.WorkspaceRoot,
		"cgroupRoot":    config.CgroupRoot,
	} {
		if err := validateAbsoluteRuntimePath(name, value); err != nil {
			return err
		}
	}
	if config.VCPUCount < 1 || config.VCPUCount > 16 {
		return errors.New("Firecracker vCPU 数量必须在 1 到 16 之间")
	}
	if config.VMMemoryBytes < 128<<20 || config.VMMemoryBytes > 64<<30 {
		return errors.New("Firecracker 内存必须在 128 MiB 到 64 GiB 之间")
	}
	if config.DiskBytes < 256<<20 || config.DiskBytes > 256<<30 {
		return errors.New("Firecracker 磁盘必须在 256 MiB 到 256 GiB 之间")
	}
	for name, path := range map[string]string{
		"binaryPath": config.BinaryPath,
		"kernelPath": config.KernelPath,
		"rootFSPath": config.RootFSPath,
	} {
		ok, err := fileExists(path)
		if err != nil {
			return fmt.Errorf("检查 Firecracker %s 失败: %w", name, err)
		}
		if !ok {
			return fmt.Errorf("Firecracker %s 不存在", name)
		}
	}
	ok, err := kvmExists(config.KVMPath)
	if err != nil {
		return fmt.Errorf("检查 KVM 失败: %w", err)
	}
	if !ok {
		return errors.New("节点缺少 /dev/kvm，禁止启用 Firecracker")
	}
	return nil
}

func validateAbsoluteRuntimePath(name, value string) error {
	value = strings.TrimSpace(value)
	if value == "" || !filepath.IsAbs(value) || strings.ContainsAny(value, "\x00\r\n;&|`$<>\t ") {
		return fmt.Errorf("Firecracker %s 必须是无参数绝对路径", name)
	}
	return nil
}

// BuildFirecrackerBootPlan 生成固定的 VMM 启动参数。
// 网络和 vsock 设备必须由受控运行器预先创建，不能从任务请求直接传入。
func BuildFirecrackerBootPlan(config FirecrackerRuntimeConfig, socketPath, machineConfigPath string) (FirecrackerBootPlan, error) {
	if err := validateAbsoluteRuntimePath("socketPath", socketPath); err != nil {
		return FirecrackerBootPlan{}, err
	}
	if err := validateAbsoluteRuntimePath("machineConfigPath", machineConfigPath); err != nil {
		return FirecrackerBootPlan{}, err
	}
	if err := validateAbsoluteRuntimePath("binaryPath", config.BinaryPath); err != nil {
		return FirecrackerBootPlan{}, err
	}
	if err := validateAbsoluteRuntimePath("kernelPath", config.KernelPath); err != nil {
		return FirecrackerBootPlan{}, err
	}
	if err := validateAbsoluteRuntimePath("rootFSPath", config.RootFSPath); err != nil {
		return FirecrackerBootPlan{}, err
	}
	if config.VCPUCount < 1 || config.VCPUCount > 16 || config.VMMemoryBytes < 128<<20 || config.DiskBytes < 256<<20 {
		return FirecrackerBootPlan{}, errors.New("Firecracker 启动资源参数无效")
	}
	return FirecrackerBootPlan{BinaryPath: config.BinaryPath, Args: []string{
		"--api-sock", socketPath,
		"--config-file", machineConfigPath,
	}}, nil
}

// BuildFirecrackerMachineConfig 生成单任务 machine-config JSON。
// 调用方必须先完成 workspace image 创建、磁盘配额和网络隔离；这里仅生成白名单配置。
func BuildFirecrackerMachineConfig(config FirecrackerRuntimeConfig, spec TaskSpec, workspaceImagePath, vsockSocketPath string, guestCID uint32) ([]byte, error) {
	if err := validateAbsoluteRuntimePath("kernelPath", config.KernelPath); err != nil {
		return nil, err
	}
	if err := validateAbsoluteRuntimePath("rootFSPath", config.RootFSPath); err != nil {
		return nil, err
	}
	if err := validateAbsoluteRuntimePath("workspaceImagePath", workspaceImagePath); err != nil {
		return nil, err
	}
	if err := validateContainedRuntimePath(config.WorkspaceRoot, workspaceImagePath); err != nil {
		return nil, fmt.Errorf("workspace image 不在受控根目录内: %w", err)
	}
	if err := validateAbsoluteRuntimePath("vsockSocketPath", vsockSocketPath); err != nil {
		return nil, err
	}
	if guestCID < 3 {
		return nil, errors.New("Firecracker guestCID 必须大于等于 3")
	}
	if config.VCPUCount < 1 || config.VCPUCount > 16 || config.VMMemoryBytes < 128<<20 || config.VMMemoryBytes%(1<<20) != 0 {
		return nil, errors.New("Firecracker machine-config CPU 或内存参数无效")
	}
	if strings.TrimSpace(spec.TaskID) == "" {
		return nil, errors.New("Firecracker machine-config 缺少 taskId")
	}
	if config.NetworkDevice != "" && !firecrackerDevicePattern.MatchString(config.NetworkDevice) {
		return nil, errors.New("Firecracker network device 名称无效")
	}
	if config.VsockDevice != "" && !firecrackerDevicePattern.MatchString(config.VsockDevice) {
		return nil, errors.New("Firecracker vsock device 名称无效")
	}
	result := FirecrackerMachineConfig{
		BootSource: FirecrackerBootSource{KernelImagePath: config.KernelPath, BootArgs: "console=ttyS0 reboot=k panic=1 pci=off"},
		Drives: []FirecrackerDrive{
			{DriveID: "rootfs", PathOnHost: config.RootFSPath, IsRootDevice: true, IsReadOnly: true},
			{DriveID: "workspace", PathOnHost: workspaceImagePath, IsRootDevice: false, IsReadOnly: false},
		},
		MachineConfig: FirecrackerMachineParameters{VCPUCount: config.VCPUCount, MemSizeMiB: int(config.VMMemoryBytes / (1 << 20)), HTEnabled: false},
		Vsock:         &FirecrackerVsock{GuestCID: guestCID, UDSPath: vsockSocketPath},
	}
	if config.NetworkDevice != "" {
		result.NetworkInterfaces = []FirecrackerNetworkInterface{{IfaceID: "eth0", HostDevName: config.NetworkDevice, GuestMAC: "02:FC:00:00:00:01"}}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("编码 Firecracker machine-config 失败: %w", err)
	}
	return encoded, nil
}

func validateContainedRuntimePath(root, candidate string) error {
	if err := validateAbsoluteRuntimePath("workspaceRoot", root); err != nil {
		return err
	}
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("路径越过 workspace root")
	}
	return nil
}

// FirecrackerTaskBackend 是真实 Firecracker guest 执行器接入前的失败关闭边界。
// 未注入 guest/vsock 执行实现时，所有生命周期动作都返回不可用错误。
type FirecrackerTaskBackend struct {
	config FirecrackerRuntimeConfig
}

// NewFirecrackerTaskBackend 只在节点具备完整 Firecracker 基础资产时构造 Backend。
func NewFirecrackerTaskBackend(config FirecrackerRuntimeConfig) (*FirecrackerTaskBackend, error) {
	if err := ValidateFirecrackerConfig(config, regularFileExists, pathExists); err != nil {
		return nil, &FirecrackerUnavailableError{Reason: err.Error()}
	}
	return &FirecrackerTaskBackend{config: config}, nil
}

// Capabilities 不声明 guest 执行能力，避免把配置存在误报为隔离已生效。
func (b *FirecrackerTaskBackend) Capabilities(context.Context) (SandboxCapabilities, error) {
	return SandboxCapabilities{}, &FirecrackerUnavailableError{Reason: "guest agent、vsock 执行代理和网络/磁盘隔离尚未接入"}
}

func (b *FirecrackerTaskBackend) unavailable() error {
	return &FirecrackerUnavailableError{Reason: "guest agent、vsock 执行代理和生命周期回收尚未接入"}
}

// Create 在真实 guest 执行器接入前失败关闭。
func (b *FirecrackerTaskBackend) Create(context.Context, TaskSpec) (string, error) {
	return "", b.unavailable()
}

// Start 在真实 guest 执行器接入前失败关闭。
func (b *FirecrackerTaskBackend) Start(context.Context, string) error { return b.unavailable() }

// Exec 在真实 guest 执行器接入前失败关闭。
func (b *FirecrackerTaskBackend) Exec(context.Context, string, []string) (TaskExecResult, error) {
	return TaskExecResult{}, b.unavailable()
}

// Cancel 在真实 guest 执行器接入前失败关闭。
func (b *FirecrackerTaskBackend) Cancel(context.Context, string) error { return b.unavailable() }

// Collect 在真实 guest 执行器接入前失败关闭。
func (b *FirecrackerTaskBackend) Collect(context.Context, string) (TaskExecResult, error) {
	return TaskExecResult{}, b.unavailable()
}

// Destroy 在真实 guest 执行器接入前失败关闭。
func (b *FirecrackerTaskBackend) Destroy(context.Context, string) error { return b.unavailable() }

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
