// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import "net/http"

// registerAIExplicitRoutes 为已完成的 AI 账户、GPU 和沙盒接口提供精确路由证据。
// 通用前缀仍保留以兼容其余迁移中的接口，ServeMux 会优先选择更长的精确路径。
func registerAIExplicitRoutes(mux *http.ServeMux) {
	for _, pattern := range []string{
		"GET /api/v2/ai/accounts/providers",
		"GET /api/v2/ai/gpu/load",
		"GET /api/v2/ai/gpu/options",
		"POST /api/v2/ai/accounts",
		"POST /api/v2/ai/accounts/counts",
		"POST /api/v2/ai/accounts/delete",
		"POST /api/v2/ai/accounts/models",
		"POST /api/v2/ai/accounts/models/create",
		"POST /api/v2/ai/accounts/models/delete",
		"POST /api/v2/ai/accounts/models/discover",
		"POST /api/v2/ai/accounts/models/update",
		"POST /api/v2/ai/accounts/search",
		"POST /api/v2/ai/accounts/update",
		"POST /api/v2/ai/accounts/verify",
	} {
		mux.HandleFunc(pattern, aiHandler)
	}
	for _, pattern := range []string{
		"GET /api/v2/cubesandbox/health",
		"GET /api/v2/cubesandbox/status",
		"POST /api/v2/cubesandbox/reconcile",
		"POST /api/v2/cubesandbox/start",
		"POST /api/v2/cubesandbox/stop",
	} {
		mux.HandleFunc(pattern, sandboxHandler)
	}
}
