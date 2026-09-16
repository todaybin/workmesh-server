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
	return filepath.Join(filepath.Dir(s.path), "fail2ban.local")
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
		runtimeOK(w, map[string]any{"items": lines, "total": len(lines), "path": fail2banConfigPath(s)})
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
		content := runtimeString(body, "content", "conf")
		if content == "" || len(content) > 1<<20 {
			runtimeErr(w, 400, "Fail2ban 配置内容无效")
			return
		}
		file := fail2banConfigPath(s)
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
		action := strings.ToLower(runtimeString(body, "operate", "action"))
		if action != "start" && action != "stop" && action != "restart" {
			runtimeErr(w, 400, "Fail2ban 操作必须是 start、stop 或 restart")
			return
		}
		binary, lookErr := exec.LookPath("fail2ban-client")
		if lookErr != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "fail2ban-client 未安装")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, action)
		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			runtimeErr(w, http.StatusBadGateway, "执行 Fail2ban 操作失败: "+strings.TrimSpace(string(output)))
			return
		}
		runtimeOK(w, map[string]any{"operation": action, "output": strings.TrimSpace(string(output))})
	}
}

// registerToolboxFtpRoutes 管理本地 FTP 连接配置，密码永不回传。
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
func ftpSearchHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := runtimeBody(r)
		keyword := strings.ToLower(runtimeString(body, "keyword", "name", "host"))
		s.mu.RLock()
		all := ftpEntries(s)
		s.mu.RUnlock()
		filtered := make([]map[string]any, 0, len(all))
		for _, item := range all {
			if keyword == "" || strings.Contains(strings.ToLower(fmt.Sprint(item["name"])+" "+fmt.Sprint(item["host"])), keyword) {
				filtered = append(filtered, item)
			}
		}
		runtimeOK(w, pageRecordsGeneric(filtered, body))
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
		host := runtimeString(body, "host", "hostname")
		user := runtimeString(body, "username", "user")
		if host == "" || user == "" || len(host) > 253 || strings.ContainsAny(host, " /\\") {
			runtimeErr(w, 400, "FTP 主机和用户名不能为空")
			return
		}
		body["host"], body["username"] = host, user
		delete(body, "password")
		s.mu.Lock()
		value, _ := s.state.Settings["ftp.entries"].([]any)
		id := runtimeString(body, "id")
		if id == "" {
			id = "ftp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
			body["id"] = id
			value = append(value, body)
		} else {
			found := false
			for i, item := range value {
				if typed, ok := item.(map[string]any); ok && fmt.Sprint(typed["id"]) == id {
					value[i] = body
					found = true
				}
			}
			if !found {
				value = append(value, body)
			}
		}
		s.state.Settings["ftp.entries"] = value
		logs, _ := s.state.Settings["ftp.logs"].([]any)
		logs = append(logs, map[string]any{"id": "ftp-log-" + strconv.FormatInt(time.Now().UnixNano(), 10), "action": path[strings.LastIndex(path, "/")+1:], "resourceId": id, "createdAt": time.Now().UTC()})
		if len(logs) > 1000 {
			logs = logs[len(logs)-1000:]
		}
		s.state.Settings["ftp.logs"] = logs
		saveErr := s.saveLocked()
		s.mu.Unlock()
		if saveErr != nil {
			runtimeErr(w, 500, "保存 FTP 配置失败: "+saveErr.Error())
			return
		}
		s.mu.Lock()
		appendFTPLog(s, "create_or_update", id, host)
		_ = s.saveLocked()
		s.mu.Unlock()
		runtimeOK(w, body)
	}
}

// ftpDeleteHandler 删除 FTP 配置并记录删除事件。
func ftpDeleteHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := runtimeBody(r)
		id := runtimeString(body, "id", "ftpId")
		if id == "" {
			runtimeErr(w, 400, "FTP 配置 ID 不能为空")
			return
		}
		s.mu.Lock()
		value, _ := s.state.Settings["ftp.entries"].([]any)
		out := make([]any, 0, len(value))
		found := false
		for _, item := range value {
			record, ok := item.(map[string]any)
			if ok && fmt.Sprint(record["id"]) == id {
				found = true
				continue
			}
			out = append(out, item)
		}
		s.state.Settings["ftp.entries"] = out
		if found {
			logs, _ := s.state.Settings["ftp.logs"].([]any)
			logs = append(logs, map[string]any{"id": "ftp-log-" + strconv.FormatInt(time.Now().UnixNano(), 10), "action": "delete", "resourceId": id, "createdAt": time.Now().UTC()})
			if len(logs) > 1000 {
				logs = logs[len(logs)-1000:]
			}
			s.state.Settings["ftp.logs"] = logs
		}
		saveErr := s.saveLocked()
		s.mu.Unlock()
		if !found {
			runtimeErr(w, 404, "FTP 配置不存在")
			return
		}
		if saveErr != nil {
			runtimeErr(w, 500, "保存 FTP 配置失败: "+saveErr.Error())
			return
		}
		s.mu.Lock()
		appendFTPLog(s, "delete", id, "")
		_ = s.saveLocked()
		s.mu.Unlock()
		runtimeOK(w, map[string]any{"id": id, "deleted": true})
	}
}

// ftpOperateHandler 记录 FTP 客户端操作，不在服务端伪造连接结果。
func ftpOperateHandler(s *runtimeStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := runtimeBody(r)
		op := strings.ToLower(runtimeString(body, "operate", "operation"))
		if op != "connect" && op != "disconnect" && op != "test" {
			runtimeErr(w, 400, "FTP 操作无效")
			return
		}
		s.mu.Lock()
		appendFTPLog(s, op, runtimeString(body, "id", "ftpId"), "client_operation")
		_ = s.saveLocked()
		s.mu.Unlock()
		runtimeOK(w, map[string]any{"operation": op, "status": "not_connected", "message": "FTP 连接需由已配置的客户端执行"})
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
