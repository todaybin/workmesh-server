// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"os"
	osuser "os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// handleFileBatchOperation 处理批量检查、删除和权限更新操作，保持原响应契约。
func handleFileBatchOperation(w http.ResponseWriter, path string, req fileAdvancedRequest) {
	switch path {
	case "batch/check":
		if len(req.Paths) == 0 || len(req.Paths) > 500 {
			fileError(w, http.StatusBadRequest, errors.New("paths 数量必须为 1-500"))
			return
		}
		items := make([]map[string]any, 0, len(req.Paths))
		for _, raw := range req.Paths {
			clean, err := cleanFilePath(raw)
			if err != nil {
				continue
			}
			if info, err := os.Stat(clean); err == nil {
				items = append(items, map[string]any{"name": info.Name(), "path": clean, "size": info.Size(), "modTime": info.ModTime(), "isDir": info.IsDir()})
			}
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": items})
	case "batch/del":
		if len(req.Paths) == 0 || len(req.Paths) > 200 {
			fileError(w, http.StatusBadRequest, errors.New("paths 数量必须为 1-200"))
			return
		}
		deleted := make([]string, 0, len(req.Paths))
		failures := make([]map[string]string, 0)
		for _, raw := range req.Paths {
			clean, err := cleanFilePath(raw)
			if err != nil {
				failures = append(failures, map[string]string{"path": raw, "error": err.Error()})
				continue
			}
			if err := os.RemoveAll(clean); err != nil {
				failures = append(failures, map[string]string{"path": clean, "error": err.Error()})
				continue
			}
			deleted = append(deleted, clean)
		}
		if len(deleted) == 0 {
			fileError(w, http.StatusBadRequest, errors.New("没有文件删除成功"))
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"deleted": deleted, "failures": failures, "count": len(deleted)}})
	case "batch/role":
		if len(req.Paths) == 0 || len(req.Paths) > 200 {
			fileError(w, http.StatusBadRequest, errors.New("paths 数量必须为 1-200"))
			return
		}
		if req.Mode < 0 || req.Mode > 0o7777 {
			fileError(w, http.StatusBadRequest, errors.New("mode 超出范围"))
			return
		}
		// Resolve owner IDs once, then apply chown/chmod to each selected path.
		// The 1Panel contract uses sub=true to include descendants of directories.
		uid, gid := -1, -1
		if strings.TrimSpace(req.User) != "" {
			u, lookupErr := osuser.Lookup(strings.TrimSpace(req.User))
			if lookupErr != nil {
				fileError(w, 400, lookupErr)
				return
			}
			uid, _ = strconv.Atoi(u.Uid)
		}
		if strings.TrimSpace(req.Group) != "" {
			g, lookupErr := osuser.LookupGroup(strings.TrimSpace(req.Group))
			if lookupErr != nil {
				fileError(w, 400, lookupErr)
				return
			}
			gid, _ = strconv.Atoi(g.Gid)
		}
		updated := make([]string, 0, len(req.Paths))
		failures := make([]map[string]string, 0)
		for _, raw := range req.Paths {
			clean, err := cleanFilePath(raw)
			if err != nil {
				failures = append(failures, map[string]string{"path": raw, "error": err.Error()})
				continue
			}
			if _, err := os.Stat(clean); err != nil {
				failures = append(failures, map[string]string{"path": clean, "error": "文件不存在"})
				continue
			}
			targets := []string{clean}
			if req.Sub || req.ContainSub {
				if info, statErr := os.Stat(clean); statErr == nil && info.IsDir() {
					_ = filepath.Walk(clean, func(child string, childInfo os.FileInfo, walkErr error) error {
						if walkErr == nil && childInfo != nil {
							targets = append(targets, child)
						}
						return nil
					})
				}
			}
			failed := false
			for _, target := range targets {
				if uid >= 0 || gid >= 0 {
					if chownErr := os.Chown(target, uid, gid); chownErr != nil {
						failures = append(failures, map[string]string{"path": target, "error": chownErr.Error()})
						failed = true
						continue
					}
				}
				if chmodErr := os.Chmod(target, os.FileMode(req.Mode)); chmodErr != nil {
					failures = append(failures, map[string]string{"path": target, "error": chmodErr.Error()})
					failed = true
					continue
				}
			}
			if !failed {
				updated = append(updated, clean)
			}
		}
		if len(updated) == 0 {
			fileError(w, http.StatusBadRequest, errors.New("没有文件权限更新成功"))
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"updated": updated, "failures": failures, "mode": req.Mode, "user": req.User, "group": req.Group}})
	}
}
