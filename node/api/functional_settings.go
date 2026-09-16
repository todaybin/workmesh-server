// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// registerSettingsRoutes 注册全局设置、终端设置、SSL 设置和设置快照的读写接口。
func registerSettingsRoutes(mux *http.ServeMux, s *domainStore) {
	// 默认字段与前端 SettingInfo/SettingBaseInfo 契约保持一致；状态文件中已有值会覆盖默认值。
	defaults := map[string]any{
		"dockerSockPath": "unix:///var/run/docker.sock", "systemIP": "", "localTime": "", "timeZone": "", "ntpSite": "",
		"defaultNetwork": "all", "defaultIO": "all", "lastCleanTime": "", "lastCleanSize": "", "lastCleanData": "",
		"monitorStatus": "enable", "monitorInterval": "10", "monitorStoreDays": "7", "fileRecycleBin": "disable", "localSSHConnShow": "disable", "firewallPortWhiteList": "",
		"systemVersion": "workmesh-server", "upgradeBackupCopies": "3", "developerMode": "false",
		"sessionTimeout": 86400, "expirationDays": 0, "panelName": "WorkMesh", "edition": "community",
		"theme": "system", "menuTabs": "false", "menuAccordion": "false", "language": "zh", "docSource": "official",
		"serverPort": 9999, "port": "9999", "ipv6": "disable", "bindAddress": "0.0.0.0", "ssl": "disable", "sslType": "self",
		"allowIPs": "", "allowIPTrustedProxies": "", "bindDomain": "", "passkeyTrustedProxies": "", "securityEntrance": "",
		"dashboardMemoVisible": "Enable", "dashboardSimpleNodeVisible": "Enable", "complexityVerification": "false", "messageType": "system",
		"emailVars": "", "weChatVars": "", "dingVars": "", "snapshotIgnore": "", "hideMenu": "", "noAuthSetting": "",
		"proxyUrl": "", "proxyType": "", "proxyPort": "", "proxyUser": "", "proxyPasswd": "", "proxyPasswdKeep": "",
		"scriptSync": "false", "lineHeight": "1.5", "letterSpacing": "0", "fontSize": "14", "fontFamily": "monospace",
		"backgroundColor": "#1e1e1e", "foregroundColor": "#d4d4d4", "cursorBlink": "true", "cursorStyle": "block", "scrollback": "1000", "scrollSensitivity": "1",
		"aiStatus": "disable", "aiAccountId": "", "aiPrefix": "", "aiRiskCommands": "",
		"appStoreVersion": "", "appStoreLastModified": "", "appStoreSyncStatus": "ready", "memo": "",
	}
	s.mu.Lock()
	if s.state.Settings == nil {
		s.state.Settings = map[string]any{}
	}
	for key, value := range defaults {
		if _, exists := s.state.Settings[key]; !exists {
			s.state.Settings[key] = value
		}
	}
	s.mu.Unlock()
	get := func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		copy := map[string]any{}
		for k, v := range s.state.Settings {
			copy[k] = v
		}
		s.mu.RUnlock()
		switch r.URL.Path {
		case "/api/v2/core/settings/search/available", "/api/v2/settings/search/available":
			success(w, map[string]any{"available": true})
		case "/api/v2/settings/basedir":
			dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
			if dir == "" {
				dir = "./data"
			}
			success(w, map[string]any{"baseDir": dir, "path": dir})
		case "/api/v2/settings/website/dir":
			dir := strings.TrimSpace(os.Getenv("PANEL_WEBSITE_DIR"))
			if dir == "" {
				dir = "/www/wwwroot"
			}
			success(w, dir)
		case "/api/v2/settings/daemonjson":
			// The panel uses this endpoint to display the effective Docker
			// daemon.json location.  Keep it tied to the same path resolver as
			// the container daemon-json read/update handlers.
			success(w, daemonJSONPath())
		case "/api/v2/settings/snapshot/load":
			s.mu.RLock()
			items := append([]settingSnapshot(nil), s.state.Snapshots...)
			s.mu.RUnlock()
			success(w, map[string]any{"items": items, "total": len(items)})
		case "/api/v2/core/settings/interface":
			success(w, []string{"127.0.0.1", "0.0.0.0"})
		case "/api/v2/core/settings/apps/store/config":
			success(w, map[string]any{"version": copy["appStoreVersion"], "lastModified": copy["appStoreLastModified"], "syncStatus": copy["appStoreSyncStatus"]})
		case "/api/v2/core/settings/ssl/info":
			success(w, map[string]any{"domain": copy["bindDomain"], "timeout": "", "rootPath": "", "cert": "", "key": "", "sslID": 0})
		case "/api/v2/core/settings/upgrade":
			success(w, map[string]any{"testVersion": "", "newVersion": "", "latestVersion": "", "releaseNote": ""})
		case "/api/v2/core/settings/upgrade/releases":
			// 当前无远端发布源时返回可迭代的空结果，并保留同步状态字段。
			success(w, map[string]any{"items": make([]map[string]any, 0), "total": 0, "source": "unconfigured"})
		case "/api/v2/core/settings/memo":
			success(w, copy["memo"])
		default:
			success(w, copy)
		}
	}
	for _, path := range []string{"/api/v2/config/global", "/api/v2/core/settings/interface", "/api/v2/core/settings/apps/store/config", "/api/v2/core/settings/search/available", "/api/v2/core/settings/ssl/info", "/api/v2/core/settings/upgrade", "/api/v2/core/settings/upgrade/releases", "/api/v2/core/settings/memo", "/api/v2/settings/basedir", "/api/v2/settings/search/available", "/api/v2/settings/snapshot/load", "/api/v2/settings/website/dir", "/api/v2/settings/daemonjson"} {
		mux.HandleFunc("GET "+path, get)
	}
	update := func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		s.mu.Lock()
		previous := cloneDomainState(s.state)
		if s.state.Settings == nil {
			s.state.Settings = map[string]any{}
		}
		apply := func(key string, val any) error {
			key = settingJSONKey(strings.TrimSpace(key))
			if key == "" {
				return nil
			}
			normalized, normalizeErr := normalizeSettingValue(key, val)
			if normalizeErr != nil {
				return normalizeErr
			}
			s.state.Settings[key] = normalized
			return nil
		}
		// SettingUpdate 使用 key/value 包装；其余批量更新则直接合并字段。
		if key := valueString(v, "key"); key != "" {
			val, exists := v["value"]
			if !exists {
				s.mu.Unlock()
				domainError(w, http.StatusBadRequest, "INVALID_SETTING", "设置 value 不能为空")
				return
			}
			if err = apply(key, val); err != nil {
				s.mu.Unlock()
				domainError(w, http.StatusBadRequest, "INVALID_SETTING", err.Error())
				return
			}
		} else if content, exists := v["content"]; exists && r.URL.Path == "/api/v2/core/settings/memo" {
			s.state.Settings["memo"] = content
		} else {
			for k, val := range v {
				if err = apply(k, val); err != nil {
					s.mu.Unlock()
					domainError(w, http.StatusBadRequest, "INVALID_SETTING", err.Error())
					return
				}
			}
		}
		err = s.saveLocked()
		if err != nil {
			s.state = previous
			s.mu.Unlock()
			domainError(w, 500, "STATE_SAVE", err.Error())
			return
		}
		copy := map[string]any{}
		for k, val := range s.state.Settings {
			copy[k] = val
		}
		s.mu.Unlock()
		if r.URL.Path == "/api/v2/core/settings/memo" {
			success(w, nil)
			return
		}
		success(w, copy)
	}
	mux.HandleFunc("POST /api/v2/settings/description/save", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		typ, id := valueString(v, "type"), valueString(v, "id")
		if typ == "" || id == "" {
			domainError(w, 400, "INVALID_DESCRIPTION", "资源类型和 ID 不能为空")
			return
		}
		s.mu.Lock()
		previous := cloneDomainState(s.state)
		if s.state.Settings == nil {
			s.state.Settings = map[string]any{}
		}
		descriptions, _ := s.state.Settings["descriptions"].(map[string]any)
		if descriptions == nil {
			descriptions = map[string]any{}
		}
		descriptions[typ+":"+id] = map[string]any{"description": valueString(v, "description"), "isPinned": boolValue(v, "isPinned")}
		s.state.Settings["descriptions"] = descriptions
		err = s.saveLocked()
		if err != nil {
			s.state = previous
			s.mu.Unlock()
			domainError(w, 500, "STATE_SAVE", err.Error())
			return
		}
		s.mu.Unlock()
		success(w, nil)
	})
	for _, path := range []string{"/api/v2/config/global", "/api/v2/core/settings/apps/store/update", "/api/v2/core/settings/menu/update", "/api/v2/core/settings/proxy/update", "/api/v2/core/settings/search", "/api/v2/core/settings/search/base", "/api/v2/core/settings/upgrade", "/api/v2/core/settings/upgrade/notes", "/api/v2/core/settings/memo", "/api/v2/core/settings/update", "/api/v2/settings/file-history/search", "/api/v2/settings/file-history/update", "/api/v2/settings/files/ai/search", "/api/v2/settings/files/ai/update", "/api/v2/settings/search", "/api/v2/settings/update"} {
		mux.HandleFunc("POST "+path, update)
	}
	mux.HandleFunc("POST /api/v2/core/settings/port/update", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		port := intValue(v, "serverPort")
		if port < 1 || port > 65535 {
			domainError(w, http.StatusBadRequest, "INVALID_SETTING", "serverPort 必须在 1 到 65535 之间")
			return
		}
		s.mu.Lock()
		previous := cloneDomainState(s.state)
		s.state.Settings["serverPort"] = port
		s.state.Settings["port"] = strconv.Itoa(port)
		if err := s.saveLocked(); err != nil {
			s.state = previous
			s.mu.Unlock()
			domainError(w, http.StatusInternalServerError, "STATE_SAVE", err.Error())
			return
		}
		s.mu.Unlock()
		success(w, map[string]any{"serverPort": port, "effective": false, "restartRequired": true, "status": "pending-restart"})
	})
	mux.HandleFunc("POST /api/v2/core/settings/bind/update", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		ipv6, bindAddress := valueString(v, "ipv6"), strings.TrimSpace(valueString(v, "bindAddress"))
		toggle, err := normalizeSettingToggle(ipv6)
		if err != nil {
			domainError(w, http.StatusBadRequest, "INVALID_SETTING", "ipv6 无效: "+err.Error())
			return
		}
		parsed := net.ParseIP(bindAddress)
		if parsed == nil || (toggle == "enable" && parsed.To4() != nil) || (toggle == "disable" && parsed.To4() == nil) {
			domainError(w, http.StatusBadRequest, "INVALID_SETTING", "bindAddress 与 ipv6 模式不匹配")
			return
		}
		s.mu.Lock()
		previous := cloneDomainState(s.state)
		s.state.Settings["ipv6"], s.state.Settings["bindAddress"] = toggle, bindAddress
		if err := s.saveLocked(); err != nil {
			s.state = previous
			s.mu.Unlock()
			domainError(w, http.StatusInternalServerError, "STATE_SAVE", err.Error())
			return
		}
		s.mu.Unlock()
		success(w, map[string]any{"ipv6": toggle, "bindAddress": bindAddress, "effective": false, "restartRequired": true, "status": "pending-restart"})
	})
	mux.HandleFunc("POST /api/v2/core/settings/ssl/update", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		mode, err := normalizePanelSSLMode(valueString(v, "ssl"))
		if err != nil {
			domainError(w, http.StatusBadRequest, "INVALID_SETTING", err.Error())
			return
		}
		sslType := strings.ToLower(strings.TrimSpace(valueString(v, "sslType")))
		cert, key := valueString(v, "cert"), valueString(v, "key")
		if mode != "disable" {
			if err := validatePanelSSLCredentials(sslType, cert, key); err != nil {
				domainError(w, http.StatusBadRequest, "INVALID_SSL", err.Error())
				return
			}
		}
		s.mu.Lock()
		previous := cloneDomainState(s.state)
		s.state.Settings["ssl"] = mode
		s.state.Settings["sslType"] = sslType
		s.state.Settings["domain"] = valueString(v, "domain")
		s.state.Settings["cert"] = cert
		s.state.Settings["key"] = key
		s.state.Settings["sslID"] = intValue(v, "sslID")
		if err := s.saveLocked(); err != nil {
			s.state = previous
			s.mu.Unlock()
			domainError(w, http.StatusInternalServerError, "STATE_SAVE", err.Error())
			return
		}
		s.mu.Unlock()
		success(w, map[string]any{"ssl": mode, "effective": false, "reloadRequired": true, "status": "pending-reload"})
	})
	settingsOperational := func(endpoint string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			s.mu.Lock()
			if s.state.Settings == nil {
				s.state.Settings = map[string]any{}
			}
			result := map[string]any{"path": endpoint, "status": "ready", "updatedAt": time.Now().UTC().Format(time.RFC3339)}
			switch endpoint {
			case "/api/v2/core/settings/menu/default":
				// 菜单默认值由持久化配置覆盖，未配置时返回完整的基础菜单标识。
				menu, ok := s.state.Settings["menu.default"]
				if !ok {
					menu = []string{"dashboard", "applications", "websites", "databases", "containers", "files", "terminal", "settings"}
				}
				result["items"] = menu
			case "/api/v2/core/settings/terminal/search":
				var term map[string]any
				if !loadNodeSetting("terminal", &term) || term == nil {
					term = map[string]any{}
				}
				result["config"] = term
			case "/api/v2/core/settings/ssl/download":
				result["config"] = s.state.Settings["ssl"]
			case "/api/v2/core/settings/ssl/reload":
				if err := runConfiguredSSLReload(r.Context()); err != nil {
					s.mu.Unlock()
					domainError(w, http.StatusServiceUnavailable, "SSL_RELOAD_UNAVAILABLE", err.Error())
					return
				}
				previous := cloneDomainState(s.state)
				s.state.Settings["ssl.lastReloadAt"] = result["updatedAt"]
				result["reloaded"] = true
				result["effective"] = true
				if err := s.saveLocked(); err != nil {
					s.state = previous
					s.mu.Unlock()
					domainError(w, 500, "STATE_SAVE", err.Error())
					return
				}
			}
			s.mu.Unlock()
			success(w, result)
		}
	}
	// 使用显式路由注册，确保契约扫描和运行时注册保持一一对应。
	mux.HandleFunc("POST /api/v2/core/settings/menu/default", settingsOperational("/api/v2/core/settings/menu/default"))
	mux.HandleFunc("POST /api/v2/core/settings/terminal/search", settingsOperational("/api/v2/core/settings/terminal/search"))
	mux.HandleFunc("POST /api/v2/core/settings/terminal/update", func(w http.ResponseWriter, r *http.Request) {
		value, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		if err := saveNodeSetting("terminal", value); err != nil {
			domainError(w, 500, "STATE_SAVE", err.Error())
			return
		}
		success(w, map[string]any{"config": value})
	})
	mux.HandleFunc("POST /api/v2/core/settings/ssl/download", settingsOperational("/api/v2/core/settings/ssl/download"))
	mux.HandleFunc("POST /api/v2/core/settings/ssl/reload", settingsOperational("/api/v2/core/settings/ssl/reload"))
	// Agent 侧设置快照使用同一份轻量状态文件，支持创建、查询、导入、恢复、回滚和删除。
	createSnapshot := func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		now := time.Now().UTC()
		s.mu.Lock()
		previous := cloneDomainState(s.state)
		data := map[string]any{}
		for k, value := range s.state.Settings {
			data[k] = value
		}
		item := settingSnapshot{ID: idToken(), Name: valueString(v, "name", "snapshotName"), Description: valueString(v, "description"), Data: data, CreatedAt: now}
		if item.Name == "" {
			item.Name = "snapshot-" + now.Format("20060102-150405")
		}
		s.state.Snapshots = append(s.state.Snapshots, item)
		err = s.saveLocked()
		if err != nil {
			s.state = previous
			s.mu.Unlock()
			domainError(w, 500, "STATE_SAVE", err.Error())
			return
		}
		s.mu.Unlock()
		success(w, item)
	}
	mux.HandleFunc("POST /api/v2/settings/snapshot", createSnapshot)
	mux.HandleFunc("POST /api/v2/settings/snapshot/recreate", createSnapshot)
	mux.HandleFunc("POST /api/v2/settings/snapshot/search", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := append([]settingSnapshot(nil), s.state.Snapshots...)
		s.mu.RUnlock()
		success(w, map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": 50})
	})
	mux.HandleFunc("POST /api/v2/settings/snapshot/import", func(w http.ResponseWriter, r *http.Request) {
		v, err := requestMap(r)
		if err != nil {
			domainError(w, 400, "INVALID_JSON", err.Error())
			return
		}
		data, _ := v["data"].(map[string]any)
		if data == nil {
			data = map[string]any{}
		}
		now := time.Now().UTC()
		item := settingSnapshot{ID: idToken(), Name: valueString(v, "name"), Description: valueString(v, "description"), Data: data, CreatedAt: now}
		if item.Name == "" {
			item.Name = "imported-" + now.Format("20060102-150405")
		}
		s.mu.Lock()
		previous := cloneDomainState(s.state)
		s.state.Snapshots = append(s.state.Snapshots, item)
		err = s.saveLocked()
		if err != nil {
			s.state = previous
			s.mu.Unlock()
			domainError(w, 500, "STATE_SAVE", err.Error())
			return
		}
		s.mu.Unlock()
		success(w, item)
	})
	mux.HandleFunc("POST /api/v2/settings/snapshot/del", func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id", "snapshotId")
		s.mu.Lock()
		for i, item := range s.state.Snapshots {
			if item.ID == id {
				previous := cloneDomainState(s.state)
				s.state.Snapshots = append(s.state.Snapshots[:i], s.state.Snapshots[i+1:]...)
				if err := s.saveLocked(); err != nil {
					s.state = previous
					s.mu.Unlock()
					domainError(w, 500, "STATE_SAVE", err.Error())
					return
				}
				s.mu.Unlock()
				success(w, nil)
				return
			}
		}
		s.mu.Unlock()
		domainError(w, 404, "NOT_FOUND", "设置快照不存在")
	})
	recoverSnapshot := func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id", "snapshotId")
		s.mu.Lock()
		for _, item := range s.state.Snapshots {
			if item.ID == id {
				previous := cloneDomainState(s.state)
				s.state.Settings = map[string]any{}
				for k, value := range item.Data {
					s.state.Settings[k] = value
				}
				if err := s.saveLocked(); err != nil {
					s.state = previous
					s.mu.Unlock()
					domainError(w, 500, "STATE_SAVE", err.Error())
					return
				}
				s.mu.Unlock()
				success(w, item)
				return
			}
		}
		s.mu.Unlock()
		domainError(w, 404, "NOT_FOUND", "设置快照不存在")
	}
	for _, path := range []string{"/api/v2/settings/snapshot/recover", "/api/v2/settings/snapshot/rollback"} {
		mux.HandleFunc("POST "+path, recoverSnapshot)
	}
	mux.HandleFunc("POST /api/v2/settings/snapshot/description/update", func(w http.ResponseWriter, r *http.Request) {
		v, _ := requestMap(r)
		id := valueString(v, "id", "snapshotId")
		description := valueString(v, "description")
		s.mu.Lock()
		for i := range s.state.Snapshots {
			if s.state.Snapshots[i].ID == id {
				previous := cloneDomainState(s.state)
				s.state.Snapshots[i].Description = description
				if err := s.saveLocked(); err != nil {
					s.state = previous
					s.mu.Unlock()
					domainError(w, 500, "STATE_SAVE", err.Error())
					return
				}
				updated := s.state.Snapshots[i]
				s.mu.Unlock()
				success(w, updated)
				return
			}
		}
		s.mu.Unlock()
		domainError(w, 404, "NOT_FOUND", "设置快照不存在")
	})
}

// settingJSONKey 将旧接口的 PascalCase 配置键转换为前端使用的 lowerCamelCase。
func settingJSONKey(key string) string {
	if key == "" {
		return key
	}
	return strings.ToLower(key[:1]) + key[1:]
}

func normalizePanelSSLMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "disable", "disabled", "off":
		return "disable", nil
	case "enable", "enabled", "strict", "on":
		return "enable", nil
	case "mux":
		return "mux", nil
	default:
		return "", fmt.Errorf("ssl 模式不受支持: %s", value)
	}
}

func validatePanelSSLCredentials(sslType, cert, key string) error {
	if sslType != "import-paste" && sslType != "import-local" && sslType != "import" {
		return nil
	}
	certPEM, keyPEM := []byte(cert), []byte(key)
	if sslType == "import-local" {
		var err error
		certPEM, err = os.ReadFile(cert)
		if err != nil {
			return fmt.Errorf("读取证书文件失败: %w", err)
		}
		keyPEM, err = os.ReadFile(key)
		if err != nil {
			return fmt.Errorf("读取私钥文件失败: %w", err)
		}
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return fmt.Errorf("证书和私钥不能为空")
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return fmt.Errorf("证书与私钥不匹配或格式无效: %w", err)
	}
	return nil
}

func runConfiguredSSLReload(ctx context.Context) error {
	command := strings.TrimSpace(os.Getenv("WORKMESH_SSL_RELOAD_COMMAND"))
	if command == "" {
		return fmt.Errorf("未配置 WORKMESH_SSL_RELOAD_COMMAND，未执行 SSL reload")
	}
	reloadCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(reloadCtx, "/bin/sh", "-c", command).CombinedOutput(); err != nil {
		output = []byte(strings.TrimSpace(string(output)))
		if len(output) > 512 {
			output = output[:512]
		}
		return fmt.Errorf("SSL reload hook 失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
