// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"os"
	"testing"

	"github.com/todaybin/workmesh-server/internal/testenv"
)

// TestMain 为服务单元测试隔离默认网站与 WAF 目录，不改变产品目录语义。
func TestMain(tests *testing.M) {
	os.Exit(testenv.RunWebsiteTests(tests.Run))
}
