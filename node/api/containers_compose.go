// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"fmt"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func handleComposeCreate(w http.ResponseWriter, r *http.Request, s *containerStore) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	path := composeFilePath(req)
	if err := validateComposeFile(path); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if req.Content == "" {
		req.Content = req.File
	}
	if strings.TrimSpace(req.Content) == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "Compose 内容不能为空"})
		return
	}
	if err := writeAtomicFile(path, []byte(req.Content)); err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("写入 Compose 文件失败: %v", err)})
		return
	}
	if req.Env != "" {
		if err := writeAtomicFile(filepath.Join(filepath.Dir(path), ".env"), []byte(req.Env)); err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("写入 Compose 环境文件失败: %v", err)})
			return
		}
	}
	now := time.Now().UTC()
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneContainerState(s.state)
	item := composeRecord{ID: idToken(), Name: name, Path: path, CreatedAt: now, UpdatedAt: now}
	s.state.Composes = append(s.state.Composes, item)
	if err := s.saveLocked(); err != nil {
		s.state = previous
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("保存 Compose 记录失败: %v", err)})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}

func handleComposeUpdate(w http.ResponseWriter, r *http.Request, s *containerStore) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	path := composeFilePath(req)
	if err := validateComposeFile(path); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if req.Content == "" {
		req.Content = req.File
	}
	if strings.TrimSpace(req.Content) == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "Compose 内容不能为空"})
		return
	}
	if err := writeAtomicFile(path, []byte(req.Content)); err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("更新 Compose 文件失败: %v", err)})
		return
	}
	if req.Env != "" {
		if err := writeAtomicFile(filepath.Join(filepath.Dir(path), ".env"), []byte(req.Env)); err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("更新 Compose 环境文件失败: %v", err)})
			return
		}
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := cloneContainerState(s.state)
	found := false
	for i := range s.state.Composes {
		if s.state.Composes[i].Path == path || (req.Name != "" && s.state.Composes[i].Name == req.Name) {
			s.state.Composes[i].Path = path
			if req.Name != "" {
				s.state.Composes[i].Name = req.Name
			}
			s.state.Composes[i].UpdatedAt = now
			found = true
			break
		}
	}
	if !found {
		s.state.Composes = append(s.state.Composes, composeRecord{ID: idToken(), Name: req.Name, Path: path, CreatedAt: now, UpdatedAt: now})
	}
	if err := s.saveLocked(); err != nil {
		s.state = previous
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("保存 Compose 记录失败: %v", err)})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": path}})
}

func handleComposePin(w http.ResponseWriter, r *http.Request, s *containerStore) {
	var req composeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "Compose 名称不能为空"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Composes {
		if s.state.Composes[i].Name == name || s.state.Composes[i].Path == req.Path {
			previous := cloneContainerState(s.state)
			s.state.Composes[i].Pinned = req.IsPinned
			s.state.Composes[i].UpdatedAt = time.Now().UTC()
			if err := s.saveLocked(); err != nil {
				s.state = previous
				wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("保存 Compose 置顶状态失败: %v", err)})
				return
			}
			wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": s.state.Composes[i]})
			return
		}
	}
	wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "NOT_FOUND"}, "message": "Compose 不存在"})
}

func registerContainerRepositoryRoutes(mux *http.ServeMux, s *containerStore) {
	list := func(w http.ResponseWriter, r *http.Request) {
		var q map[string]any
		if r != nil && r.Body != nil {
			_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q)
		}
		keyword := strings.ToLower(valueString(q, "name", "keyword"))
		s.mu.RLock()
		items := make([]imageRepository, 0, len(s.state.Repositories))
		for _, item := range s.state.Repositories {
			if keyword == "" || strings.Contains(strings.ToLower(item.Name), keyword) || strings.Contains(strings.ToLower(item.DownloadURL), keyword) {
				items = append(items, sanitizeImageRepository(item))
			}
		}
		s.mu.RUnlock()
		sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
	}
	mux.HandleFunc("GET /api/v2/containers/repo", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := make([]imageRepository, 0, len(s.state.Repositories))
		for _, item := range s.state.Repositories {
			items = append(items, sanitizeImageRepository(item))
		}
		s.mu.RUnlock()
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
	})
	mux.HandleFunc("POST /api/v2/containers/repo/search", list)
	mux.HandleFunc("POST /api/v2/containers/repo", func(w http.ResponseWriter, r *http.Request) {
		var in imageRepository
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "仓库名称不能为空"})
			return
		}
		now := time.Now().UTC()
		in.ID = idToken()
		in.CreatedAt = now
		in.UpdatedAt = now
		s.mu.Lock()
		previous := cloneContainerState(s.state)
		s.state.Repositories = append(s.state.Repositories, in)
		err := s.saveLocked()
		if err != nil {
			s.state = previous
		}
		s.mu.Unlock()
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": sanitizeImageRepository(in)})
	})
	mux.HandleFunc("POST /api/v2/containers/repo/update", func(w http.ResponseWriter, r *http.Request) {
		var in imageRepository
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil || strings.TrimSpace(in.ID) == "" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "仓库 ID 无效"})
			return
		}
		s.mu.Lock()
		previous := cloneContainerState(s.state)
		for i := range s.state.Repositories {
			if s.state.Repositories[i].ID == in.ID {
				if in.Name != "" {
					s.state.Repositories[i].Name = in.Name
				}
				if in.DownloadURL != "" {
					s.state.Repositories[i].DownloadURL = in.DownloadURL
				}
				if in.Protocol != "" {
					s.state.Repositories[i].Protocol = in.Protocol
				}
				if in.Username != "" {
					s.state.Repositories[i].Username = in.Username
				}
				if in.Password != "" {
					s.state.Repositories[i].Password = in.Password
				}
				s.state.Repositories[i].Auth = in.Auth
				s.state.Repositories[i].UpdatedAt = time.Now().UTC()
				if err := s.saveLocked(); err != nil {
					s.state = previous
					s.mu.Unlock()
					wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("保存镜像仓库失败: %v", err)})
					return
				}
				item := sanitizeImageRepository(s.state.Repositories[i])
				s.mu.Unlock()
				wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
				return
			}
		}
		s.mu.Unlock()
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "仓库不存在"})
	})
	for _, path := range []string{"/api/v2/containers/repo/del"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				ID  string   `json:"id"`
				IDs []string `json:"ids"`
			}
			_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in)
			ids := in.IDs
			if in.ID != "" {
				ids = append(ids, in.ID)
			}
			if len(ids) == 0 {
				wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "仓库 ID 不能为空"})
				return
			}
			s.mu.Lock()
			previous := cloneContainerState(s.state)
			kept := s.state.Repositories[:0]
			removed := 0
			for _, item := range s.state.Repositories {
				found := false
				for _, id := range ids {
					if id == item.ID {
						found = true
					}
				}
				if found {
					removed++
				} else {
					kept = append(kept, item)
				}
			}
			s.state.Repositories = kept
			if err := s.saveLocked(); err != nil {
				s.state = previous
				s.mu.Unlock()
				wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("删除镜像仓库失败: %v", err)})
				return
			}
			s.mu.Unlock()
			wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": removed}})
		})
	}
	mux.HandleFunc("POST /api/v2/containers/repo/status", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in)
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, item := range s.state.Repositories {
			if item.ID == in.ID {
				wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"id": item.ID, "configured": item.DownloadURL != "", "auth": item.Auth}})
				return
			}
		}
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "仓库不存在"})
	})
}

func registerContainerTemplateRoutes(mux *http.ServeMux, s *containerStore) {
	list := func(w http.ResponseWriter, r *http.Request) {
		var q map[string]any
		if r != nil && r.Body != nil {
			_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q)
		}
		keyword := strings.ToLower(valueString(q, "name", "keyword", "description"))
		s.mu.RLock()
		items := make([]composeTemplate, 0, len(s.state.Templates))
		for _, item := range s.state.Templates {
			if keyword == "" || strings.Contains(strings.ToLower(item.Name), keyword) || strings.Contains(strings.ToLower(item.Description), keyword) {
				items = append(items, item)
			}
		}
		s.mu.RUnlock()
		sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
	}
	mux.HandleFunc("GET /api/v2/containers/template", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		items := append([]composeTemplate(nil), s.state.Templates...)
		s.mu.RUnlock()
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
	})
	mux.HandleFunc("POST /api/v2/containers/template/search", list)
	mux.HandleFunc("POST /api/v2/containers/template", func(w http.ResponseWriter, r *http.Request) {
		var in composeTemplate
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "模板名称不能为空"})
			return
		}
		if len(in.Content) > 4<<20 {
			wmhttp.JSON(w, 413, map[string]any{"code": "ERR", "message": "模板内容超过限制"})
			return
		}
		now := time.Now().UTC()
		in.ID = idToken()
		in.CreatedAt = now
		in.UpdatedAt = now
		s.mu.Lock()
		previous := cloneContainerState(s.state)
		s.state.Templates = append(s.state.Templates, in)
		err := s.saveLocked()
		if err != nil {
			s.state = previous
		}
		s.mu.Unlock()
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": in})
	})
	mux.HandleFunc("POST /api/v2/containers/template/update", func(w http.ResponseWriter, r *http.Request) {
		var in composeTemplate
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&in); err != nil || strings.TrimSpace(in.ID) == "" {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "模板 ID 无效"})
			return
		}
		if len(in.Content) > 4<<20 {
			wmhttp.JSON(w, 413, map[string]any{"code": "ERR", "message": "模板内容超过限制"})
			return
		}
		s.mu.Lock()
		previous := cloneContainerState(s.state)
		for i := range s.state.Templates {
			if s.state.Templates[i].ID == in.ID {
				if in.Name != "" {
					s.state.Templates[i].Name = in.Name
				}
				if in.Description != "" {
					s.state.Templates[i].Description = in.Description
				}
				if in.Content != "" {
					s.state.Templates[i].Content = in.Content
				}
				s.state.Templates[i].UpdatedAt = time.Now().UTC()
				if err := s.saveLocked(); err != nil {
					s.state = previous
					s.mu.Unlock()
					wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("保存 Compose 模板失败: %v", err)})
					return
				}
				item := s.state.Templates[i]
				s.mu.Unlock()
				wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
				return
			}
		}
		s.mu.Unlock()
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "模板不存在"})
	})
	mux.HandleFunc("POST /api/v2/containers/template/batch", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Templates []composeTemplate `json:"templates"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&in); err != nil || len(in.Templates) == 0 {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "模板列表不能为空"})
			return
		}
		now := time.Now().UTC()
		s.mu.Lock()
		previous := cloneContainerState(s.state)
		for _, item := range in.Templates {
			if strings.TrimSpace(item.Name) == "" {
				continue
			}
			item.ID = idToken()
			item.CreatedAt = now
			item.UpdatedAt = now
			s.state.Templates = append(s.state.Templates, item)
		}
		err := s.saveLocked()
		if err != nil {
			s.state = previous
		}
		s.mu.Unlock()
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"count": len(in.Templates)}})
	})
	mux.HandleFunc("POST /api/v2/containers/template/del", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID  string   `json:"id"`
			IDs []string `json:"ids"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in)
		ids := in.IDs
		if in.ID != "" {
			ids = append(ids, in.ID)
		}
		if len(ids) == 0 {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "模板 ID 不能为空"})
			return
		}
		s.mu.Lock()
		previous := cloneContainerState(s.state)
		kept := s.state.Templates[:0]
		removed := 0
		for _, item := range s.state.Templates {
			found := false
			for _, id := range ids {
				if item.ID == id {
					found = true
				}
			}
			if found {
				removed++
			} else {
				kept = append(kept, item)
			}
		}
		s.state.Templates = kept
		if err := s.saveLocked(); err != nil {
			s.state = previous
			s.mu.Unlock()
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": fmt.Sprintf("删除 Compose 模板失败: %v", err)})
			return
		}
		s.mu.Unlock()
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": removed}})
	})
}

func registerContainerSettingsRoutes(mux *http.ServeMux, s *containerStore) {
	for _, path := range []string{"/api/v2/containers/logoption/update", "/api/v2/containers/ipv6option/update"} {
		p := path
		mux.HandleFunc("POST "+p, func(w http.ResponseWriter, r *http.Request) {
			var in map[string]any
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil {
				wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
			s.mu.Lock()
			previous := cloneContainerState(s.state)
			if s.state.Settings == nil {
				s.state.Settings = map[string]any{}
			}
			key := strings.TrimPrefix(strings.TrimPrefix(p, "/api/v2/containers/"), "/update")
			s.state.Settings[key] = in
			err := s.saveLocked()
			if err != nil {
				s.state = previous
			}
			s.mu.Unlock()
			if err != nil {
				wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
			wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": in})
		})
	}
}
