// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func handleFileAISearch(w http.ResponseWriter, r *http.Request, req fileAdvancedRequest) {
	root, err := cleanFilePath(req.Path)
	if err != nil || strings.TrimSpace(req.Query) == "" {
		if err == nil {
			err = errors.New("query 不能为空")
		}
		fileError(w, http.StatusBadRequest, err)
		return
	}
	if st, statErr := os.Stat(root); statErr != nil || !st.IsDir() {
		if statErr == nil {
			statErr = errors.New("搜索路径不是目录")
		}
		fileError(w, http.StatusNotFound, statErr)
		return
	}
	maxFiles := req.MaxScanFiles
	if maxFiles <= 0 || maxFiles > 1000 {
		maxFiles = 500
	}
	maxHits := req.MaxTotalHits
	if maxHits <= 0 || maxHits > 5000 {
		maxHits = 500
	}
	matcher := func(string) bool { return false }
	if req.UseRegex {
		rx, compileErr := regexp.Compile(req.Query)
		if compileErr != nil {
			fileError(w, http.StatusBadRequest, compileErr)
			return
		}
		matcher = rx.MatchString
	} else {
		needle := req.Query
		if !req.MatchCase {
			needle = strings.ToLower(needle)
		}
		matcher = func(s string) bool {
			target := s
			if !req.MatchCase {
				target = strings.ToLower(target)
			}
			if req.WholeWord {
				for _, f := range strings.FieldsFunc(target, func(r rune) bool { return r < '0' || (r > '9' && r < 'A') || (r > 'Z' && r < 'a') || r > 'z' }) {
					if f == needle {
						return true
					}
				}
				return false
			}
			return strings.Contains(target, needle)
		}
	}
	type hit struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	hits := make([]hit, 0)
	scanned := 0
	truncated := false
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			if !req.ContainSub && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if scanned >= maxFiles {
			truncated = true
			return filepath.SkipAll
		}
		if info.Size() > 8<<20 {
			return nil
		}
		scanned++
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if matcher(line) {
				hits = append(hits, hit{Path: path, Line: i + 1, Text: line})
				if len(hits) >= maxHits {
					truncated = true
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"mode": "grep", "summary": "", "hits": hits, "contentScannedFiles": scanned, "contentHitsTruncated": truncated, "truncated": truncated, "itemCount": len(hits)}})
}

// handleFileWget 启动带超时和取消能力的远程文件下载任务。
