// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"os"
	"testing"

	"github.com/todaybin/workmesh-server/internal/testenv"
)

// TestMain 为 API 单元测试隔离所有默认网站写入位置，避免污染本机站点。
func TestMain(tests *testing.M) {
	os.Exit(testenv.RunWebsiteTests(tests.Run))
}
