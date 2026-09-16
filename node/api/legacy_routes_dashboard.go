// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import "net/http"

// registerLegacyDashboardRoutes 注册 dashboard 领域尚未迁移的兼容路由。
func registerLegacyDashboardRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v2/dashboard/app/launcher", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardLauncher(w, r)
	})
	mux.HandleFunc("GET /api/v2/dashboard/base/:ioOption/:netOption", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardBase(w, r)
	})
	mux.HandleFunc("GET /api/v2/dashboard/base/os", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardOS(w, r)
	})
	mux.HandleFunc("GET /api/v2/dashboard/current/:ioOption/:netOption", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardCurrent(w, r)
	})
	mux.HandleFunc("GET /api/v2/dashboard/current/node", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardNode(w, r)
	})
	mux.HandleFunc("GET /api/v2/dashboard/current/top/cpu", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardTopCPU(w, r)
	})
	mux.HandleFunc("GET /api/v2/dashboard/current/top/mem", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardTopMem(w, r)
	})
	mux.HandleFunc("GET /api/v2/dashboard/quick/option", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardQuickOption(w, r)
	})
	mux.HandleFunc("POST /api/v2/dashboard/app/launcher/option", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardLauncherOption(w, r)
	})
	mux.HandleFunc("POST /api/v2/dashboard/app/launcher/show", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardMutation(w, r)
	})
	mux.HandleFunc("POST /api/v2/dashboard/quick/change", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardMutation(w, r)
	})
	mux.HandleFunc("POST /api/v2/dashboard/system/restart/:operation", func(w http.ResponseWriter, r *http.Request) {
		handleDashboardRestart(w, r)
	})
}
