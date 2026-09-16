// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// hostOperationalStatePath 返回主机监控兼容状态文件路径。
func hostOperationalStatePath() string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	return filepath.Join(root, "host-operational.json")
}

// hostOperationalState 保存主机监控采样配置；仅保存用户设置，不缓存无限增长的采样数据。
type hostOperationalState struct {
	Monitor map[string]any `json:"monitor"`
}

var hostOperationalMu sync.Mutex

func loadHostOperationalState() hostOperationalState {
	hostOperationalMu.Lock()
	defer hostOperationalMu.Unlock()
	return loadHostOperationalStateLocked()
}

func loadHostOperationalStateLocked() hostOperationalState {
	state := hostOperationalState{Monitor: map[string]any{"enabled": true, "interval": 10}}
	// SQLite 已初始化时，主机监控设置只从 node_settings 读取；旧 JSON 仅在首次启动时导入并归档。
	if sharedDB() != nil {
		if loadNodeSetting("host_operational", &state) {
			if state.Monitor == nil {
				state.Monitor = map[string]any{}
			}
			return state
		}
		legacyPath := hostOperationalStatePath()
		if b, err := os.ReadFile(legacyPath); err == nil && len(b) > 0 {
			var legacy hostOperationalState
			if json.Unmarshal(b, &legacy) == nil {
				if legacy.Monitor != nil {
					state = legacy
				}
				if saveErr := saveNodeSetting("host_operational", state); saveErr == nil {
					archiveDir := filepath.Join(filepath.Dir(legacyPath), "backups")
					if os.MkdirAll(archiveDir, 0o750) == nil {
						archivePath := filepath.Join(archiveDir, "legacy-host-operational-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".json")
						_ = os.Rename(legacyPath, archivePath)
					}
				}
			}
		}
		if state.Monitor == nil {
			state.Monitor = map[string]any{}
		}
		return state
	}
	b, err := os.ReadFile(hostOperationalStatePath())
	if err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &state)
	}
	if state.Monitor == nil {
		state.Monitor = map[string]any{}
	}
	return state
}

func saveHostOperationalState(state hostOperationalState) error {
	hostOperationalMu.Lock()
	defer hostOperationalMu.Unlock()
	return saveHostOperationalStateLocked(state)
}

func saveHostOperationalStateLocked(state hostOperationalState) error {
	if sharedDB() != nil {
		if state.Monitor == nil {
			state.Monitor = map[string]any{}
		}
		return saveNodeSetting("host_operational", state)
	}
	if err := os.MkdirAll(filepath.Dir(hostOperationalStatePath()), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmp := hostOperationalStatePath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, hostOperationalStatePath())
}

// hostRuntimeMetrics 读取轻量级本机运行状态，供主机监控接口返回。
func hostRuntimeMetrics(ctx context.Context) map[string]any {
	result := map[string]any{"timestamp": time.Now().UTC(), "cpus": runtime.NumCPU(), "goroutines": runtime.NumGoroutine()}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	result["heapAlloc"] = mem.HeapAlloc
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(b))
		if len(fields) > 0 {
			result["load1"] = fields[0]
		}
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && (f[0] == "MemTotal:" || f[0] == "MemAvailable:") {
				result[strings.TrimSuffix(f[0], ":")] = f[1]
			}
		}
	}
	select {
	case <-ctx.Done():
		result["timedOut"] = true
	default:
	}
	return result
}

type lsblkDevice struct {
	Path       string        `json:"path"`
	Size       uint64        `json:"size"`
	Model      string        `json:"model"`
	Type       string        `json:"type"`
	Removable  bool          `json:"rm"`
	Filesystem string        `json:"fstype"`
	MountPoint string        `json:"mountpoint"`
	Serial     string        `json:"serial"`
	Children   []lsblkDevice `json:"children"`
}

func firewallBinary(name string) string {
	if name == "firewalld" {
		return "firewall-cmd"
	}
	return name
}

func diskUsage(mountPoint string) (used, avail uint64, percent float64) {
	if strings.TrimSpace(mountPoint) == "" {
		return 0, 0, 0
	}
	out, err := exec.Command("df", "-P", "-B1", "--", mountPoint).Output()
	if err != nil {
		return 0, 0, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, 0, 0
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 5 {
		return 0, 0, 0
	}
	total, _ := strconv.ParseUint(fields[1], 10, 64)
	used, _ = strconv.ParseUint(fields[2], 10, 64)
	avail, _ = strconv.ParseUint(fields[3], 10, 64)
	if total > 0 {
		percent = float64(used) * 100 / float64(total)
	}
	return used, avail, percent
}

func diskBasicInfo(d lsblkDevice) map[string]any {
	used, avail, percent := diskUsage(d.MountPoint)
	isSystem := d.MountPoint == "/" || strings.HasPrefix(d.MountPoint, "/boot")
	return map[string]any{
		"device": d.Path, "size": d.Size, "model": strings.TrimSpace(d.Model), "diskType": d.Type,
		"isRemovable": d.Removable, "isSystem": isSystem, "filesystem": d.Filesystem,
		"used": used, "avail": avail, "usePercent": percent, "mountPoint": d.MountPoint,
		"isMounted": strings.TrimSpace(d.MountPoint) != "", "serial": strings.TrimSpace(d.Serial),
	}
}

// hostDiskInfo 从系统块设备和挂载表读取完整磁盘契约。不会缓存或猜测设备状态。
func hostDiskInfo() (map[string]any, error) {
	if runtime.GOOS == "windows" {
		return map[string]any{"disks": []any{}, "unpartitionedDisks": []any{}, "systemDisks": []any{}, "totalDisks": 0, "totalCapacity": uint64(0)}, nil
	}
	path, err := exec.LookPath("lsblk")
	if err != nil {
		return nil, fmt.Errorf("lsblk 不可用: %w", err)
	}
	out, err := exec.Command(path, "-b", "-J", "-o", "PATH,SIZE,MODEL,TYPE,RM,FSTYPE,MOUNTPOINT,SERIAL").Output()
	if err != nil {
		return nil, fmt.Errorf("读取块设备失败: %w", err)
	}
	var payload struct {
		Devices []lsblkDevice `json:"blockdevices"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("解析块设备信息失败: %w", err)
	}
	disks := make([]map[string]any, 0, len(payload.Devices))
	unpartitioned := make([]map[string]any, 0)
	system := make([]map[string]any, 0)
	var totalCapacity uint64
	for _, d := range payload.Devices {
		if d.Type != "disk" && d.Type != "loop" && d.Type != "mmc" {
			continue
		}
		item := diskBasicInfo(d)
		if len(d.Children) > 0 {
			parts := make([]map[string]any, 0, len(d.Children))
			for _, child := range d.Children {
				parts = append(parts, diskBasicInfo(child))
			}
			item["partitions"] = parts
		} else {
			unpartitioned = append(unpartitioned, item)
		}
		if item["isSystem"] == true {
			system = append(system, item)
		}
		disks = append(disks, item)
		totalCapacity += d.Size
	}
	return map[string]any{"disks": disks, "unpartitionedDisks": unpartitioned, "systemDisks": system, "totalDisks": len(disks), "totalCapacity": totalCapacity}, nil
}

func firewallStatus(parent context.Context) map[string]any {
	result := map[string]any{"available": false, "provider": "", "name": "", "backend": "", "isExist": false, "isActive": false, "isInit": false, "isBind": false, "pingStatus": "disable", "ipv4": map[string]any{"available": false, "initialized": false, "bound": false}, "ipv6": map[string]any{"available": false, "initialized": false, "bound": false}}
	providers := []struct {
		name  string
		probe []string
	}{
		{"ufw", []string{"status", "verbose"}},
		{"firewalld", []string{"--state"}},
		{"nftables", []string{"list", "ruleset"}},
		{"iptables", []string{"-L", "-n"}},
	}
	for _, candidate := range providers {
		path, err := exec.LookPath(firewallBinary(candidate.name))
		if err != nil {
			continue
		}
		result["available"], result["isExist"], result["provider"], result["name"], result["backend"], result["path"] = true, true, candidate.name, candidate.name, candidate.name, path
		ctx, cancel := context.WithTimeout(parent, 2*time.Second)
		cmd := exec.CommandContext(ctx, path, candidate.probe...)
		output, runErr := cmd.CombinedOutput()
		cancel()
		result["isActive"] = runErr == nil
		result["isInit"] = runErr == nil
		result["isBind"] = runErr == nil
		if runErr == nil {
			result["pingStatus"] = "enable"
			result["ipv4"] = map[string]any{"available": true, "initialized": true, "bound": true}
			result["ipv6"] = map[string]any{"available": true, "initialized": true, "bound": true}
		} else if len(output) > 0 {
			result["reason"] = strings.TrimSpace(string(output))
		}
		if version, versionErr := exec.Command(path, "--version").CombinedOutput(); versionErr == nil {
			result["version"] = strings.TrimSpace(strings.SplitN(string(version), "\n", 2)[0])
		}
		return result
	}
	result["reason"] = "未检测到可用防火墙命令"
	return result
}

func firewallBackendOptions(parent context.Context) []map[string]any {
	providers := []struct {
		name  string
		probe []string
	}{
		{"iptables", []string{"-L", "-n"}},
		{"nftables", []string{"list", "ruleset"}},
		{"firewalld", []string{"--state"}},
		{"ufw", []string{"status"}},
	}
	options := make([]map[string]any, 0, len(providers))
	for _, p := range providers {
		path, lookErr := exec.LookPath(firewallBinary(p.name))
		option := map[string]any{
			"name": p.name, "installed": lookErr == nil, "active": false, "initialized": false,
			"bound": false, "supported": lookErr == nil, "ipv4": map[string]any{"available": false, "initialized": false, "bound": false},
			"ipv6": map[string]any{"available": false, "initialized": false, "bound": false},
		}
		if lookErr != nil {
			option["supportReason"] = "命令不可用"
			options = append(options, option)
			continue
		}
		ctx, cancel := context.WithTimeout(parent, 2*time.Second)
		result := exec.CommandContext(ctx, path, p.probe...)
		runErr := result.Run()
		cancel()
		active := runErr == nil
		option["active"], option["initialized"], option["bound"] = active, active, active
		option["ipv4"] = map[string]any{"available": active, "initialized": active, "bound": active}
		option["ipv6"] = map[string]any{"available": active, "initialized": active, "bound": active}
		if !active {
			option["message"] = "后端已安装但当前未激活"
		}
		options = append(options, option)
	}
	return options
}

func firewallSettings(parent context.Context) map[string]any {
	options := firewallBackendOptions(parent)
	selected := ""
	for _, item := range options {
		if active, _ := item["active"].(bool); active {
			selected, _ = item["name"].(string)
			break
		}
	}
	group := func() map[string]any {
		return map[string]any{"selected": selected, "current": selected, "options": options}
	}
	return map[string]any{"system": group(), "forwarding": group(), "docker": group(), "pingStatus": "disable", "portWhiteList": ""}
}

func firewallInventory(ctx context.Context, scope map[string]any) (map[string]any, error) {
	provider, _ := scope["provider"].(string)
	if provider == "" {
		status := firewallStatus(ctx)
		provider, _ = status["provider"].(string)
	}
	if provider == "nftables" {
		return nftablesInventory(ctx, scope)
	}
	if provider == "firewalld" {
		return firewalldInventory(ctx, scope)
	}
	if provider == "ufw" {
		return ufwInventory(ctx, scope)
	}
	if provider != "iptables" {
		return nil, fmt.Errorf("暂不支持读取 %s 规则库存", provider)
	}
	family, _ := scope["family"].(string)
	requestedChain, _ := scope["chain"].(string)
	command := "iptables"
	if family == "ipv6" {
		command = "ip6tables"
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return nil, fmt.Errorf("%s 不可用: %w", command, err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "-S").Output()
	if err != nil {
		return nil, fmt.Errorf("读取 %s 规则失败: %w", command, err)
	}
	items := make([]map[string]any, 0)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "-A ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		chain := fields[1]
		if strings.TrimSpace(requestedChain) != "" && chain != requestedChain {
			continue
		}
		rule := map[string]any{"scope": map[string]any{"provider": provider, "family": familyOrIPv4(family), "table": "filter", "chain": chain, "direction": "input"}, "protocol": "all", "action": "accept", "description": line}
		parseStatus := "supported"
		for i := 2; i < len(fields); i++ {
			switch fields[i] {
			case "-p":
				if i+1 < len(fields) {
					rule["protocol"] = fields[i+1]
					i++
				}
			case "-s":
				if i+1 < len(fields) {
					rule["sourceAddress"] = fields[i+1]
					i++
				}
			case "-d":
				if i+1 < len(fields) {
					rule["destinationAddress"] = fields[i+1]
					i++
				}
			case "--dport", "--destination-port":
				if i+1 < len(fields) {
					rule["destinationPort"] = fields[i+1]
					i++
				}
			case "-j":
				if i+1 < len(fields) {
					jump := strings.ToLower(fields[i+1])
					switch jump {
					case "accept", "drop", "reject":
						rule["action"] = jump
					default:
						parseStatus = "opaque"
					}
					i++
				}
			}
		}
		digest := sha256.Sum256([]byte(line))
		uuid := fmt.Sprintf("%x", digest[:16])
		rule["uuid"] = uuid
		observed := map[string]any{"rule": rule, "locator": map[string]any{"provider": provider, "scopeKey": provider + ":" + chain, "canonical": line}, "parseStatus": parseStatus, "raw": line, "protected": false}
		items = append(items, map[string]any{"rule": rule, "observed": observed, "state": "external", "match": "none"})
	}
	return map[string]any{"items": items, "notices": []any{}}, nil
}

func familyOrIPv4(family string) string {
	if family == "ipv6" {
		return "ipv6"
	}
	if family == "inet" {
		return "inet"
	}
	return "ipv4"
}
