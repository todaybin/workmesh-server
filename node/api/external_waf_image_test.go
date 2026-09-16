// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"os"
	"strings"
	"testing"
)

// externalWAFImage 返回当前仓库 WAF 镜像的可覆盖引用。
// 外部黑盒测试默认使用本地构建标签，避免依赖历史镜像。
func externalWAFImage(t *testing.T) string {
	t.Helper()
	if image := strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_WAF_IMAGE")); image != "" {
		return image
	}
	return "workmesh/openresty-waf:local"
}
