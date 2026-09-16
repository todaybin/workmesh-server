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
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Host          string `json:"host"`
	ContainerName string `json:"containerName"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	Description   string `json:"description"`
	Version       string `json:"version"`
	From          string `json:"from"`
	Address       string `json:"address"`
	InitialDB     string `json:"initialDB"`
	SSL           bool   `json:"ssl"`
	Password      string `json:"password"`
	Timeout       int    `json:"timeout"`
	Page          int    `json:"page"`
	PageSize      int    `json:"pageSize"`
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
		req.ContainerName = strings.TrimSpace(req.ContainerName)
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
	if req.Type == "" {
		req.Type = "mysql"
	}
	if req.Host == "" {
		req.Host = req.Address
	}
	req.Host, req.ContainerName = normalizedDatabaseTarget(req.From, req.Host, req.ContainerName)
	// /databases/db is the generic metadata registration endpoint. Real database
	// connectivity is handled by /databases/db/check and type-specific create
	// handlers such as POST /api/v2/databases.
	item, err := databaseService.Create(r.Context(), service.Database{Name: req.Name, Type: req.Type, Version: req.Version, From: databaseSourceForTarget(req.From, req.ContainerName), Host: req.Host, Port: req.Port, ContainerName: req.ContainerName, InitialDB: req.InitialDB, Username: req.Username, Password: decodeDatabaseSecret(req.Password), SSL: req.SSL, Description: req.Description})
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
	current, found := databaseService.Find(r.Context(), req.ID)
	if !found {
		wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": "数据库不存在"})
		return
	}
	if req.Name == "" {
		req.Name = current.Name
	}
	if req.Type == "" {
		req.Type = current.Type
	}
	if req.From == "" {
		req.From = current.From
	}
	if req.Version == "" {
		req.Version = current.Version
	}
	if req.InitialDB == "" {
		req.InitialDB = current.InitialDB
	}
	if req.Username == "" {
		req.Username = current.Username
	}
	if req.Host == "" {
		req.Host = req.Address
	}
	if req.Host == "" {
		req.Host = current.Host
	}
	if req.ContainerName == "" {
		req.ContainerName = current.ContainerName
	}
	if req.Port == 0 {
		req.Port = current.Port
	}
	req.Host, req.ContainerName = normalizedDatabaseTarget(req.From, req.Host, req.ContainerName)
	if requiresDatabaseConnectionCheck(req.Type) {
		checkReq := req
		if checkReq.Password == "" {
			checkReq.Password = current.Password
		}
		if err = checkDatabaseConnection(r.Context(), checkReq); err != nil {
			wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
	}
	item, err := databaseService.Update(r.Context(), service.Database{ID: req.ID, Name: req.Name, Type: req.Type, Version: req.Version, From: req.From, Host: req.Host, Port: req.Port, ContainerName: req.ContainerName, InitialDB: req.InitialDB, Username: req.Username, Password: decodeDatabaseSecret(req.Password), SSL: req.SSL, Description: req.Description})
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
	if req.Host == "" {
		req.Host = req.Address
	}
	if req.ID > 0 {
		if current, found := databaseService.Find(r.Context(), req.ID); found {
			if req.Type == "" {
				req.Type = current.Type
			}
			if req.From == "" {
				req.From = current.From
			}
			if req.Host == "" {
				req.Host = current.Host
			}
			if req.ContainerName == "" {
				req.ContainerName = current.ContainerName
			}
			if req.Port == 0 {
				req.Port = current.Port
			}
			if req.Username == "" {
				req.Username = current.Username
			}
			if req.InitialDB == "" {
				req.InitialDB = current.InitialDB
			}
			if req.Password == "" {
				req.Password = current.Password
			}
		}
	}
	req.Host, req.ContainerName = normalizedDatabaseTarget(req.From, req.Host, req.ContainerName)
	if err = checkDatabaseConnection(r.Context(), req); err != nil {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": true})
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
	if err := deleteRegisteredDatabase(r.Context(), req.ID); err != nil {
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}
