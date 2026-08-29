// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("请求体不能为空")
	}
	return json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(target)
}

func writeError(w http.ResponseWriter, status int, err error) {
	message := "请求失败"
	if err != nil {
		message = err.Error()
	}
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": message})
}
