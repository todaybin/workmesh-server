// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var sshDirectivePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

func sshdConfigPath() string {
	if value := strings.TrimSpace(os.Getenv("WORKMESH_SSHD_CONFIG")); value != "" {
		return value
	}
	return "/etc/ssh/sshd_config"
}

func sshServiceName() string {
	if value := strings.TrimSpace(os.Getenv("WORKMESH_SSH_SERVICE")); value != "" {
		return value
	}
	return "sshd"
}

func registerHostSSHRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/hosts/ssh/search", handleSSHInfo)
	mux.HandleFunc("POST /api/v2/hosts/ssh/operate", handleSSHOperate)
	mux.HandleFunc("POST /api/v2/hosts/ssh/update", handleSSHUpdate)
	mux.HandleFunc("POST /api/v2/hosts/ssh/file", handleSSHFileRead)
	mux.HandleFunc("POST /api/v2/hosts/ssh/file/update", handleSSHFileUpdate)
}

func handleSSHInfo(w http.ResponseWriter, r *http.Request) {
	configPath := sshdConfigPath()
	config, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "读取 SSH 配置失败: " + err.Error()})
		return
	}
	values := parseSSHDirectives(string(config))
	service, active, enabled, statusMessage := sshServiceStatus(r)
	port := values["Port"]
	if port == "" {
		port = "22"
	}
	result := map[string]any{
		"autoStart": enabled, "isExist": service != "" || err == nil, "isActive": active,
		"message": statusMessage, "port": port,
		"listenAddress":          values["ListenAddress"],
		"passwordAuthentication": defaultSSHValue(values["PasswordAuthentication"], "yes"),
		"pubkeyAuthentication":   defaultSSHValue(values["PubkeyAuthentication"], "yes"),
		"permitRootLogin":        defaultSSHValue(values["PermitRootLogin"], "yes"),
		"useDNS":                 defaultSSHValue(values["UseDNS"], "yes"),
	}
	if current, err := user.Current(); err == nil {
		result["currentUser"] = current.Username
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
}

func parseSSHDirectives(content string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !sshDirectivePattern.MatchString(fields[0]) {
			continue
		}
		if _, exists := result[fields[0]]; !exists {
			result[fields[0]] = strings.Join(fields[1:], " ")
		}
	}
	return result
}

func defaultSSHValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func sshServiceStatus(r *http.Request) (string, bool, bool, string) {
	name := sshServiceName()
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return "", false, false, "systemctl 不可用"
	}
	ctx := r.Context()
	active := exec.CommandContext(ctx, systemctl, "is-active", name).Run() == nil
	enabled := exec.CommandContext(ctx, systemctl, "is-enabled", name).Run() == nil
	exists := exec.CommandContext(ctx, systemctl, "cat", name).Run() == nil
	if !exists {
		return "", active, enabled, "SSH 服务单元不存在"
	}
	return name, active, enabled, ""
}

func handleSSHOperate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Operation string `json:"operation"`
	}
	if err := decodeJSON(r, &request); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	operation := strings.ToLower(strings.TrimSpace(request.Operation))
	if operation != "start" && operation != "stop" && operation != "restart" && operation != "enable" && operation != "disable" {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "SSH 操作只允许 start、stop、restart、enable 或 disable"})
		return
	}
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "systemctl 不可用，无法操作 SSH 服务"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, systemctl, operation, sshServiceName()).CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": "SSH 服务操作失败: " + message})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"operation": operation, "service": sshServiceName()}})
}

func contextWithTimeout(r *http.Request, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), timeout)
}

func handleSSHUpdate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Key      string `json:"key"`
		NewValue string `json:"newValue"`
	}
	if err := decodeJSON(r, &request); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	path := sshdConfigPath()
	old, _ := os.ReadFile(path)
	if err := updateSSHConfig(request.Key, request.NewValue); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if err := restartSSHService(r); err != nil {
		_ = writeAtomicSSHFile(path, old, 0o600)
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": nil})
}

func updateSSHConfig(key, value string) error {
	if !sshDirectivePattern.MatchString(strings.TrimSpace(key)) {
		return errors.New("SSH 配置项名称无效")
	}
	allowed := map[string]bool{"Port": true, "ListenAddress": true, "PasswordAuthentication": true, "PubkeyAuthentication": true, "PermitRootLogin": true, "UseDNS": true, "AllowUsers": true, "AllowGroups": true}
	if !allowed[key] {
		return fmt.Errorf("SSH 配置项 %q 不允许修改", key)
	}
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 1024 || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("SSH 配置值无效")
	}
	if key == "Port" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return errors.New("SSH 端口无效")
		}
	}
	path := sshdConfigPath()
	old, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 SSH 配置失败: %w", err)
	}
	lines := strings.Split(string(old), "\n")
	found := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) > 0 && fields[0] == key {
			lines[index] = key + " " + value
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, key+" "+value)
	}
	updated := []byte(strings.Join(lines, "\n"))
	if err := writeAtomicSSHFile(path, updated, 0o600); err != nil {
		return err
	}
	if sshd, lookErr := exec.LookPath("sshd"); lookErr == nil {
		if output, checkErr := exec.Command(sshd, "-t", "-f", path).CombinedOutput(); checkErr != nil {
			_ = writeAtomicSSHFile(path, old, 0o600)
			message := strings.TrimSpace(string(output))
			if message == "" {
				message = checkErr.Error()
			}
			return fmt.Errorf("SSH 配置语法校验失败，已恢复旧文件: %s", message)
		}
	}
	return nil
}

func restartSSHService(r *http.Request) error {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return errors.New("systemctl 不可用，SSH 配置已保存但未能重启服务")
	}
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, systemctl, "restart", sshServiceName()).CombinedOutput(); err != nil {
		return fmt.Errorf("SSH 重启失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func sshFilePath(key, requested string) (string, error) {
	switch key {
	case "authKeys":
		current, err := user.Current()
		if err != nil {
			return "", err
		}
		return filepath.Join(current.HomeDir, ".ssh", "authorized_keys"), nil
	case "sshdConf":
		return sshdConfigPath(), nil
	case "sshdConfPath":
		path := filepath.Clean(strings.TrimSpace(requested))
		root := filepath.Dir(sshdConfigPath())
		if path != sshdConfigPath() && !strings.HasPrefix(path, root+string(os.PathSeparator)) {
			return "", errors.New("SSH 配置路径不在允许目录内")
		}
		return path, nil
	default:
		return "", errors.New("不支持的 SSH 文件类型")
	}
}

func handleSSHFileRead(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &request); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if request.Name == "sshdConfOptions" {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": []string{sshdConfigPath()}})
		return
	}
	key, requested := request.Name, ""
	if strings.HasPrefix(request.Name, "sshdConfPath:") {
		key, requested = "sshdConfPath", strings.TrimPrefix(request.Name, "sshdConfPath:")
	}
	path, err := sshFilePath(key, requested)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if len(content) > 2<<20 {
		wmhttp.JSON(w, http.StatusRequestEntityTooLarge, map[string]any{"code": "ERR", "message": "SSH 文件超过 2 MiB 限制"})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": string(content)})
}

func handleSSHFileUpdate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Key   string `json:"key"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}
	if err := decodeJSON(r, &request); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	path, err := sshFilePath(request.Key, request.Path)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if len(request.Value) > 2<<20 || strings.IndexByte(request.Value, 0) >= 0 {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "SSH 文件内容无效"})
		return
	}
	old, _ := os.ReadFile(path)
	if err := writeAtomicSSHFile(path, []byte(request.Value), 0o600); err != nil {
		wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if request.Key != "authKeys" {
		if err := validateSSHConfigFile(path); err != nil {
			_ = writeAtomicSSHFile(path, old, 0o600)
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
	}
	if request.Key != "authKeys" {
		if err := restartSSHService(r); err != nil {
			_ = writeAtomicSSHFile(path, old, 0o600)
			wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": nil})
}

func validateSSHConfigFile(path string) error {
	sshd, err := exec.LookPath("sshd")
	if err != nil {
		return nil
	}
	output, err := exec.Command(sshd, "-t", "-f", path).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("SSH 配置语法校验失败: %s", message)
	}
	return nil
}

func writeAtomicSSHFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".workmesh-ssh-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
