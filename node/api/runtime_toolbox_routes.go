// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// registerTerminalRoutes 注册运行时相关 HTTP 路由，并保持请求响应契约。
func registerTerminalRoutes(mux *http.ServeMux) {
	for _, p := range []string{"/api/v2/hosts/terminal/local", "/api/v2/hosts/terminal/container", "/api/v2/hosts/terminal/ssh"} {
		mux.HandleFunc("GET "+p, handleTerminalStream)
	}
}

// registerSSHRoutes 注册运行时相关 HTTP 路由，并保持请求响应契约。
func registerSSHRoutes(mux *http.ServeMux, s *runtimeStore) {
	get := func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, loadSSHConnResponse())
	}
	mux.HandleFunc("GET /api/v2/settings/ssh/conn", get)
	mux.HandleFunc("POST /api/v2/settings/ssh", func(w http.ResponseWriter, r *http.Request) {
		v, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, "解析 SSH 配置失败: "+err.Error())
			return
		}
		if err := validateSSHConfig(v); err != nil {
			runtimeErr(w, 400, err.Error())
			return
		}
		if err := saveNodeSetting("ssh", v); err != nil {
			runtimeErr(w, 500, "保存 SSH 配置失败: "+err.Error())
			return
		}
		out := cloneMapRuntime(v)
		delete(out, "password")
		delete(out, "privateKey")
		delete(out, "passPhrase")
		runtimeOK(w, map[string]any{"config": out})
	})
	mux.HandleFunc("POST /api/v2/settings/ssh/default", func(w http.ResponseWriter, r *http.Request) {
		v, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, "解析默认连接配置失败: "+err.Error())
			return
		}
		show, ok := normalizeLocalSSHConnShow(runtimeString(v, "defaultConn"))
		if !ok {
			runtimeErr(w, 400, "defaultConn 无效")
			return
		}
		if show == "Disable" && hostBoolValue(v["withReset"]) {
			if err := saveNodeSetting("ssh", map[string]any{}); err != nil {
				runtimeErr(w, 500, "重置 SSH 连接失败: "+err.Error())
				return
			}
		}
		if err := setDomainSetting("localSSHConnShow", show); err != nil {
			runtimeErr(w, 500, "保存默认连接配置失败: "+err.Error())
			return
		}
		runtimeOK(w, map[string]any{"defaultConn": show, "localSSHConnShow": show})
	})
	mux.HandleFunc("POST /api/v2/settings/ssh/check/info", func(w http.ResponseWriter, r *http.Request) {
		v, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, "解析 SSH 测试参数失败: "+err.Error())
			return
		}
		runtimeSSHCheck(w, v)
	})
	mux.HandleFunc("POST /api/v2/settings/ssh/check", func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		if !loadNodeSetting("ssh", &v) || v == nil || runtimeString(v, "host", "addr", "address") == "" {
			runtimeOK(w, false)
			return
		}
		runtimeSSHCheck(w, v)
	})
}

// loadSSHConnResponse 返回本地 SSH 连接的非敏感字段，并带上默认连接开关。
func loadSSHConnResponse() map[string]any {
	var stored map[string]any
	if !loadNodeSetting("ssh", &stored) || stored == nil {
		stored = map[string]any{}
	}
	out := cloneMapRuntime(stored)
	delete(out, "password")
	delete(out, "privateKey")
	delete(out, "passPhrase")
	if runtimeString(out, "addr") == "" {
		if host := runtimeString(out, "host", "address"); host != "" {
			out["addr"] = host
		}
	}
	show := domainSettingString("localSSHConnShow")
	if show == "" {
		show = "disable"
	}
	out["localSSHConnShow"] = show
	return out
}

// normalizeLocalSSHConnShow 把默认连接开关规范为前端比较使用的 Enable/Disable。
func normalizeLocalSSHConnShow(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "enable":
		return "Enable", true
	case "disable":
		return "Disable", true
	default:
		return "", false
	}
}

// cloneMapRuntime 复制运行时数据，避免调用方共享可变状态。
func cloneMapRuntime(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

// validateSSHConfig 校验运行时参数和外部资源边界。
func validateSSHConfig(v map[string]any) error {
	host := runtimeString(v, "host", "addr", "address")
	if host == "" {
		return errors.New("SSH 主机地址不能为空")
	}
	port := runtimeIntValue(v["port"])
	if port == 0 {
		port = 22
	}
	if port < 1 || port > 65535 {
		return errors.New("SSH 端口无效")
	}
	return nil
}

// runtimeSSHCheck 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeSSHCheck(w http.ResponseWriter, v map[string]any) {
	if err := validateSSHConfig(v); err != nil {
		runtimeErr(w, 400, err.Error())
		return
	}
	host := runtimeString(v, "host", "addr", "address")
	port := runtimeIntValue(v["port"])
	if port == 0 {
		port = 22
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	if conn != nil {
		_ = conn.Close()
	}
	runtimeOK(w, err == nil)
}

// runtimeIntValue 处理运行时业务规则，并保持 SQLite 与外部资源一致。
func runtimeIntValue(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	default:
		return 0
	}
}

// registerToolboxRoutes 注册运行时相关 HTTP 路由，并保持请求响应契约。
func registerToolboxRoutes(mux *http.ServeMux, s *runtimeStore) {
	// 显式注册查询路由，便于契约扫描和文档准确发现每个功能。
	mux.HandleFunc("GET /api/v2/toolbox/device/users", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, listHostUsers())
	})
	mux.HandleFunc("GET /api/v2/toolbox/device/zone/options", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, listTimeZones())
	})
	mux.HandleFunc("GET /api/v2/toolbox/fail2ban/base", func(w http.ResponseWriter, r *http.Request) {
		runtimeOK(w, fail2banBaseInfo(r.Context(), s))
	})
	mux.HandleFunc("GET /api/v2/toolbox/fail2ban/load/conf", func(w http.ResponseWriter, _ *http.Request) {
		content, err := readFail2banConfig(s)
		if err != nil {
			runtimeErr(w, http.StatusInternalServerError, "读取 Fail2ban 配置失败: "+err.Error())
			return
		}
		runtimeOK(w, content)
	})
	mux.HandleFunc("GET /api/v2/toolbox/ftp/base", func(w http.ResponseWriter, r *http.Request) {
		runtimeOK(w, ftpBaseInfo(r.Context()))
	})
	registerToolboxDeviceRoutes(mux, s)
	registerToolboxFail2BanRoutes(mux, s)
	registerToolboxFtpRoutes(mux, s)
	registerToolboxCleanRoutes(mux)
	registerToolboxClamRoutes(mux)
	registerTerminalAIRoutes(mux)
	_ = s
}

const (
	terminalAISettingKey          = "terminal_ai"
	terminalAIDefaultStatus       = "Disable"
	terminalAIDefaultPrefix       = "@ai"
	terminalAIDefaultRiskCommands = `["rm","mkfs","dd if=","dd of=/dev/","wipefs","fdisk","parted","sfdisk","shred","curl | sh","wget | sh","chmod -R 777 /","shutdown","reboot","poweroff","init 0",":(){ :|:& };:"]`
)

// registerTerminalAIRoutes 注册终端 AI 设置，并将完整配置保存到 node_settings。
func registerTerminalAIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/settings/terminal/ai/search", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, loadTerminalAISettings())
	})
	mux.HandleFunc("POST /api/v2/settings/terminal/ai/update", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, "解析终端 AI 配置失败: "+err.Error())
			return
		}
		settings, err := normalizeTerminalAISettings(body)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := saveNodeSetting(terminalAISettingKey, settings); err != nil {
			runtimeErr(w, http.StatusInternalServerError, "保存终端 AI 配置失败: "+err.Error())
			return
		}
		runtimeOK(w, map[string]any{"status": "accepted", "config": settings})
	})
}

// loadTerminalAISettings 返回前端需要的完整终端 AI 配置，并在缺少记录时使用安全默认值。
func loadTerminalAISettings() map[string]any {
	stored := map[string]any{}
	if !loadNodeSetting(terminalAISettingKey, &stored) || len(stored) == 0 {
		settings := terminalAIDefaultSettings()
		settings["aiRiskCommands"] = terminalAIDefaultRiskCommands
		return settings
	}
	settings, err := normalizeTerminalAISettings(stored)
	if err != nil {
		fallback := terminalAIDefaultSettings()
		fallback["aiRiskCommands"] = terminalAIDefaultRiskCommands
		return fallback
	}
	return settings
}

func terminalAIDefaultSettings() map[string]any {
	return map[string]any{
		"aiStatus":              terminalAIDefaultStatus,
		"aiAccountId":           "",
		"aiPrefix":              terminalAIDefaultPrefix,
		"aiRiskCommands":        "[]",
		"aiRiskCommandsDefault": terminalAIDefaultRiskCommands,
	}
}

// normalizeTerminalAISettings 在写入前完成字段归一化，避免保存不可解析或不稳定的风险命令 JSON。
func normalizeTerminalAISettings(body map[string]any) (map[string]any, error) {
	settings := terminalAIDefaultSettings()
	if value, exists := body["aiStatus"]; exists {
		status, err := normalizeTerminalAIStatus(value)
		if err != nil {
			return nil, err
		}
		settings["aiStatus"] = status
	}
	if value, exists := body["aiPrefix"]; exists {
		prefix, err := normalizeTerminalAIPrefix(value)
		if err != nil {
			return nil, err
		}
		settings["aiPrefix"] = prefix
	}
	if value, exists := body["aiRiskCommands"]; exists {
		riskCommands, err := normalizeTerminalAIRiskCommands(value)
		if err != nil {
			return nil, err
		}
		settings["aiRiskCommands"] = riskCommands
	}
	if value, exists := body["aiAccountId"]; exists {
		accountID, ok := value.(string)
		if !ok {
			if converted := runtimeString(map[string]any{"value": value}, "value"); converted != "" {
				accountID = converted
			} else {
				return nil, errors.New("aiAccountId 必须是字符串")
			}
		}
		settings["aiAccountId"] = strings.TrimSpace(accountID)
	}
	if settings["aiStatus"] == terminalAIDefaultStatus {
		settings["aiAccountId"] = ""
	} else if strings.TrimSpace(settings["aiAccountId"].(string)) == "" {
		return nil, errors.New("启用终端 AI 时 aiAccountId 不能为空")
	}
	return settings, nil
}

func normalizeTerminalAIStatus(value any) (string, error) {
	status, ok := value.(string)
	if !ok {
		return "", errors.New("aiStatus 必须是 Enable 或 Disable")
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "enable":
		return "Enable", nil
	case "disable", "":
		return terminalAIDefaultStatus, nil
	default:
		return "", errors.New("aiStatus 必须是 Enable 或 Disable")
	}
}

func normalizeTerminalAIPrefix(value any) (string, error) {
	prefix, ok := value.(string)
	if !ok {
		return "", errors.New("aiPrefix 必须是 ASCII 可见字符")
	}
	if prefix == "" {
		return terminalAIDefaultPrefix, nil
	}
	if len(prefix) > 64 {
		return "", errors.New("aiPrefix 长度不能超过 64 个字符")
	}
	for i := 0; i < len(prefix); i++ {
		if prefix[i] < 0x21 || prefix[i] > 0x7e || prefix[i] == ' ' {
			return "", errors.New("aiPrefix 只能包含不含空格的 ASCII 可见字符")
		}
	}
	return prefix, nil
}

func normalizeTerminalAIRiskCommands(value any) (string, error) {
	if value == nil {
		return "[]", nil
	}
	raw, ok := value.(string)
	if !ok {
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", errors.New("aiRiskCommands 必须是 JSON 数组")
		}
		raw = string(encoded)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "[]", nil
	}
	var commands []string
	if err := json.Unmarshal([]byte(raw), &commands); err != nil {
		return "", errors.New("aiRiskCommands 必须是字符串 JSON 数组")
	}
	seen := make(map[string]struct{}, len(commands))
	normalized := make([]string, 0, len(commands))
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		if _, exists := seen[command]; exists {
			continue
		}
		seen[command] = struct{}{}
		normalized = append(normalized, command)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", errors.New("aiRiskCommands JSON 编码失败")
	}
	return string(encoded), nil
}

// registerToolboxDeviceRoutes 注册设备信息、主机配置和 DNS 探测。
func registerToolboxDeviceRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("POST /api/v2/toolbox/device/base", func(w http.ResponseWriter, r *http.Request) {
		runtimeOK(w, deviceBaseInfo(r.Context()))
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/check/dns", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, "解析 DNS 请求失败: "+err.Error())
			return
		}
		resolved, err := checkDeviceDNS(r.Context(), body)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if runtimeString(body, "host", "domain") != "" {
			runtimeOK(w, map[string]any{"host": runtimeString(body, "host", "domain"), "resolved": resolved})
			return
		}
		runtimeOK(w, resolved)
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/conf", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, "解析设备配置失败: "+err.Error())
			return
		}
		content, err := deviceConfContent(runtimeString(body, "name"))
		if err != nil {
			if os.IsNotExist(err) {
				runtimeOK(w, "")
				return
			}
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		runtimeOK(w, content)
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/update/conf", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, "解析设备配置失败: "+err.Error())
			return
		}
		if err := applyDeviceConf(r.Context(), runtimeString(body, "key"), runtimeString(body, "value")); err != nil {
			writeHostMutationError(w, err)
			return
		}
		runtimeOK(w, map[string]any{"updated": true})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/update/byconf", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, "解析设备配置失败: "+err.Error())
			return
		}
		name := strings.ToLower(runtimeString(body, "name"))
		file := runtimeString(body, "file")
		var applyErr error
		switch name {
		case "hosts":
			applyErr = applyHostsFile(file)
		case "dns":
			applyErr = writeNameServers(file)
		default:
			applyErr = errors.New("设备配置名称无效")
		}
		if applyErr != nil {
			writeHostMutationError(w, applyErr)
			return
		}
		runtimeOK(w, map[string]any{"updated": true})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/update/host", func(w http.ResponseWriter, r *http.Request) {
		raw, err := decodeDeviceBody(r.Body)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, "解析 Hosts 失败: "+err.Error())
			return
		}
		if err := applyHostsEntries(r.Context(), raw); err != nil {
			writeHostMutationError(w, err)
			return
		}
		runtimeOK(w, map[string]any{"updated": true})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/update/swap", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, "解析交换分区请求失败: "+err.Error())
			return
		}
		if err := applyDeviceSwap(r.Context(), body); err != nil {
			writeHostMutationError(w, err)
			return
		}
		runtimeOK(w, map[string]any{"updated": true})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/update/passwd", toolboxDevicePasswordHandler())
}

func writeHostMutationError(w http.ResponseWriter, err error) {
	var denied hostMutationDenied
	if errors.As(err, &denied) {
		runtimeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	runtimeErr(w, http.StatusBadRequest, err.Error())
}

// toolboxDevicePasswordHandler 校验密码更新请求且不持久化敏感字段。
func toolboxDevicePasswordHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, "解析密码请求失败: "+err.Error())
			return
		}
		encoded := runtimeString(body, "password", "passwd")
		if encoded == "" {
			runtimeErr(w, http.StatusBadRequest, "密码不能为空")
			return
		}
		if err := applyDevicePassword(r.Context(), runtimeString(body, "user"), encoded); err != nil {
			writeHostMutationError(w, err)
			return
		}
		runtimeOK(w, map[string]any{"updated": true})
	}
}

// registerToolboxFail2BanRoutes 注册 Fail2ban 配置读取与受限服务操作。
