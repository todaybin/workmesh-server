// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func registerToolboxClamRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/toolbox/clam/base", func(w http.ResponseWriter, r *http.Request) {
		runtimeOK(w, clamBaseInfo(r.Context()))
	})
	mux.HandleFunc("POST /api/v2/toolbox/clam/operate", handleClamOperate)
	mux.HandleFunc("POST /api/v2/toolbox/clam/search", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil || body == nil {
			body = map[string]any{}
		}
		runtimeOK(w, pageRecordsGeneric([]map[string]any{}, body))
	})
	for _, path := range []string{"/api/v2/toolbox/clam", "/api/v2/toolbox/clam/update", "/api/v2/toolbox/clam/del", "/api/v2/toolbox/clam/handle", "/api/v2/toolbox/clam/status/update", "/api/v2/toolbox/clam/record/search", "/api/v2/toolbox/clam/record/clean", "/api/v2/toolbox/clam/file/search", "/api/v2/toolbox/clam/file/update"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, _ *http.Request) {
			runtimeErr(w, http.StatusNotImplemented, "病毒扫描任务尚未接入")
		})
	}
}

func clamBaseInfo(ctx context.Context) map[string]any {
	scanner := ""
	for _, name := range []string{"clamdscan", "clamscan"} {
		if _, err := hostBinary(name); err == nil {
			scanner = name
			break
		}
	}
	_, freshErr := hostBinary("freshclam")
	version := ""
	if scanner != "" {
		version = commandVersion(ctx, scanner, "--version")
	}
	freshVersion := ""
	if freshErr == nil {
		freshVersion = commandVersion(ctx, "freshclam", "--version")
	}
	return map[string]any{
		"isExist": scanner != "", "isActive": systemdUnitActive(ctx, "clamav-daemon", "clamd"), "version": version,
		"freshIsExist": freshErr == nil, "freshIsActive": systemdUnitActive(ctx, "clamav-freshclam"), "freshVersion": freshVersion,
	}
}

func handleClamOperate(w http.ResponseWriter, r *http.Request) {
	if err := mutationRequired(); err != nil {
		runtimeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	body, err := runtimeBody(r)
	if err != nil {
		runtimeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	action := strings.ToLower(runtimeString(body, "operation", "operate"))
	if action != "start" && action != "stop" && action != "restart" {
		runtimeErr(w, http.StatusBadRequest, "ClamAV 操作必须是 start、stop 或 restart")
		return
	}
	if !clamBaseInfo(r.Context())["isExist"].(bool) {
		runtimeErr(w, http.StatusServiceUnavailable, "ClamAV 未安装")
		return
	}
	unit := "clamav-daemon"
	if !hostServiceUnitExists(r.Context(), unit) {
		unit = "clamd"
	}
	if _, err := hostBinary("systemctl"); err != nil {
		runtimeErr(w, http.StatusServiceUnavailable, "systemctl 未安装")
		return
	}
	if _, err := hostCommand(r.Context(), 20*time.Second, "systemctl", action, unit); err != nil {
		runtimeErr(w, http.StatusBadGateway, "执行 ClamAV 操作失败")
		return
	}
	runtimeOK(w, map[string]any{"operation": action, "service": unit})
}

func hostServiceUnitExists(ctx context.Context, unit string) bool {
	if _, err := hostBinary("systemctl"); err != nil {
		return false
	}
	result, err := hostCommand(ctx, 5*time.Second, "systemctl", "cat", unit)
	return err == nil && result.ExitCode == 0
}
