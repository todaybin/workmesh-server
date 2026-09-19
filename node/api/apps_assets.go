// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func handleAppUpdate(w http.ResponseWriter, s *appStore, body map[string]any) {
	id := appValue(body, "installID", "installId", "appInstallId", "id")
	s.mu.Lock()
	index, item := findApp(s.state.Apps, id)
	if index >= 0 {
		if port, ok := body["port"]; ok {
			if item.Config == nil {
				item.Config = map[string]any{}
			}
			item.Config["port"] = port
		}
		for key, value := range body {
			if item.Config == nil {
				item.Config = map[string]any{}
			}
			item.Config[key] = value
		}
		item.UpdatedAt = time.Now().UTC()
		s.state.Apps[index] = item
	}
	_ = s.saveLocked()
	s.mu.Unlock()
	appOK(w, map[string]any{"id": id, "updated": index >= 0})
}

func appIcon(w http.ResponseWriter, s *appStore, key string) {
	var source, cacheKey string
	s.mu.Lock()
	_ = s.ensureCatalogLocked()
	_, item := findApp(s.state.Catalog, key)
	if item.ID == "" {
		_, item = findApp(s.state.Apps, key)
	}
	s.mu.Unlock()
	if item.ID != "" && item.IconURL != "" {
		source = item.IconURL
		cacheKey = item.Key
	}
	if source != "" {
		cachePath := filepath.Join(filepath.Dir(s.path), "app-icons", appStableID(cacheKey)+".png")
		if cached, err := os.ReadFile(cachePath); err == nil && len(cached) > 0 {
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "public, max-age=2592000")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cached)
			return
		}
		if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
			source = strings.TrimRight(appStoreRemoteBase()+"/"+item.Key, "/") + "/" + strings.TrimLeft(source, "/")
		}
		if data := fetchRemoteAsset(source); len(data) > 0 {
			if cacheKey != "" {
				cacheRemoteIcon(s, cacheKey, data)
			}
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "public, max-age=2592000")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			return
		}
	}
	// 透明 1x1 PNG：目录没有图标时也返回真实图片响应，避免浏览器将 JSON 当脚本解析。
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func fetchRemoteAsset(url string) []byte {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}

func cacheRemoteIcon(s *appStore, key string, data []byte) {
	if len(data) == 0 {
		return
	}
	path := filepath.Join(filepath.Dir(s.path), "app-icons", appStableID(key)+".png")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err == nil {
		_ = os.Rename(tmp, path)
	}
}

func isAppRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	p := pattern
	if len(parts) == 2 {
		p = parts[1]
	}
	return p == "/api/v2/apps" || strings.HasPrefix(p, "/api/v2/apps/")
}

// handleCustomAppSync 将配置的自定义应用归档复制到本机资源目录；未配置真实归档时返回 503。
// 该接口不接受浏览器上传内容，归档必须由受控部署流程放置并通过环境变量指定。
func handleCustomAppSync(w http.ResponseWriter, r *http.Request, s *appStore) {
	source := strings.TrimSpace(os.Getenv("WORKMESH_CUSTOM_APP_ARCHIVE"))
	if source == "" {
		source = strings.TrimSpace(os.Getenv("WORKMESH_CUSTOM_APP_PACKAGE"))
	}
	if source == "" {
		domainError(w, http.StatusServiceUnavailable, "CUSTOM_APP_SOURCE_UNAVAILABLE", "未配置自定义应用商店归档")
		return
	}
	info, err := os.Stat(source)
	if err != nil || !info.Mode().IsRegular() {
		if err == nil {
			err = errors.New("归档不是普通文件")
		}
		domainError(w, http.StatusBadRequest, "CUSTOM_APP_SOURCE_INVALID", "自定义应用归档不可用: "+err.Error())
		return
	}
	const maxArchive = 512 << 20
	if info.Size() <= 0 || info.Size() > maxArchive {
		domainError(w, http.StatusBadRequest, "CUSTOM_APP_SOURCE_INVALID", "自定义应用归档大小无效")
		return
	}
	destDir := filepath.Join(filepath.Dir(s.path), "custom-app")
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		domainError(w, http.StatusInternalServerError, "CUSTOM_APP_SAVE_FAILED", "创建自定义应用目录失败: "+err.Error())
		return
	}
	dest := filepath.Join(destDir, "apps.tar.gz")
	tmp := dest + ".tmp-" + idToken()
	in, err := os.Open(source)
	if err != nil {
		domainError(w, http.StatusBadRequest, "CUSTOM_APP_SOURCE_INVALID", "读取自定义应用归档失败: "+err.Error())
		return
	}
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		_, err = io.Copy(out, io.LimitReader(in, maxArchive+1))
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
	}
	_ = in.Close()
	if err == nil {
		if copied, statErr := os.Stat(tmp); statErr != nil || copied.Size() != info.Size() {
			err = errors.New("归档复制大小校验失败")
		}
	}
	if err == nil {
		err = os.Rename(tmp, dest)
	}
	if err != nil {
		_ = os.Remove(tmp)
		domainError(w, http.StatusInternalServerError, "CUSTOM_APP_SAVE_FAILED", "保存自定义应用归档失败: "+err.Error())
		return
	}
	taskID := strings.TrimSpace(appValue(appBody(r), "taskID", "taskId"))
	if taskID == "" {
		taskID = idToken()
	}
	ensureAppTaskLog(taskID, "", "custom-app", "completed", "自定义应用归档同步完成")
	appOK(w, map[string]any{"taskID": taskID, "path": dest, "size": info.Size(), "synced": true})
}
