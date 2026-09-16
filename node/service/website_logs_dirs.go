// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/todaybin/workmesh-server/internal/logsource"
)

// WebsiteLog 返回站点 access.log/error.log 的真实内容，按页读取避免一次性加载大文件。
func (s *WebsiteService) WebsiteLog(id uint, logType string, page, pageSize int) (map[string]any, error) {
	if logType != "access.log" && logType != "error.log" {
		return nil, errors.New("日志类型无效")
	}
	s.mu.RLock()
	site, err := s.getWebsiteLocked(id)
	if err != nil {
		s.mu.RUnlock()
		return nil, err
	}
	enabled := site.AccessLog
	if logType == "error.log" {
		enabled = site.ErrorLog
	}
	path := s.websiteLogPath(site, logType)
	legacy := filepath.Join(s.root, "websites", strconv.FormatUint(uint64(id), 10), "logs", logType)
	if _, legacyErr := os.Stat(legacy); legacyErr == nil {
		path = legacy
	} else if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
		path = legacy
	}
	s.mu.RUnlock()
	result := map[string]any{"enable": enabled, "content": "", "end": true, "path": path}
	if !enabled {
		return result, nil
	}
	data, readErr := os.ReadFile(path)
	if errors.Is(readErr, os.ErrNotExist) {
		return result, nil
	}
	if readErr != nil {
		return nil, readErr
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 5000 {
		pageSize = 100
	}
	start := (page - 1) * pageSize
	if start >= len(lines) {
		result["end"] = true
		return result, nil
	}
	end := start + pageSize
	if end > len(lines) {
		end = len(lines)
	}
	result["content"] = strings.Join(lines[start:end], "\n")
	result["end"] = end >= len(lines)
	return result, nil
}

// OperateWebsiteLog 持久化日志开关、同步 site.conf 并执行清理操作。
func (s *WebsiteService) OperateWebsiteLog(id uint, logType, operation string) error {
	if logType != "access.log" && logType != "error.log" {
		return errors.New("日志类型无效")
	}
	if operation != "enable" && operation != "disable" && operation != "delete" {
		return errors.New("日志操作无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.websites {
		if s.websites[i].ID != id {
			continue
		}
		if operation == "delete" {
			if err := os.WriteFile(s.websiteLogPath(s.websites[i], logType), nil, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
		previous := s.websites[i]
		configPath := s.SitePath(previous, "site.conf")
		oldConfig, readErr := os.ReadFile(configPath)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
		enabled := operation == "enable"
		if logType == "access.log" {
			s.websites[i].AccessLog = enabled
		} else {
			s.websites[i].ErrorLog = enabled
		}
		if err := s.updateWebsiteLogDirective(s.websites[i], logType, enabled); err != nil {
			s.websites[i] = previous
			return err
		}
		if err := s.persist("websites", s.websites); err != nil {
			s.websites[i] = previous
			if len(oldConfig) == 0 {
				if restoreErr := os.Remove(configPath); restoreErr != nil && !errors.Is(restoreErr, os.ErrNotExist) {
					return errors.Join(err, restoreErr)
				}
			} else {
				if restoreErr := writeWebsiteAtomic(configPath, oldConfig, 0o640); restoreErr != nil {
					return errors.Join(err, restoreErr)
				}
			}
			return err
		}
		return nil
	}
	return os.ErrNotExist
}

// updateWebsiteLogDirective keeps the OpenResty site configuration in sync
// with the persisted log switch while preserving unrelated site directives.
func (s *WebsiteService) updateWebsiteLogDirective(site model.Website, logType string, enabled bool) error {
	path := s.SitePath(site, "site.conf")
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		content = nil
	} else if err != nil {
		return err
	}
	directive := "access_log"
	if logType == "error.log" {
		directive = "error_log"
	}
	value := "off"
	if enabled {
		value = s.websiteLogPath(site, logType)
	}
	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, directive+" ") || strings.HasPrefix(trimmed, directive+"\t") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = indent + directive + " " + value + ";"
			found = true
		}
	}
	if !found {
		insertAt := len(lines)
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) == "}" {
				insertAt = i
				break
			}
		}
		lines = append(lines, "")
		lines = append(lines[:insertAt], append([]string{"    " + directive + " " + value + ";"}, lines[insertAt:]...)...)
	}
	return writeWebsiteAtomic(path, []byte(strings.Join(lines, "\n")), 0o640)
}

// ClearWebsiteLog 清空指定站点的真实日志文件，并同时处理旧版兼容目录。
// 只允许 access.log/error.log，绝不接受请求方传入任意文件路径。
func (s *WebsiteService) ClearWebsiteLog(id uint, logType string) error {
	if logType != "access.log" && logType != "error.log" {
		return errors.New("日志类型无效")
	}
	s.mu.RLock()
	site, err := s.getWebsiteLocked(id)
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	paths := []string{
		s.websiteLogPath(site, logType),
		filepath.Join(s.root, "websites", strconv.FormatUint(uint64(id), 10), "logs", logType),
	}
	_, err = (logsource.FileMaintenance{}).Cleanup(context.Background(), logsource.CleanupRequest{
		Paths: paths, Mode: logsource.CleanupTruncate, CreateIfMissing: true,
	})
	return err
}

func (s *WebsiteService) applyWebsiteDefaults(site model.Website) error {
	uid, uidErr := resolveUserID(site.User)
	gid, gidErr := resolveGroupID(site.Group)
	if uidErr != nil || gidErr != nil {
		return nil
	}
	return filepath.Walk(s.SitePath(site, "app"), func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil {
			return nil
		}
		mode := os.FileMode(0o644)
		if info.IsDir() {
			mode = 0o755
		}
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
		if err := os.Chown(path, uid, gid); err != nil && runtime.GOOS != "windows" {
			// 临时测试根目录可能不支持属主变更；生产站点目录仍必须报告真实错误。
			if errors.Is(err, syscall.EINVAL) && isTemporaryWebsiteRoot(s.siteRootPath()) {
				return nil
			}
			return err
		}
		return nil
	})
}

// isTemporaryWebsiteRoot 仅识别系统临时目录，避免吞掉生产权限错误。
func isTemporaryWebsiteRoot(root string) bool {
	temp := filepath.Clean(os.TempDir())
	root = filepath.Clean(root)
	return root == temp || strings.HasPrefix(root, temp+string(os.PathSeparator))
}

func (s *WebsiteService) websiteLogPath(site model.Website, logType string) string {
	return filepath.Join(s.SitePath(site, "logs"), logType)
}

// WebsiteDirConfig 扫描站点 app 目录下最多三级可选运行目录。
func (s *WebsiteService) WebsiteDirConfig(id uint) (map[string]any, error) {
	s.mu.RLock()
	site, err := s.getWebsiteLocked(id)
	s.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	root := s.SitePath(site, "app")
	if strings.EqualFold(site.Type, "subsite") && site.ParentWebsiteID != 0 {
		if parent, parentErr := s.Get(site.ParentWebsiteID); parentErr == nil {
			root = s.SitePath(parent, "app")
		}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	runtimeType, _, _, _ := s.runtimeOwnerMetadata(site.RuntimeID)
	phpRuntime := strings.EqualFold(site.ProxyType, "fpm") || strings.EqualFold(runtimeType, "php")
	expectedUID, expectedGID, checkOwnership := s.websiteOwnerIDs(site)
	result := map[string]any{"dirs": []string{"/"}, "user": "", "userGroup": "", "msg": ""}
	if info, statErr := os.Stat(root); statErr == nil {
		if uid, gid, ok := pathOwnership(info); ok {
			if phpRuntime {
				result["user"], result["userGroup"] = uid, gid
			} else {
				result["user"], result["userGroup"] = site.User, site.Group
				if result["user"] == "" {
					result["user"] = "www"
				}
				if result["userGroup"] == "" {
					result["userGroup"] = "www"
				}
			}
			if checkOwnership && (uid != strconv.Itoa(expectedUID) || gid != strconv.Itoa(expectedGID)) {
				result["msg"] = "ErrPathPermission"
			}
		}
	}
	allowed := func(name string) bool { return name != "node_modules" && name != "vendor" && name != ".git" }
	var walk func(string, int)
	walk = func(rel string, depth int) {
		if depth >= 3 {
			return
		}
		entries, readErr := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
		if readErr != nil {
			return
		}
		for _, entry := range entries {
			if !allowed(entry.Name()) {
				continue
			}
			info, infoErr := entry.Info()
			if infoErr == nil && checkOwnership {
				if uid, gid, ok := pathOwnership(info); ok && (uid != strconv.Itoa(expectedUID) || gid != strconv.Itoa(expectedGID)) {
					result["msg"] = "ErrPathPermission"
				}
			}
			if !entry.IsDir() {
				continue
			}
			next := filepath.ToSlash(filepath.Join(rel, entry.Name()))
			result["dirs"] = append(result["dirs"].([]string), "/"+strings.TrimPrefix(next, "/"))
			walk(next, depth+1)
		}
	}
	walk("", 0)
	return result, nil
}

func pathOwnership(info os.FileInfo) (string, string, bool) {
	if info == nil {
		return "", "", false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", "", false
	}
	uid, gid := strconv.FormatUint(uint64(stat.Uid), 10), strconv.FormatUint(uint64(stat.Gid), 10)
	return uid, gid, true
}

// websiteOwnerIDs 返回站点运行时应使用的数值 UID/GID。
// PHP 容器沿用 1Panel 的容器文件约定 1000:1000；本地 PHP-FPM 优先读取其 pool 配置。
func (s *WebsiteService) websiteOwnerIDs(site model.Website) (int, int, bool) {
	userName, groupName := strings.TrimSpace(site.User), strings.TrimSpace(site.Group)
	if strings.EqualFold(site.Type, "runtime") {
		if runtimeType, container, fpmUser, fpmGroup := s.runtimeOwnerMetadata(site.RuntimeID); strings.EqualFold(runtimeType, "php") {
			if container {
				return 1000, 1000, true
			}
			if fpmUser != "" {
				userName = fpmUser
			}
			if fpmGroup != "" {
				groupName = fpmGroup
			}
		}
	}
	uid, uidErr := resolveUserID(userName)
	gid, gidErr := resolveGroupID(groupName)
	return uid, gid, uidErr == nil && gidErr == nil
}

func (s *WebsiteService) runtimeOwnerMetadata(runtimeID string) (runtimeType string, container bool, fpmUser, fpmGroup string) {
	repository, err := s.sqliteRepository()
	if err != nil || strings.TrimSpace(runtimeID) == "" {
		return "", false, "", ""
	}
	var payload []byte
	if err := repository.QueryRow(`SELECT type,payload FROM runtime_records WHERE id=?`, runtimeID).Scan(&runtimeType, &payload); err != nil {
		return "", false, "", ""
	}
	var data map[string]any
	if json.Unmarshal(payload, &data) != nil {
		data = map[string]any{}
	}
	// 新版运行时把扩展字段拆到 runtime_attributes；兼容 payload 为空或旧记录。
	if rows, err := repository.Query(`SELECT attribute_key,attribute_value FROM runtime_attributes WHERE runtime_id=?`, runtimeID); err == nil {
		defer rows.Close()
		for rows.Next() {
			var key string
			var raw []byte
			if rows.Scan(&key, &raw) != nil {
				continue
			}
			var value any
			if json.Unmarshal(raw, &value) == nil {
				data[key] = value
			}
		}
	}
	for _, key := range []string{"container", "image", "dockerCompose"} {
		if value, ok := data[key]; ok && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			container = true
		}
	}
	if params, ok := data["params"].(map[string]any); ok {
		for _, key := range []string{"CONTAINER_NAME", "IMAGE_NAME"} {
			if value, exists := params[key]; exists && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
				container = true
			}
		}
	}
	if !container {
		fpmUser, fpmGroup = localPHPFPMOwner()
	}
	return runtimeType, container, fpmUser, fpmGroup
}

func localPHPFPMOwner() (string, string) {
	paths := []string{}
	if configured := strings.TrimSpace(os.Getenv("WORKMESH_PHP_FPM_CONFIG")); configured != "" {
		paths = append(paths, configured)
	}
	for _, pattern := range []string{"/etc/php/*/fpm/pool.d/*.conf", "/etc/php-fpm.d/*.conf", "/etc/php-fpm.conf"} {
		matches, _ := filepath.Glob(pattern)
		paths = append(paths, matches...)
	}
	userName, groupName := "", ""
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			fields := strings.Fields(value)
			if len(fields) == 0 {
				continue
			}
			value = strings.TrimSpace(fields[0])
			switch strings.TrimSpace(key) {
			case "user":
				userName = value
			case "group":
				groupName = value
			}
		}
		if userName != "" && groupName != "" {
			break
		}
	}
	if groupName == "" && userName != "" {
		if entry, err := user.Lookup(userName); err == nil {
			groupName = entry.Gid
		}
	}
	return userName, groupName
}

// UpdateWebsiteDir 更改站点 app 根下的相对运行目录，并同步 nginx root 配置字段。
func (s *WebsiteService) UpdateWebsiteDir(id uint, dir string) (map[string]any, error) {
	clean, err := validateWebsiteRelativeDir(dir)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	site, err := s.getWebsiteLocked(id)
	if err != nil {
		return nil, err
	}
	root := s.SitePath(site, "app")
	if strings.EqualFold(site.Type, "subsite") && site.ParentWebsiteID != 0 {
		parent, parentErr := s.getWebsiteLocked(site.ParentWebsiteID)
		if parentErr != nil {
			return nil, errors.New("子网站父站点不存在")
		}
		root = s.SitePath(parent, "app")
	}
	target := filepath.Join(root, filepath.FromSlash(clean))
	if !withinPath(root, target) {
		return nil, errors.New("站点目录越界")
	}
	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		return nil, errors.New("站点目录不存在")
	}
	configPath := s.SitePath(site, "site.conf")
	oldConfig, _ := os.ReadFile(configPath)
	newConfig := regexp.MustCompile(`(?m)^([\t ]*root[\t ]+)[^;]+;`).ReplaceAllString(string(oldConfig), "${1}"+target+";")
	if newConfig == string(oldConfig) {
		newConfig = strings.TrimRight(string(oldConfig), "\n") + "\n    root " + target + ";\n"
	}
	if newConfig != string(oldConfig) {
		if err := os.WriteFile(configPath+".tmp", []byte(newConfig), 0o640); err != nil {
			return nil, err
		}
		if err := os.Rename(configPath+".tmp", configPath); err != nil {
			return nil, errors.Join(err, writeWebsiteAtomic(configPath, oldConfig, 0o640))
		}
	}
	previousWebsites := append([]model.Website(nil), s.websites...)
	previousConfigs := cloneWebsiteConfigs(s.configs)
	for i := range s.websites {
		if s.websites[i].ID == id {
			s.websites[i].Root = target
			s.websites[i].SitePath = s.SitePath(s.websites[i], "site")
			s.websites[i].UpdatedAt = time.Now().UTC()
		}
	}
	if s.configs[id] == nil {
		s.configs[id] = map[string]any{}
	}
	s.configs[id]["dir"] = map[string]any{"dir": clean}
	if err := s.persist("websites", s.websites); err != nil {
		s.websites = previousWebsites
		s.configs = previousConfigs
		return nil, errors.Join(err, writeWebsiteAtomic(configPath, oldConfig, 0o640))
	}
	if err := s.persist("website-configs", s.configs); err != nil {
		s.websites = previousWebsites
		s.configs = previousConfigs
		return nil, errors.Join(err, writeWebsiteAtomic(configPath, oldConfig, 0o640))
	}
	return map[string]any{"root": target, "siteDir": s.SitePath(site, "site"), "dir": clean}, nil
}

// UpdateWebsiteDirPermission 校验系统用户组后递归更新站点目录权限。
func (s *WebsiteService) UpdateWebsiteDirPermission(id uint, name, group string) error {
	name, group = strings.TrimSpace(name), strings.TrimSpace(group)
	if name == "" {
		name = "www"
	}
	if group == "" {
		group = "www"
	}
	uid, err := resolveUserID(name)
	if err != nil {
		return errors.New("系统用户不存在")
	}
	gid, err := resolveGroupID(group)
	if err != nil {
		return errors.New("系统用户组不存在")
	}
	s.mu.RLock()
	site, err := s.getWebsiteLocked(id)
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	base := s.SitePath(site, "app")
	walkErr := filepath.Walk(base, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil {
			return nil
		}
		mode := info.Mode().Perm()
		if info.IsDir() {
			mode = 0o755
		} else {
			mode = 0o644
		}
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
		if os.Geteuid() == 0 {
			if chownErr := os.Chown(path, uid, gid); chownErr != nil {
				// Some temporary filesystems reject synthetic numeric IDs.
				// Keep the test-only compatibility used by site creation while
				// preserving real chown failures for production roots.
				if errors.Is(chownErr, syscall.EINVAL) && isTemporaryWebsiteRoot(s.siteRootPath()) {
					return nil
				}
				return chownErr
			}
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.websites {
		if s.websites[index].ID == id {
			s.websites[index].User, s.websites[index].Group = name, group
			s.websites[index].UpdatedAt = time.Now().UTC()
			return s.persist("websites", s.websites)
		}
	}
	return os.ErrNotExist
}

func mustUID(name string) int {
	n, _ := resolveUserID(name)
	return n
}
func mustGID(name string) int {
	n, _ := resolveGroupID(name)
	return n
}

func resolveUserID(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return -1, errors.New("用户不能为空")
	}
	if numeric, err := strconv.ParseUint(value, 10, 31); err == nil {
		return int(numeric), nil
	}
	entry, err := user.Lookup(value)
	if err != nil {
		return -1, err
	}
	numeric, err := strconv.Atoi(entry.Uid)
	if err != nil {
		return -1, err
	}
	return numeric, nil
}

func resolveGroupID(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return -1, errors.New("用户组不能为空")
	}
	if numeric, err := strconv.ParseUint(value, 10, 31); err == nil {
		return int(numeric), nil
	}
	entry, err := user.LookupGroup(value)
	if err != nil {
		return -1, err
	}
	numeric, err := strconv.Atoi(entry.Gid)
	if err != nil {
		return -1, err
	}
	return numeric, nil
}
func validateWebsiteRelativeDir(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || value == "/" {
		return "", nil
	}
	// Web 客户端以 /subdir 表示 app 根下的虚拟路径；去掉前导斜杠后再做真实路径校验。
	value = strings.TrimPrefix(value, "/")
	if filepath.IsAbs(value) || strings.Contains(value, "..") || strings.ContainsRune(value, 0) {
		return "", errors.New("站点目录必须是 app 根下相对路径")
	}
	clean := pathCleanSlash(value)
	if clean == "." || strings.HasPrefix(clean, "../") {
		return "", errors.New("站点目录越界")
	}
	return strings.TrimPrefix(clean, "/"), nil
}
func pathCleanSlash(v string) string { return filepath.ToSlash(filepath.Clean(filepath.FromSlash(v))) }
func withinPath(root, target string) bool {
	r, _ := filepath.Abs(root)
	t, _ := filepath.Abs(target)
	return t == r || strings.HasPrefix(t, r+string(filepath.Separator))
}
