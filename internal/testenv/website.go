// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package testenv 为单元测试隔离网站写入目录，不改变产品的部署路径默认值。
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
)

// RunWebsiteTests 为一个测试进程分配独立的网站及 WAF 根目录并执行用例。
// 不提供生产环境旁路开关；真实部署验收须由独立验收程序执行。
// 清理只针对本函数创建的临时目录，清理失败也会使测试进程失败。
func RunWebsiteTests(run func() int) int {
	root, err := os.MkdirTemp("", "workmesh-website-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建网站测试目录失败:", err)
		return 1
	}
	values := map[string]string{
		"WORKMESH_WEBSITE_ROOT":          filepath.Join(root, "websites"),
		"WORKMESH_WAF_GLOBAL_DIR":        filepath.Join(root, "waf"),
		"WORKMESH_OPENRESTY_CONTAINER":   "workmesh-test-openresty-unavailable",
		"WORKMESH_OPENRESTY_BIN":         filepath.Join(root, "missing-openresty"),
		"WORKMESH_OPENRESTY_CACHE_DIR":   filepath.Join(root, "cache"),
		"WORKMESH_OPENRESTY_REWRITE_DIR": filepath.Join(root, "rewrite"),
		"WORKMESH_WAF_ENFORCE":           "0",
	}
	previous := make(map[string]*string, len(values))
	for key, value := range values {
		if original, exists := os.LookupEnv(key); exists {
			previous[key] = &original
		} else {
			previous[key] = nil
		}
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintln(os.Stderr, "设置网站测试环境失败:", err)
			_ = os.RemoveAll(root)
			return 1
		}
	}
	result := run()
	for key, value := range previous {
		var restoreErr error
		if value == nil {
			restoreErr = os.Unsetenv(key)
		} else {
			restoreErr = os.Setenv(key, *value)
		}
		if restoreErr != nil {
			fmt.Fprintln(os.Stderr, "恢复网站测试环境失败:", restoreErr)
			result = 1
		}
	}
	if err := os.RemoveAll(root); err != nil {
		fmt.Fprintln(os.Stderr, "清理网站测试目录失败:", err)
		result = 1
	}
	return result
}
