// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func handleFileWget(w http.ResponseWriter, r *http.Request, req fileAdvancedRequest) {
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		fileError(w, http.StatusBadRequest, errors.New("url 必须为有效的 http(s) 地址"))
		return
	}
	dir, err := cleanFilePath(req.Path)
	if err != nil || strings.TrimSpace(req.Name) == "" {
		if err == nil {
			err = errors.New("name 不能为空")
		}
		fileError(w, http.StatusBadRequest, err)
		return
	}
	if strings.ContainsAny(req.Name, `/\\`) || req.Name == "." || req.Name == ".." {
		fileError(w, http.StatusBadRequest, errors.New("name 包含非法路径"))
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	key := fileAuxID("wget")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	initFileWgetState()
	proc := &fileWgetProcess{Key: key, URL: u.String(), Path: filepath.Join(dir, req.Name), Status: "running", StartedAt: time.Now().UTC()}
	fileWgetState.Lock()
	if len(fileWgetState.items) >= 200 {
		for oldKey, oldItem := range fileWgetState.items {
			if oldItem.Status == "completed" || oldItem.Status == "failed" || oldItem.Status == "cancelled" {
				delete(fileWgetState.items, oldKey)
				if len(fileWgetState.items) < 200 {
					break
				}
			}
		}
	}
	fileWgetState.items[key] = proc
	fileWgetState.cancel[key] = cancel
	fileWgetState.Unlock()
	go func() {
		defer cancel()
		client := &http.Client{Timeout: 30 * time.Minute}
		request, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if reqErr == nil {
			var resp *http.Response
			resp, reqErr = client.Do(request)
			if reqErr == nil {
				defer resp.Body.Close()
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					reqErr = errors.New(resp.Status)
				} else {
					tmp := proc.Path + ".tmp"
					var out *os.File
					out, reqErr = os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
					if reqErr == nil {
						fileWgetState.Lock()
						proc.Total, proc.Downloaded = resp.ContentLength, 0
						fileWgetState.Unlock()
						// 每次写入后更新已下载字节，WebSocket 进度查询可实时反映长任务状态。
						counter := &wgetProgressWriter{dst: out, key: key}
						_, reqErr = io.Copy(counter, resp.Body)
						_ = out.Close()
						if reqErr == nil {
							reqErr = os.Rename(tmp, proc.Path)
						} else {
							_ = os.Remove(tmp)
						}
					}
				}
			}
		}
		fileWgetState.Lock()
		now := time.Now().UTC()
		proc.FinishedAt = &now
		if ctx.Err() == context.Canceled || ctx.Err() == context.DeadlineExceeded {
			proc.Status = "cancelled"
		} else if reqErr != nil {
			proc.Status = "failed"
			proc.Error = reqErr.Error()
		} else {
			proc.Status = "completed"
		}
		delete(fileWgetState.cancel, key)
		fileWgetState.Unlock()
	}()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"key": key, "path": proc.Path}})
}
