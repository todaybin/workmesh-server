// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package logsource

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// CleanupMode identifies the supported file maintenance operation.
type CleanupMode string

const (
	CleanupTruncate CleanupMode = "truncate"
	CleanupRemove   CleanupMode = "remove"
)

// CleanupRequest contains paths selected by a domain-specific log policy.
// The maintenance layer does not discover files or accept caller-provided
// policy; callers must construct the allow-listed paths.
type CleanupRequest struct {
	Paths           []string
	Mode            CleanupMode
	CreateIfMissing bool
	FileMode        os.FileMode
}

type CleanupResult struct {
	ClearedPaths []string
}

type FileMaintenance struct{}

func (FileMaintenance) Cleanup(ctx context.Context, request CleanupRequest) (CleanupResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(request.Paths) == 0 {
		return CleanupResult{}, errors.New("日志维护未提供文件路径")
	}
	if request.Mode != CleanupTruncate && request.Mode != CleanupRemove {
		return CleanupResult{}, errors.New("日志维护操作无效")
	}
	if request.Mode == CleanupRemove && request.CreateIfMissing {
		return CleanupResult{}, errors.New("删除操作不能创建缺失文件")
	}
	fileMode := request.FileMode
	if fileMode == 0 {
		fileMode = 0o600
	}

	result := CleanupResult{ClearedPaths: make([]string, 0, len(request.Paths))}
	seen := make(map[string]struct{}, len(request.Paths))
	for _, rawPath := range request.Paths {
		if err := ctx.Err(); err != nil {
			return CleanupResult{}, err
		}
		if strings.ContainsAny(rawPath, "\x00\r\n") {
			return CleanupResult{}, errors.New("日志维护文件路径无效")
		}
		path := filepath.Clean(strings.TrimSpace(rawPath))
		if path == "." || path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}

		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			if !request.CreateIfMissing || request.Mode != CleanupTruncate {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				return CleanupResult{}, err
			}
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
			if err != nil {
				return CleanupResult{}, err
			}
			if err := file.Close(); err != nil {
				return CleanupResult{}, err
			}
			result.ClearedPaths = append(result.ClearedPaths, path)
			continue
		}
		if err != nil {
			return CleanupResult{}, err
		}
		if !info.Mode().IsRegular() {
			return CleanupResult{}, errors.New("日志维护目标不是普通文件: " + path)
		}

		switch request.Mode {
		case CleanupTruncate:
			if err := os.Truncate(path, 0); err != nil {
				return CleanupResult{}, err
			}
		case CleanupRemove:
			if err := os.Remove(path); err != nil {
				return CleanupResult{}, err
			}
		}
		result.ClearedPaths = append(result.ClearedPaths, path)
	}
	return result, nil
}
