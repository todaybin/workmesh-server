// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	Page        int    `json:"page"`
	PageSize    int    `json:"pageSize"`
}

func decodeDatabase(r *http.Request) (databaseRequest, error) {
	var req databaseRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func handleDatabaseSearch(w http.ResponseWriter, r *http.Request) {
	req, err := decodeDatabase(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	items := databaseService.Search(r.Context(), req.Type, req.Name)
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": req.Page, "pageSize": req.PageSize}})
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
