// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

func writeError(w http.ResponseWriter, status int, err error) {
	message := "请求失败"
	if err != nil {
		message = err.Error()
	}
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": message})
}
