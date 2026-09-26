// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// registerToolboxFail2BanRoutes 注册运行时相关 HTTP 路由，并保持请求响应契约。
func registerToolboxFail2BanRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("POST /api/v2/toolbox/fail2ban/search", fail2banSearchHandler(s))
	for _, path := range []string{"/api/v2/toolbox/fail2ban/update", "/api/v2/toolbox/fail2ban/update/byconf"} {
		mux.HandleFunc("POST "+path, fail2banUpdateHandler(s))
	}
	for _, path := range []string{"/api/v2/toolbox/fail2ban/operate", "/api/v2/toolbox/fail2ban/operate/sshd"} {
		mux.HandleFunc("POST "+path, fail2banOperateHandler())
	}
}

// fail2banConfigPath 返回 Fail2ban 配置文件路径。
func fail2banConfigPath(s *runtimeStore) string {
	if value := strings.TrimSpace(os.Getenv("WORKMESH_FAIL2BAN_CONFIG")); value != "" {
		return filepath.Clean(value)
	}
	_ = s
	if _, err := os.Stat("/etc/fail2ban/jail.local"); err == nil {
		return "/etc/fail2ban/jail.local"
	}
	if _, err := os.Stat("/etc/fail2ban/jail.conf"); err == nil {
		return "/etc/fail2ban/jail.conf"
	}
	return "/etc/fail2ban/jail.local"
}

func fail2banBaseInfo(ctx context.Context, s *runtimeStore) map[string]any {
	content, _ := readFail2banConfig(s)
	section := parseFail2banSection(content, "sshd")
	port, _ := strconv.Atoi(section["port"])
	maxRetry, _ := strconv.Atoi(section["maxretry"])
	if port == 0 {
		port = 22
	}
	if maxRetry == 0 {
		maxRetry = 5
	}
	enabled := strings.EqualFold(section["enabled"], "true")
	exist := hostBinaryExists("fail2ban-client")
	return map[string]any{
		"isExist": exist, "isActive": exist && systemdUnitActive(ctx, "fail2ban"), "isEnable": enabled,
		"version": commandVersion(ctx, "fail2ban-client", "version"), "port": port, "maxRetry": maxRetry,
		"banTime": section["bantime"], "findTime": section["findtime"], "banAction": section["banaction"], "logPath": section["logpath"],
	}
}

func hostBinaryExists(name string) bool {
	_, err := hostBinary(name)
	return err == nil
}

func parseFail2banSection(content, name string) map[string]string {
	values := map[string]string{}
	active := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			active = strings.EqualFold(strings.Trim(line, "[]"), name)
			continue
		}
		if !active {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return values
}

// readFail2banConfig 读取受大小限制的真实 Fail2ban 配置。
func readFail2banConfig(s *runtimeStore) (string, error) {
	value, err := os.ReadFile(fail2banConfigPath(s))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(value) > 1<<20 {
		return "", errors.New("Fail2ban 配置超过 1 MiB 限制")
	}
	return string(value), nil
}

// fail2banSearchHandler 查询真实 Fail2ban 配置内容。
func fail2banSearchHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, err.Error())
			return
		}
		content, err := readFail2banConfig(s)
		if err != nil {
			runtimeErr(w, 500, "读取 Fail2ban 配置失败: "+err.Error())
			return
		}
		if status := runtimeString(body, "status"); status != "" {
			runtimeOK(w, fail2banStatusAddresses(r.Context(), status))
			return
		}
		keyword := runtimeString(body, "keyword", "name")
		lines := make([]string, 0, 100)
		for _, line := range strings.Split(content, "\n") {
			if keyword == "" || strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
				if strings.TrimSpace(line) != "" {
					lines = append(lines, line)
				}
				if len(lines) >= 100 {
					break
				}
			}
		}
		runtimeOK(w, lines)
	}
}

// fail2banUpdateHandler 原子写入真实 Fail2ban 配置文件。
func fail2banUpdateHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, err.Error())
			return
		}
		content := runtimeString(body, "content", "conf", "file")
		if content == "" && runtimeString(body, "key") != "" {
			current, readErr := readFail2banConfig(s)
			if readErr != nil {
				runtimeErr(w, 500, "读取 Fail2ban 配置失败: "+readErr.Error())
				return
			}
			content = upsertFail2banValue(current, runtimeString(body, "key"), runtimeString(body, "value"))
		}
		if content == "" || len(content) > 1<<20 {
			runtimeErr(w, 400, "Fail2ban 配置内容无效")
			return
		}
		file := fail2banConfigPath(s)
		if strings.HasPrefix(file, "/etc/") && !hostMutationAllowed() {
			runtimeErr(w, http.StatusServiceUnavailable, "修改 Fail2ban 需要 WORKMESH_ALLOW_HOST_MUTATION=1")
			return
		}
		if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
			runtimeErr(w, 500, "创建 Fail2ban 配置目录失败: "+err.Error())
			return
		}
		tmp := file + ".tmp"
		if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
			runtimeErr(w, 500, "写入 Fail2ban 配置失败: "+err.Error())
			return
		}
		if err := os.Rename(tmp, file); err != nil {
			_ = os.Remove(tmp)
			runtimeErr(w, 500, "替换 Fail2ban 配置失败: "+err.Error())
			return
		}
		runtimeOK(w, map[string]any{"updated": true, "path": file})
	}
}

// fail2banOperateHandler 执行受限的 Fail2ban 服务操作。
func fail2banOperateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, err.Error())
			return
		}
		action := strings.ToLower(runtimeString(body, "operation", "operate", "action"))
		if action != "start" && action != "stop" && action != "restart" && action != "enable" && action != "disable" {
			runtimeErr(w, 400, "Fail2ban 操作无效")
			return
		}
		if !hostMutationAllowed() {
			runtimeErr(w, http.StatusServiceUnavailable, "修改 Fail2ban 需要 WORKMESH_ALLOW_HOST_MUTATION=1")
			return
		}
		if _, err := hostBinary("systemctl"); err != nil || !hostBinaryExists("fail2ban-client") {
			runtimeErr(w, http.StatusServiceUnavailable, "fail2ban 未安装")
			return
		}
		command := action
		if action == "enable" || action == "disable" {
			command = action
		}
		if _, err := hostCommand(r.Context(), 20*time.Second, "systemctl", command, "fail2ban"); err != nil {
			runtimeErr(w, http.StatusBadGateway, "执行 Fail2ban 操作失败")
			return
		}
		runtimeOK(w, map[string]any{"operation": action})
	}
}

func fail2banStatusAddresses(ctx context.Context, status string) []string {
	if !hostBinaryExists("fail2ban-client") {
		return []string{}
	}
	result, err := hostCommand(ctx, 8*time.Second, "fail2ban-client", "status", "sshd")
	if err != nil && strings.TrimSpace(result.Stdout) == "" {
		return []string{}
	}
	marker := "Banned IP list:"
	if status == "ignore" {
		marker = "Ignored IP list:"
	}
	addresses := make([]string, 0)
	for _, line := range strings.Split(result.Stdout, "\n") {
		if !strings.Contains(line, marker) {
			continue
		}
		_, value, ok := strings.Cut(line, marker)
		if !ok {
			continue
		}
		for _, item := range strings.Fields(value) {
			item = strings.Trim(item, ",")
			if item != "" {
				addresses = append(addresses, item)
			}
		}
	}
	return addresses
}

func upsertFail2banValue(content, key, value string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	if key == "" || strings.ContainsAny(key, "\r\n#;[]=") || strings.ContainsAny(value, "\r\n") {
		return content
	}
	lines := strings.Split(content, "\n")
	if strings.TrimSpace(content) == "" {
		lines = []string{"[sshd]"}
	}
	active := false
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if active && !replaced {
				lines = append(lines[:i], append([]string{key + " = " + value}, lines[i:]...)...)
				replaced = true
			}
			active = strings.EqualFold(strings.Trim(trimmed, "[]"), "sshd")
			continue
		}
		if !active {
			continue
		}
		current, _, ok := strings.Cut(trimmed, "=")
		if ok && strings.EqualFold(strings.TrimSpace(current), key) {
			lines[i] = key + " = " + value
			replaced = true
		}
	}
	if !replaced {
		if !strings.Contains(content, "[sshd]") {
			lines = append(lines, "[sshd]")
		}
		lines = append(lines, key+" = "+value)
	}
	return strings.Join(lines, "\n")
}

// registerToolboxFtpRoutes 管理本机 pure-ftpd 用户，不回传密码。
func registerToolboxFtpRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("POST /api/v2/toolbox/ftp/search", ftpSearchHandler(s))
	for _, path := range []string{"/api/v2/toolbox/ftp", "/api/v2/toolbox/ftp/update"} {
		mux.HandleFunc("POST "+path, ftpSaveHandler(s, path))
	}
	mux.HandleFunc("POST /api/v2/toolbox/ftp/del", ftpDeleteHandler(s))
	mux.HandleFunc("POST /api/v2/toolbox/ftp/operate", ftpOperateHandler(s))
	mux.HandleFunc("POST /api/v2/toolbox/ftp/sync", ftpSyncHandler())
	mux.HandleFunc("POST /api/v2/toolbox/ftp/log/search", ftpLogSearchHandler(s))
}

// appendFTPLog 在共享 SQLite 状态中保留最近 FTP 操作记录。
func appendFTPLog(s *runtimeStore, action, id, detail string) {
	record := map[string]any{"id": "ftp-log-" + strconv.FormatInt(time.Now().UnixNano(), 10), "action": action, "ftpId": id, "detail": detail, "createdAt": time.Now().UTC().Format(time.RFC3339Nano)}
	value, _ := s.state.Settings["ftp.logs"].([]any)
	value = append(value, record)
	if len(value) > 1000 {
		value = value[len(value)-1000:]
	}
	s.state.Settings["ftp.logs"] = value
}

// ftpEntries 返回脱敏后的 FTP 配置列表。
func ftpEntries(s *runtimeStore) []map[string]any {
	value, _ := s.state.Settings["ftp.entries"].([]any)
	result := make([]map[string]any, 0, len(value))
	for _, item := range value {
		if typed, ok := item.(map[string]any); ok {
			copy := map[string]any{}
			for key, val := range typed {
				if key != "password" {
					copy[key] = val
				}
			}
			result = append(result, copy)
		}
	}
	return result
}

// ftpSearchHandler 搜索真实持久化的 FTP 配置。
func ftpBaseInfo(ctx context.Context) map[string]any {
	exist := hostBinaryExists("pure-ftpd") || hostBinaryExists("pure-pw")
	return map[string]any{"isExist": exist, "isActive": exist && systemdUnitActive(ctx, "pure-ftpd", "pure-ftpd-mysql")}
}

func pureFTPDPasswdPath() string {
	if value := strings.TrimSpace(os.Getenv("WORKMESH_PURE_FTPD_PASSWD")); value != "" {
		return filepath.Clean(value)
	}
	return "/etc/pure-ftpd/pureftpd.passwd"
}

func listPureFTPUsers(keyword string) []map[string]any {
	content, err := os.ReadFile(pureFTPDPasswdPath())
	if err != nil {
		return []map[string]any{}
	}
	items := make([]map[string]any, 0)
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	for index, line := range strings.Split(string(content), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 6 || fields[0] == "" || strings.HasPrefix(fields[0], "#") {
			continue
		}
		userName := fields[0]
		if keyword != "" && !strings.Contains(strings.ToLower(userName), keyword) {
			continue
		}
		path := fields[5]
		items = append(items, map[string]any{"id": index + 1, "user": userName, "password": "", "status": "Enable", "path": path, "description": ""})
		if len(items) >= 500 {
			break
		}
	}
	return items
}

func ftpSearchHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := runtimeBody(r)
		_ = s
		if !ftpBaseInfo(r.Context())["isExist"].(bool) {
			runtimeOK(w, pageRecordsGeneric([]map[string]any{}, body))
			return
		}
		keyword := runtimeString(body, "info", "keyword", "user", "name")
		runtimeOK(w, pageRecordsGeneric(listPureFTPUsers(keyword), body))
	}
}

// ftpSaveHandler 新增或更新 FTP 配置并写入操作日志。
func ftpSaveHandler(s *runtimeStore, path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, err.Error())
			return
		}
		if !ftpBaseInfo(r.Context())["isExist"].(bool) {
			runtimeErr(w, http.StatusServiceUnavailable, "pure-ftpd 未安装")
			return
		}
		if !hostMutationAllowed() {
			runtimeErr(w, http.StatusServiceUnavailable, "修改 FTP 需要 WORKMESH_ALLOW_HOST_MUTATION=1")
			return
		}
		user := runtimeString(body, "user", "username")
		if !validHostUser(user) {
			runtimeErr(w, http.StatusBadRequest, "FTP 用户名无效")
			return
		}
		directory := filepath.Clean(runtimeString(body, "path"))
		if !filepath.IsAbs(directory) || strings.Contains(directory, "..") {
			runtimeErr(w, http.StatusBadRequest, "FTP 目录无效")
			return
		}
		if _, err := hostBinary("pure-pw"); err != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "pure-pw 未安装")
			return
		}
		password, err := decodePanelSecret(runtimeString(body, "password"))
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		args := []string{"useradd", user, "-u", "ftpuser", "-d", directory, "-f", pureFTPDPasswdPath(), "-m"}
		if strings.HasSuffix(path, "/update") {
			args = []string{"passwd", user, "-f", pureFTPDPasswdPath(), "-m"}
		}
		commandCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		command := hostExec(commandCtx, "pure-pw", args...)
		command.Stdin = strings.NewReader(password + "\n" + password + "\n")
		output, runErr := command.CombinedOutput()
		if runErr != nil {
			runtimeErr(w, http.StatusBadGateway, "执行 pure-pw 失败: "+trimCommandOutput(output))
			return
		}
		delete(body, "password")
		s.mu.Lock()
		appendFTPLog(s, "create_or_update", user, directory)
		_ = s.saveLocked()
		s.mu.Unlock()
		runtimeOK(w, map[string]any{"user": user, "path": directory, "status": "Enable"})
	}
}

// ftpDeleteHandler 删除 FTP 配置并记录删除事件。
func ftpDeleteHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := runtimeBody(r)
		if !ftpBaseInfo(r.Context())["isExist"].(bool) {
			runtimeErr(w, http.StatusServiceUnavailable, "pure-ftpd 未安装")
			return
		}
		if !hostMutationAllowed() {
			runtimeErr(w, http.StatusServiceUnavailable, "修改 FTP 需要 WORKMESH_ALLOW_HOST_MUTATION=1")
			return
		}
		names := ftpDeleteNames(body)
		if len(names) == 0 {
			runtimeErr(w, 400, "FTP 用户不能为空")
			return
		}
		if _, err := hostBinary("pure-pw"); err != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "pure-pw 未安装")
			return
		}
		for _, name := range names {
			if !validHostUser(name) {
				runtimeErr(w, http.StatusBadRequest, "FTP 用户名无效")
				return
			}
			if _, err := hostCommand(r.Context(), 15*time.Second, "pure-pw", "userdel", name, "-f", pureFTPDPasswdPath(), "-m"); err != nil {
				runtimeErr(w, http.StatusBadGateway, "删除 FTP 用户失败")
				return
			}
		}
		runtimeOK(w, map[string]any{"deleted": names})
	}
}

func ftpDeleteNames(body map[string]any) []string {
	names := make([]string, 0)
	if list, ok := body["ids"].([]any); ok {
		users := listPureFTPUsers("")
		for _, item := range list {
			id := int(0)
			switch value := item.(type) {
			case float64:
				id = int(value)
			case string:
				parsed, _ := strconv.Atoi(value)
				id = parsed
			}
			for _, user := range users {
				if user["id"] == id {
					names = append(names, fmt.Sprint(user["user"]))
				}
			}
		}
	}
	if name := runtimeString(body, "user", "username"); name != "" {
		names = append(names, name)
	}
	return names
}

// ftpOperateHandler 记录 FTP 客户端操作，不在服务端伪造连接结果。
func ftpOperateHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := runtimeBody(r)
		op := strings.ToLower(runtimeString(body, "operation", "operate"))
		if op != "start" && op != "stop" && op != "restart" {
			runtimeErr(w, 400, "FTP 操作无效")
			return
		}
		if !hostMutationAllowed() {
			runtimeErr(w, http.StatusServiceUnavailable, "修改 FTP 需要 WORKMESH_ALLOW_HOST_MUTATION=1")
			return
		}
		if !ftpBaseInfo(r.Context())["isExist"].(bool) {
			runtimeErr(w, http.StatusServiceUnavailable, "pure-ftpd 未安装")
			return
		}
		if _, err := hostBinary("systemctl"); err != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "systemctl 未安装")
			return
		}
		if _, err := hostCommand(r.Context(), 20*time.Second, "systemctl", op, "pure-ftpd"); err != nil {
			runtimeErr(w, http.StatusBadGateway, "执行 FTP 操作失败")
			return
		}
		s.mu.Lock()
		appendFTPLog(s, op, "", "pure-ftpd")
		_ = s.saveLocked()
		s.mu.Unlock()
		runtimeOK(w, map[string]any{"operation": op})
	}
}

// ftpSyncHandler 返回 FTP 同步任务排队结果。
func ftpSyncHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := runtimeBody(r)
		runtimeOK(w, map[string]any{"status": "queued", "id": runtimeString(body, "id", "ftpId")})
	}
}

// ftpLogSearchHandler 查询真实 FTP 操作日志。
func ftpLogSearchHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, "解析 FTP 日志查询失败: "+err.Error())
			return
		}
		keyword := strings.ToLower(runtimeString(body, "keyword", "action", "resourceId"))
		s.mu.RLock()
		stored, _ := s.state.Settings["ftp.logs"].([]any)
		logs := make([]map[string]any, 0, len(stored))
		for _, item := range stored {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if keyword != "" && !strings.Contains(strings.ToLower(fmt.Sprint(record["action"])+" "+fmt.Sprint(record["resourceId"])), keyword) {
				continue
			}
			logs = append(logs, cloneRuntimeMap(record))
		}
		s.mu.RUnlock()
		runtimeOK(w, pageRecordsGeneric(logs, body))
	}
}
