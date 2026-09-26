// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
)

// hostMutationDenied 表示写宿主机被显式开关拒绝。
type hostMutationDenied struct{}

func (hostMutationDenied) Error() string {
	return "修改宿主机需要 WORKMESH_ALLOW_HOST_MUTATION=1"
}

// hostMutationAllowed 复用守护进程同一开关，避免工具箱写操作绕过宿主机变更确认。
func hostMutationAllowed() bool { return supervisorMutationAllowed() }

func hostBinary(name string) (string, error) {
	return exec.LookPath(name)
}

func hostCommand(ctx context.Context, timeout time.Duration, program string, args ...string) (model.CommandResult, error) {
	return (service.CommandService{}).Execute(ctx, model.CommandRequest{Program: program, Args: args, Timeout: timeout})
}

// systemdUnitActive 只根据 systemctl is-active 的成功退出判断服务是否在运行。
func systemdUnitActive(ctx context.Context, units ...string) bool {
	if _, err := hostBinary("systemctl"); err != nil {
		return false
	}
	for _, unit := range units {
		unit = strings.TrimSpace(unit)
		if unit == "" || strings.ContainsAny(unit, " \t/\\") {
			continue
		}
		result, err := hostCommand(ctx, 5*time.Second, "systemctl", "is-active", unit)
		if err == nil && result.ExitCode == 0 && strings.TrimSpace(result.Stdout) == "active" {
			return true
		}
	}
	return false
}

func readLimitedHostFile(path string, limit int) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if limit > 0 && len(content) > limit {
		content = content[:limit]
	}
	return string(content), nil
}

func mutationRequired() error {
	if hostMutationAllowed() {
		return nil
	}
	return hostMutationDenied{}
}

func hostExec(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

func writePublicHostFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func validHostUser(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for i, r := range name {
		ok := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if i > 0 && ((r >= '0' && r <= '9') || r == '-' || r == '.') {
			ok = true
		}
		if !ok {
			return false
		}
	}
	return true
}

func decodePanelSecret(value string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return "", errors.New("密码编码无效")
	}
	secret := string(raw)
	if secret == "" || len(secret) > 128 || strings.ContainsAny(secret, "\r\n\x00") {
		return "", errors.New("密码无效")
	}
	return secret, nil
}

func parseNameServers(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t' || r == ';'
	})
	servers := make([]string, 0, 4)
	for _, field := range fields {
		field = strings.TrimPrefix(strings.TrimSpace(field), "nameserver")
		field = strings.TrimSpace(field)
		if net.ParseIP(field) == nil {
			continue
		}
		servers = append(servers, field)
		if len(servers) >= 4 {
			break
		}
	}
	return servers
}

func commandVersion(ctx context.Context, program string, args ...string) string {
	if _, err := hostBinary(program); err != nil {
		return ""
	}
	result, err := hostCommand(ctx, 5*time.Second, program, args...)
	if err != nil && result.Stdout == "" {
		return ""
	}
	line := strings.TrimSpace(result.Stdout)
	if index := strings.IndexByte(line, '\n'); index >= 0 {
		line = strings.TrimSpace(line[:index])
	}
	if len(line) > 200 {
		line = line[:200]
	}
	return line
}
