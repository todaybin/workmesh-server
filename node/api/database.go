// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var databaseService = service.NewDatabaseService(nil)

type databaseRequest struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Description string `json:"description"`
	Password    string `json:"password"` // 仅兼容旧字段，永不落库
	Page        int    `json:"page"`
	PageSize    int    `json:"pageSize"`
}

func decodeDatabase(r *http.Request) (databaseRequest, error) {
	if r.Body == nil {
		return databaseRequest{}, errors.New("请求体不能为空")
	}
	var req databaseRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err == nil {
		req.Name = strings.TrimSpace(req.Name)
		req.Type = strings.ToLower(strings.TrimSpace(req.Type))
		req.Host = strings.TrimSpace(req.Host)
		if req.Page < 0 || req.PageSize < 0 {
			return req, errors.New("分页参数无效")
		}
		if req.PageSize > 100 {
			return req, errors.New("分页大小不能超过100")
		}
	}
	return req, err
}
func handleDatabaseSearch(w http.ResponseWriter, r *http.Request) {
	req, err := decodeDatabase(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	items := databaseService.Search(r.Context(), req.Type, req.Name)
	page, pageSize := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	start := (page - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items[start:end], "total": len(items), "page": page, "pageSize": pageSize}})
}
func handleDatabaseCreate(w http.ResponseWriter, r *http.Request) {
	req, err := decodeDatabase(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	item, err := databaseService.Create(r.Context(), service.Database{Name: req.Name, Type: req.Type, Host: req.Host, Port: req.Port, Username: req.Username, Description: req.Description})
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}

func handleDatabaseUpdate(w http.ResponseWriter, r *http.Request) {
	req, err := decodeDatabase(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if req.ID <= 0 {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "数据库 ID 无效"})
		return
	}
	item, err := databaseService.Update(r.Context(), service.Database{ID: req.ID, Name: req.Name, Type: req.Type, Host: req.Host, Port: req.Port, Username: req.Username, Description: req.Description})
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}
func handleDatabaseCheck(w http.ResponseWriter, r *http.Request) {
	req, err := decodeDatabase(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": databaseService.Check(r.Context(), service.Database{Host: req.Host, Port: req.Port})})
}
func handleDatabaseDelete(w http.ResponseWriter, r *http.Request) {
	req, err := decodeDatabase(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if req.ID == 0 {
		req.ID, _ = strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	}
	if err := databaseService.Delete(r.Context(), req.ID); err != nil {
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}
