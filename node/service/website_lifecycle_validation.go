// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// validateCreatedWebsiteConfig 在站点文件落盘后重新执行 OpenResty 配置检查。
// 此处只运行 -t，不发送 reload 信号；失败由 Create 的延迟清理统一回滚文件。
func validateCreatedWebsiteConfig(ctx context.Context, status OpenRestyStatus) error {
	if !status.Available || !status.ConfigValid || strings.TrimSpace(status.Binary) == "" {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var output []byte
	var err error
	if strings.HasPrefix(status.Binary, "docker://") {
		name := strings.TrimPrefix(status.Binary, "docker://")
		output, err = runContainerOpenRestyCommand(checkCtx, dockerBinaryOrName(), name, "-t")
	} else if !strings.HasPrefix(status.Binary, "proc://") {
		output, err = exec.CommandContext(checkCtx, status.Binary, "-t").CombinedOutput()
	}
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("新网站 OpenResty 配置检查失败: %s", detail)
}
