// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package log 提供低开销结构化日志和本地文件轮转能力。
package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	defaultMaxBytes = 10 << 20
	defaultBackups  = 5
)

// Config 描述日志输出位置和保留策略。
type Config struct {
	Path       string
	MaxBytes   int64
	MaxBackups int
}

// New 创建结构化日志实例；显式传入 writer 时不创建文件，便于测试和嵌入调用。
func New(output io.Writer) *slog.Logger {
	if output == nil {
		if path := strings.TrimSpace(os.Getenv("WORKMESH_LOG_FILE")); path != "" {
			if writer, err := Open(Config{Path: path}); err == nil {
				return slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo}))
			}
		}
		output = os.Stderr
	}
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// RotatingWriter 是线程安全的按大小轮转日志写入器。
type RotatingWriter struct {
	mu         sync.Mutex
	file       *os.File
	path       string
	maxBytes   int64
	maxBackups int
	size       int64
}

// Open 创建日志文件写入器并恢复已有文件大小。
func Open(cfg Config) (*RotatingWriter, error) {
	if strings.TrimSpace(cfg.Path) == "" {
		return nil, os.ErrInvalid
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = defaultMaxBytes
	}
	if cfg.MaxBackups <= 0 {
		cfg.MaxBackups = defaultBackups
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, err
	}
	info, _ := f.Stat()
	var size int64
	if info != nil {
		size = info.Size()
	}
	return &RotatingWriter{file: f, path: cfg.Path, maxBytes: cfg.MaxBytes, maxBackups: cfg.MaxBackups, size: size}, nil
}

// Write 写入日志并在达到上限时执行轮转。
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if int64(len(p))+w.size > w.maxBytes {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// Close 刷新并关闭当前日志文件。
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Sync()
	closeErr := w.file.Close()
	w.file = nil
	if err != nil {
		return err
	}
	return closeErr
}

func (w *RotatingWriter) rotateLocked() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	for i := w.maxBackups - 1; i >= 1; i-- {
		old := w.path + "." + strconv.Itoa(i)
		next := w.path + "." + strconv.Itoa(i+1)
		if _, err := os.Stat(old); err == nil {
			_ = os.Remove(next)
			_ = os.Rename(old, next)
		}
	}
	_ = os.Remove(w.path + ".1")
	if err := os.Rename(w.path, w.path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	w.file, w.size = f, 0
	return nil
}
