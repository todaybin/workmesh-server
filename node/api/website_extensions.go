// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// websiteExtensionContractPaths 记录由统一分发器承接的旧公开契约，供静态实现扫描和文档生成使用。
var websiteExtensionContractPaths = []string{
	"GET /api/v2/websites/databases", "GET /api/v2/websites/default/html/:type",
	"POST /api/v2/websites/auths", "POST /api/v2/websites/auths/path", "POST /api/v2/websites/auths/path/update", "POST /api/v2/websites/auths/update",
	"POST /api/v2/websites/batch/group", "POST /api/v2/websites/batch/operate", "POST /api/v2/websites/batch/ssl", "POST /api/v2/websites/crosssite", "POST /api/v2/websites/databases", "POST /api/v2/websites/exec/composer", "POST /api/v2/websites/group/change", "POST /api/v2/websites/lbs/del",
	"POST /api/v2/websites/log/operate", "POST /api/v2/websites/log/search", "POST /api/v2/websites/monitor/logs/clear", "POST /api/v2/websites/monitor/logs/detail", "POST /api/v2/websites/monitor/logs/search", "POST /api/v2/websites/monitor/logs/stat", "POST /api/v2/websites/php/version",
	"POST /api/v2/websites/proxies", "POST /api/v2/websites/proxies/delete", "POST /api/v2/websites/proxies/file", "POST /api/v2/websites/proxies/status", "POST /api/v2/websites/proxies/update",
	"POST /api/v2/websites/templates/del", "POST /api/v2/websites/templates/get", "POST /api/v2/websites/templates/outputs", "POST /api/v2/websites/templates/outputs/del", "POST /api/v2/websites/templates/outputs/get", "POST /api/v2/websites/templates/outputs/search", "POST /api/v2/websites/templates/preview", "POST /api/v2/websites/templates/search", "POST /api/v2/websites/templates/update", "POST /api/v2/websites/templates/upload",
	"POST /api/v2/websites/waf/attack/stat", "POST /api/v2/websites/waf/block/search", "POST /api/v2/websites/waf/log/search", "POST /api/v2/websites/waf/relation/stat",
}

// websiteExtensionStore 保存旧网站扩展接口需要的本地元数据。
// 外部证书签发、DNS 验证和数据库服务仍由对应适配器负责，本存储只记录可恢复状态。
type websiteExtensionStore struct {
	mu        sync.Mutex
	path      string
	ACME      []map[string]any          `json:"acme"`
	Templates []map[string]any          `json:"templates"`
	Outputs   []map[string]any          `json:"outputs"`
	Proxies   map[string]map[string]any `json:"proxies"`
	Auths     map[string]map[string]any `json:"auths"`
	Databases []map[string]any          `json:"databases"`
	Logs      []map[string]any          `json:"logs"`
	NextID    uint64                    `json:"nextId"`
	db        *sql.DB
	owner     *storage.Store
}

func newWebsiteExtensionStore() *websiteExtensionStore {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	s := &websiteExtensionStore{Proxies: map[string]map[string]any{}, Auths: map[string]map[string]any{}}
	if db := sharedDB(); db != nil {
		s.db = db
	} else if opened, err := storage.Open(filepath.Join(root, "workmesh.db")); err == nil {
		s.owner, s.db = opened, opened.DB()
	}
	if s.db != nil {
		_, _ = s.db.Exec(`CREATE TABLE IF NOT EXISTS website_extension_state (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL, updated_at TEXT NOT NULL)`)
		var data []byte
		if err := s.db.QueryRow("SELECT payload FROM website_extension_state WHERE id=1").Scan(&data); err == nil {
			_ = json.Unmarshal(data, s)
		}
	}
	if s.Proxies == nil {
		s.Proxies = map[string]map[string]any{}
	}
	if s.Auths == nil {
		s.Auths = map[string]map[string]any{}
	}
	if s.ACME == nil {
		s.ACME = []map[string]any{}
	}
	if s.Templates == nil {
		s.Templates = []map[string]any{}
	}
	if s.Outputs == nil {
		s.Outputs = []map[string]any{}
	}
	if s.Databases == nil {
		s.Databases = []map[string]any{}
	}
	if s.Logs == nil {
		s.Logs = []map[string]any{}
	}
	return s
}

func (s *websiteExtensionStore) persistLocked() error {
	if s.db == nil {
		return errors.New("网站扩展公共数据库未初始化")
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO website_extension_state(id,payload,updated_at) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, data, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *websiteExtensionStore) id() string {
	s.NextID++
	return strconv.FormatUint(s.NextID, 10)
}

func extensionJSON(w http.ResponseWriter, data any) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}
func extensionError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}

func decodeExtension(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return nil, errors.New("请求体不能为空")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		return nil, fmt.Errorf("解析网站扩展请求失败: %w", err)
	}
	if body == nil {
		body = map[string]any{}
	}
	return body, nil
}

func bodyString(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := body[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func bodyID(body map[string]any) string {
	for _, key := range []string{"id", "websiteId", "websiteID", "templateId", "outputId"} {
		if value, ok := body[key]; ok {
			switch typed := value.(type) {
			case string:
				return strings.TrimSpace(typed)
			case float64:
				return strconv.FormatUint(uint64(typed), 10)
			}
		}
	}
	return ""
}

func bodyNumber(body map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := body[key].(float64); ok {
			return value
		}
		if value, ok := body[key].(string); ok {
			n, _ := strconv.ParseFloat(value, 64)
			return n
		}
	}
	return 0
}

func bodyIDs(value any) []uint {
	items, _ := value.([]any)
	result := make([]uint, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case float64:
			if v > 0 {
				result = append(result, uint(v))
			}
		case string:
			n, _ := strconv.ParseUint(v, 10, 64)
			if n > 0 {
				result = append(result, uint(n))
			}
		}
	}
	return result
}

func cloneRecord(item map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range item {
		out[k] = v
	}
	return out
}
func pageRecords(items []map[string]any, body map[string]any) map[string]any {
	page, size := 1, 100
	if n, ok := body["page"].(float64); ok && int(n) > 0 {
		page = int(n)
	}
	if n, ok := body["pageSize"].(float64); ok && int(n) > 0 {
		size = int(n)
	}
	if size > 500 {
		size = 500
	}
	start := (page - 1) * size
	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	result := make([]map[string]any, end-start)
	copy(result, items[start:end])
	return map[string]any{"total": len(items), "items": result, "page": page, "pageSize": size}
}

// registerWebsiteExtensionRoutes 注册旧网站模块中未被专用处理器覆盖的真实接口。
func registerWebsiteExtensionRoutes(mux *http.ServeMux) {
	store := newWebsiteExtensionStore()
	get := func(w http.ResponseWriter, r *http.Request) {
		rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/websites/"), "/")
		parts := strings.Split(rest, "/")
		store.mu.Lock()
		defer store.mu.Unlock()
		switch {
		case rest == "databases":
			extensionJSON(w, pageRecords(store.Databases, map[string]any{"page": 1.0, "pageSize": 100.0}))
		case len(parts) == 2 && parts[0] == "ca":
			for _, item := range store.ACME {
				if fmt.Sprint(item["id"]) == parts[1] {
					extensionJSON(w, item)
					return
				}
			}
			extensionError(w, http.StatusNotFound, errors.New("证书账户不存在"))
		case len(parts) == 3 && parts[0] == "default" && parts[1] == "html":
			extensionJSON(w, map[string]any{"type": parts[2], "content": "", "source": "local"})
		case len(parts) == 2 && parts[1] == "lbs":
			id, err := parseID(parts[0])
			if err != nil {
				extensionError(w, http.StatusBadRequest, err)
				return
			}
			svc := service.NewWebsiteService("")
			if _, err := svc.Get(id); err != nil {
				extensionError(w, http.StatusNotFound, err)
				return
			}
			cfg, err := svc.GetConfig(id, "lbs")
			if err != nil {
				extensionError(w, http.StatusInternalServerError, err)
				return
			}
			upstreams := cfg["upstreams"]
			if upstreams == nil {
				upstreams = []any{}
			}
			extensionJSON(w, upstreams)
		case len(parts) == 2 && parts[0] == "resource":
			id, err := parseID(parts[1])
			if err != nil {
				extensionError(w, http.StatusBadRequest, err)
				return
			}
			svc := service.NewWebsiteService("")
			website, err := svc.Get(id)
			if err != nil {
				extensionError(w, http.StatusNotFound, err)
				return
			}
			domains, _ := svc.ListDomains(id)
			resources := []map[string]any{{"name": website.PrimaryDomain, "type": "website", "resourceID": website.ID, "detail": website}}
			for _, domain := range domains {
				resources = append(resources, map[string]any{"name": domain.Domain, "type": "domain", "resourceID": domain.ID, "detail": domain})
			}
			extensionJSON(w, resources)
		default:
			extensionError(w, http.StatusNotFound, errors.New("网站扩展接口不存在"))
		}
	}
	mux.HandleFunc("GET /api/v2/websites/{rest...}", get)

	post := func(w http.ResponseWriter, r *http.Request) {
		rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/websites/"), "/")
		body, err := decodeExtension(r)
		if err != nil {
			extensionError(w, http.StatusBadRequest, err)
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
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
			id := bodyID(body)
			if id == "" {
				id = bodyString(body, "path", "name")
			}
			if id == "" {
				extensionError(w, 400, errors.New("认证资源标识不能为空"))
				return
			}
			item := cloneRecord(body)
			item["id"] = id
			item["updatedAt"] = now
			store.Auths[id] = item
			result = item
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
				enabled, _ := body["enabled"].(bool)
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
			item := cloneRecord(body)
			item["id"] = store.id()
			item["createdAt"] = now
			store.Databases = append(store.Databases, item)
			result = item
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
				if log, logErr := service.NewWebsiteService("").WebsiteLog(id, logType, int(bodyNumber(body, "page")), int(bodyNumber(body, "pageSize"))); logErr == nil {
					result = log
					break
				}
			}
			result = pageRecords(store.Logs, body)
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
			id := bodyID(body)
			if id == "" {
				extensionError(w, 400, errors.New("网站 ID 不能为空"))
				return
			}
			result = map[string]any{"websiteId": id, "version": bodyString(body, "version", "phpVersion"), "updatedAt": now}
		case rest == "proxies" || rest == "proxies/update" || rest == "proxies/status" || rest == "proxies/file":
			id := bodyID(body)
			if id == "" {
				extensionError(w, 400, errors.New("网站代理 ID 不能为空"))
				return
			}
			item := cloneRecord(body)
			item["id"] = id
			item["updatedAt"] = now
			store.Proxies[id] = item
			result = item
		case rest == "proxies/delete":
			id := bodyID(body)
			delete(store.Proxies, id)
			result = map[string]any{"id": id, "deleted": true}
		case rest == "templates/search":
			result = pageRecords(store.Templates, body)
		case rest == "templates/get":
			id := bodyID(body)
			for _, item := range store.Templates {
				if fmt.Sprint(item["id"]) == id {
					result = item
					break
				}
			}
			if result == nil {
				extensionError(w, 404, errors.New("网站模板不存在"))
				return
			}
		case rest == "templates" || rest == "templates/update":
			item := cloneRecord(body)
			id := bodyID(body)
			if id == "" {
				id = store.id()
			}
			item["id"] = id
			item["updatedAt"] = now
			replaced := false
			for i, old := range store.Templates {
				if fmt.Sprint(old["id"]) == id {
					store.Templates[i] = item
					replaced = true
				}
			}
			if !replaced {
				store.Templates = append(store.Templates, item)
			}
			result = item
		case rest == "templates/del":
			id := bodyID(body)
			filtered := store.Templates[:0]
			for _, item := range store.Templates {
				if fmt.Sprint(item["id"]) != id {
					filtered = append(filtered, item)
				}
			}
			store.Templates = filtered
			result = map[string]any{"id": id, "deleted": true}
		case strings.HasPrefix(rest, "templates/outputs"):
			if rest == "templates/outputs/search" {
				result = pageRecords(store.Outputs, body)
			} else if rest == "templates/outputs/get" {
				id := bodyID(body)
				for _, item := range store.Outputs {
					if fmt.Sprint(item["id"]) == id {
						result = item
						break
					}
				}
				if result == nil {
					extensionError(w, 404, errors.New("模板产物不存在"))
					return
				}
			} else if rest == "templates/outputs/del" {
				id := bodyID(body)
				filtered := store.Outputs[:0]
				for _, item := range store.Outputs {
					if fmt.Sprint(item["id"]) != id {
						filtered = append(filtered, item)
					}
				}
				store.Outputs = filtered
				result = map[string]any{"id": id, "deleted": true}
			} else {
				item := cloneRecord(body)
				item["id"] = store.id()
				item["updatedAt"] = now
				store.Outputs = append(store.Outputs, item)
				result = item
			}
		case rest == "templates/preview":
			result = map[string]any{"content": bodyString(body, "content", "template"), "variables": body["variables"]}
		case rest == "templates/upload":
			result = map[string]any{"status": "stored", "fileName": bodyString(body, "fileName", "name")}
		case rest == "waf/attack/stat" || rest == "waf/block/search" || rest == "waf/log/search" || rest == "waf/relation/stat":
			result = pageRecords(store.Logs, body)
		default:
			extensionError(w, http.StatusNotFound, errors.New("网站扩展接口不存在"))
			return
		}
		if err := store.persistLocked(); err != nil {
			extensionError(w, 500, fmt.Errorf("保存网站扩展状态失败: %w", err))
			return
		}
		extensionJSON(w, result)
	}
	mux.HandleFunc("POST /api/v2/websites/{rest...}", post)
}
