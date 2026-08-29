// SPDX-License-Identifier: LicenseRef-WorkMesh-Pending
// Copyright (c) 2026 WorkMesh contributors

package log

import (
	"io"
	"log/slog"
	"os"
)

// New 创建单一结构化日志实例，避免 Core 与 Agent 分别初始化 writer。
func New(output io.Writer) *slog.Logger {
	if output == nil {
		output = os.Stderr
	}
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
