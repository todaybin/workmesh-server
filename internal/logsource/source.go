// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package logsource provides bounded read-only sources for file-backed logs.
package logsource

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultMaxBytes = 8 << 20
	DefaultMaxLines = 50000
)

type Request struct {
	Paths           []string
	MaxBytesPerFile int64
	MaxLines        int
	FirstAvailable  bool
}

type Line struct {
	Path string
	Text string
}

type Result struct {
	Lines     []Line
	UsedPaths []string
}

type Source interface {
	Read(context.Context, Request) (Result, error)
}

type FileSource struct{}

func (FileSource) Read(ctx context.Context, request Request) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(request.Paths) == 0 {
		return Result{}, errors.New("日志 source 未提供文件路径")
	}
	maxBytes := request.MaxBytesPerFile
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	maxLines := request.MaxLines
	if maxLines <= 0 {
		maxLines = DefaultMaxLines
	}
	result := Result{Lines: make([]Line, 0), UsedPaths: make([]string, 0)}
	seenPaths := map[string]struct{}{}
	for _, rawPath := range request.Paths {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if strings.ContainsAny(rawPath, "\x00\r\n") {
			continue
		}
		path := filepath.Clean(strings.TrimSpace(rawPath))
		if path == "." || path == "" {
			continue
		}
		if _, seen := seenPaths[path]; seen {
			continue
		}
		seenPaths[path] = struct{}{}
		file, err := os.Open(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return Result{}, err
		}
		result.UsedPaths = append(result.UsedPaths, path)
		scanner := bufio.NewScanner(io.LimitReader(file, maxBytes))
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				_ = file.Close()
				return Result{}, err
			}
			result.Lines = append(result.Lines, Line{Path: path, Text: scanner.Text()})
			if len(result.Lines) >= maxLines {
				break
			}
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if scanErr != nil {
			return Result{}, scanErr
		}
		if closeErr != nil {
			return Result{}, closeErr
		}
		if request.FirstAvailable || len(result.Lines) >= maxLines {
			break
		}
	}
	return result, nil
}

var _ Source = FileSource{}
