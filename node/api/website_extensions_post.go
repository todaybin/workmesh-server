// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
)

// websiteExtensionPostHandler 返回网站扩展统一 POST 分发处理器。
// 处理期间持有扩展状态锁，并在成功响应前按原顺序写入 SQLite。
func websiteExtensionPostHandler(store *websiteExtensionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/websites/"), "/")
		body, err := decodeExtension(r)
		if err != nil {
			extensionError(w, http.StatusBadRequest, err)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		previous := cloneWebsiteExtensionState(store)
		now := time.Now().UTC().Format(time.RFC3339)
		addLog := func(action string) {
			store.Logs = append(store.Logs, map[string]any{"id": store.id(), "action": action, "path": r.URL.Path, "createdAt": now})
			if len(store.Logs) > 2000 {
				store.Logs = store.Logs[len(store.Logs)-2000:]
			}
		}
		var result any
		switch {
		case rest == "acme/search" || rest == "ca/search":
			result = pageRecords(store.ACME, body)
		case rest == "acme/update":
			id := bodyID(body)
			if id == "" {
				extensionError(w, 400, errors.New("证书账户 ID 不能为空"))
				return
			}
			found := false
			for i, item := range store.ACME {
				if fmt.Sprint(item["id"]) == id {
					for k, v := range body {
						if k != "privateKey" {
							item[k] = v
						}
					}
					item["updatedAt"] = now
					store.ACME[i] = item
					result = item
					found = true
					break
				}
			}
			if !found {
				extensionError(w, 404, errors.New("证书账户不存在"))
				return
			}
		case rest == "acme/del" || rest == "ca/del":
			id := bodyID(body)
			if id == "" {
				extensionError(w, 400, errors.New("证书账户 ID 不能为空"))
				return
			}
			filtered := store.ACME[:0]
			for _, item := range store.ACME {
				if fmt.Sprint(item["id"]) != id {
					filtered = append(filtered, item)
				}
			}
			store.ACME = filtered
			result = map[string]any{"id": id, "deleted": true}
		case rest == "acme" || rest == "ca/obtain":
			email := bodyString(body, "email")
			if email == "" || !strings.Contains(email, "@") {
				extensionError(w, 400, errors.New("证书账户邮箱无效"))
				return
			}
			item := cloneRecord(body)
			item["id"] = store.id()
			item["email"] = email
			item["status"] = "pending"
			item["createdAt"] = now
			delete(item, "privateKey")
			store.ACME = append(store.ACME, item)
			result = item
		case rest == "ca/renew":
			result = map[string]any{"status": "queued", "message": "续期任务已记录，需配置 ACME 适配器后执行"}
		case rest == "ca/download":
			result = map[string]any{"status": "unavailable", "message": "证书下载需要已签发的本地证书"}
		case rest == "auths" || rest == "auths/update" || rest == "auths/path" || rest == "auths/path/update":
			id, idErr := parseID(bodyID(body))
			if idErr != nil || id == 0 {
				id = uint(bodyNumber(body, "websiteID", "websiteId"))
				if id == 0 {
					idErr = errors.New("网站 ID 无效")
				} else {
					idErr = nil
				}
			}
			if idErr != nil || id == 0 {
				extensionError(w, 400, errors.New("认证资源必须包含有效的网站 ID"))
				return
			}
			svc := service.NewWebsiteService("")
			switch rest {
			case "auths":
				// 1Panel 的历史契约允许同一路径承担查询和更新：只带
				// websiteID 时读取配置，带用户名/密码时写入 Basic Auth。
				if bodyString(body, "username", "user") == "" && bodyString(body, "password") == "" && bodyString(body, "operate", "action") == "" {
					result, err = svc.ListWebsiteAuths(id)
				} else {
					if bodyString(body, "username", "user") == "" || bodyString(body, "password") == "" {
						body["enabled"] = false
					}
					result, err = svc.UpdateConfig(id, "auths", body)
				}
			case "auths/path":
				// 路径认证查询通常只带 websiteID；提交 path/user/password
				// 时兼容旧前端直接调用该路径进行更新。
				if bodyString(body, "username", "user") == "" && bodyString(body, "password") == "" && bodyString(body, "operate", "action") == "" {
					result, err = svc.ListWebsitePathAuths(id)
				} else {
					if bodyString(body, "username", "user") == "" || bodyString(body, "password") == "" {
						body["enabled"] = false
					}
					result, err = svc.UpdateConfig(id, "path-auth", body)
				}
			default:
				typ := "auths"
				if strings.Contains(rest, "/path") {
					typ = "path-auth"
				}
				if bodyString(body, "username", "user") == "" || bodyString(body, "password") == "" {
					body["enabled"] = false
				}
				result, err = svc.UpdateConfig(id, typ, body)
			}
			if err != nil {
				extensionError(w, http.StatusBadRequest, err)
				return
			}
		case rest == "batch/group" || rest == "batch/operate" || rest == "batch/ssl" || rest == "group/change":
			svc := service.NewWebsiteService("")
			if rest == "batch/group" || rest == "group/change" {
				groupID := uint(bodyNumber(body, "groupID", "websiteGroupId"))
				ids := bodyIDs(body["ids"])
				if err := service.NewWebsiteService("").SetGroups(ids, groupID); err != nil {
					extensionError(w, 400, err)
					return
				}
				result = map[string]any{"ids": ids, "groupID": groupID, "updatedAt": now}
			} else if rest == "batch/operate" {
				operation := bodyString(body, "operate", "operation")
				if operation == "" {
					extensionError(w, 400, errors.New("网站操作不能为空"))
					return
				}
				ids := bodyIDs(body["ids"])
				updated := make([]any, 0, len(ids))
				for _, id := range ids {
					item, opErr := svc.Operate(id, operation)
					if opErr != nil {
						extensionError(w, 400, opErr)
						return
					}
					updated = append(updated, item)
				}
				result = map[string]any{"operation": operation, "items": updated, "updatedAt": now}
			} else {
				ids := bodyIDs(body["ids"])
				enabled, _ := body["enable"].(bool)
				if raw, exists := body["enabled"]; exists {
					// 兼容早期 WorkMesh 客户端的 enabled 字段；正式 1Panel
					// 契约使用 enable，优先读取调用方明确传入的字段。
					if value, ok := raw.(bool); ok {
						enabled = value
					}
				}
				for _, id := range ids {
					if _, opErr := svc.UpdateHTTPS(id, enabled, uint(bodyNumber(body, "websiteSSLId", "websiteSSLID")), bodyString(body, "httpConfig")); opErr != nil {
						extensionError(w, 400, opErr)
						return
					}
					if _, opErr := svc.UpdateConfig(id, "https", map[string]any{"enabled": enabled}); opErr != nil {
						extensionError(w, 400, opErr)
						return
					}
				}
				result = map[string]any{"enabled": enabled, "ids": ids, "updatedAt": now}
			}
		case rest == "crosssite":
			id, idErr := parseID(bodyID(body))
			operation := bodyString(body, "operation", "operate")
			if idErr != nil || id == 0 || operation == "" {
				extensionError(w, 400, errors.New("网站 ID 和跨站访问操作不能为空"))
				return
			}
			if err := service.NewWebsiteService("").OperateCrossSiteAccess(id, operation); err != nil {
				extensionError(w, 400, err)
				return
			}
			result = map[string]any{"websiteID": id, "operation": operation, "updatedAt": now}
		case rest == "databases":
			id, idErr := parseID(bodyID(body))
			if idErr != nil || id == 0 {
				extensionError(w, 400, errors.New("网站 ID 无效"))
				return
			}
			dbID := uint(bodyNumber(body, "databaseID", "dbID"))
			typ := bodyString(body, "databaseType", "dbType")
			svc := service.NewWebsiteService("")
			if dbID > 0 {
				found := false
				for _, database := range service.NewDatabaseService(nil).Search(r.Context(), "", "") {
					if uint(database.ID) == dbID {
						found = true
						if typ == "" {
							typ = database.Type
						}
						break
					}
				}
				if !found {
					extensionError(w, http.StatusNotFound, errors.New("数据库记录不存在"))
					return
				}
			}
			var dbid *uint = &dbID
			if dbID == 0 {
				dbid = new(uint)
			}
			if _, err := svc.Update(model.WebsiteUpdateRequest{ID: id, DbID: dbid, DbType: typ}); err != nil {
				extensionError(w, 400, err)
				return
			}
			result = map[string]any{"websiteID": id, "databaseID": dbID, "databaseType": typ}
		case rest == "exec/composer":
			path := bodyString(body, "path", "siteDir")
			if path == "" {
				extensionError(w, 400, errors.New("composer.json 路径不能为空"))
				return
			}
			info, statErr := os.Stat(filepath.Join(path, "composer.json"))
			if statErr != nil || info.IsDir() {
				extensionError(w, 404, errors.New("composer.json 文件不存在"))
				return
			}
			result = map[string]any{"path": filepath.Join(path, "composer.json"), "status": "validated", "executed": false}
		case rest == "log/search" || rest == "monitor/logs/search":
			if id, err := parseID(bodyID(body)); err == nil && id > 0 {
				logType := bodyString(body, "logType", "type")
				if logType == "" {
					logType = "access.log"
				}
				log, logErr := service.NewWebsiteService("").WebsiteLog(id, logType, int(bodyNumber(body, "page")), int(bodyNumber(body, "pageSize")))
				if logErr != nil {
					status := http.StatusBadRequest
					if errors.Is(logErr, os.ErrNotExist) {
						status = http.StatusNotFound
					}
					extensionError(w, status, logErr)
					return
				}
				result = log
				break
			}
			extensionError(w, http.StatusBadRequest, errors.New("网站日志查询必须包含有效的网站 ID"))
			return
		case rest == "log/operate":
			id, idErr := parseID(bodyID(body))
			logType := bodyString(body, "logType", "type")
			if idErr == nil && id > 0 && logType != "" {
				if err := service.NewWebsiteService("").OperateWebsiteLog(id, logType, bodyString(body, "operate", "action")); err != nil {
					extensionError(w, 400, err)
					return
				}
				result = map[string]any{"updated": true}
			} else {
				addLog(bodyString(body, "operate", "action"))
				result = map[string]any{"updated": true}
			}
		case rest == "monitor/logs/clear":
			store.Logs = []map[string]any{}
			result = map[string]any{"cleared": true}
		case rest == "monitor/logs/detail":
			if len(store.Logs) == 0 {
				result = map[string]any{"found": false}
			} else {
				result = cloneRecord(store.Logs[len(store.Logs)-1])
			}
		case rest == "monitor/logs/stat":
			result = map[string]any{"total": len(store.Logs), "source": "local"}
		case rest == "php/version":
			id, idErr := parseID(bodyID(body))
			if idErr != nil || id == 0 {
				extensionError(w, 400, errors.New("网站 ID 不能为空"))
				return
			}
			body["version"] = bodyString(body, "version", "phpVersion")
			cfg, cfgErr := service.NewWebsiteService("").UpdateConfig(id, "php", body)
			if cfgErr != nil {
				extensionError(w, 400, cfgErr)
				return
			}
			result = cfg
		case rest == "proxies" || rest == "proxies/update" || rest == "proxies/status" || rest == "proxies/file":
			id, idErr := parseID(bodyID(body))
			if idErr != nil || id == 0 {
				extensionError(w, http.StatusBadRequest, errors.New("网站代理 ID 不能为空"))
				return
			}
			svc := service.NewWebsiteService("")
			if rest == "proxies" {
				result, err = svc.ListWebsiteProxies(id)
			} else if rest == "proxies/file" {
				fileName := bodyString(body, "name", "fileName", "filePath")
				content := bodyString(body, "content")
				result, err = svc.UpdateWebsiteProxyFile(id, fileName, content)
			} else {
				if rest == "proxies/status" {
					status := strings.ToLower(bodyString(body, "status", "operate"))
					// 状态接口的前端契约只提交 id/name/status；合并已保存的
					// 代理配置，避免状态切换要求调用方重复提交 proxyPass。
					current, currentErr := svc.GetConfig(id, "proxy")
					if currentErr != nil {
						extensionError(w, http.StatusBadRequest, currentErr)
						return
					}
					for key, value := range current {
						if _, exists := body[key]; !exists {
							body[key] = value
						}
					}
					body["enabled"] = status == "enable" || status == "enabled" || status == "running" || status == "on"
				}
				result, err = svc.UpdateWebsiteProxy(id, body)
			}
			if err != nil {
				extensionError(w, http.StatusBadRequest, err)
				return
			}
		case rest == "proxies/delete":
			id, idErr := parseID(bodyID(body))
			if idErr != nil || id == 0 {
				extensionError(w, http.StatusBadRequest, errors.New("网站代理 ID 不能为空"))
				return
			}
			if err := service.NewWebsiteService("").DeleteWebsiteProxy(id, bodyString(body, "name", "fileName", "filePath")); err != nil {
				extensionError(w, http.StatusBadRequest, err)
				return
			}
			result = map[string]any{"id": id, "deleted": true}
		case rest == "waf/attack/stat" || rest == "waf/block/search" || rest == "waf/log/search" || rest == "waf/relation/stat":
			result = pageRecords(store.Logs, body)
		default:
			extensionError(w, http.StatusNotFound, errors.New("网站扩展接口不存在"))
			return
		}
		if err := store.persistLocked(); err != nil {
			store.ACME = previous.ACME
			store.Templates = previous.Templates
			store.Outputs = previous.Outputs
			store.Proxies = previous.Proxies
			store.Auths = previous.Auths
			store.Databases = previous.Databases
			store.Logs = previous.Logs
			store.NextID = previous.NextID
			extensionError(w, 500, fmt.Errorf("保存网站扩展状态失败: %w", err))
			return
		}
		if rest == "log/search" || rest == "monitor/logs/search" {
			if payload, ok := result.(map[string]any); ok {
				result = redactWebsiteLogResult(payload)
			}
		}
		extensionJSON(w, result)
	}
}
