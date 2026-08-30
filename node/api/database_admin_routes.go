// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var databaseAdmin = service.NewDatabaseAdminStore()

func readDatabaseBody(r *http.Request) (map[string]any, error) {
	var b map[string]any
	if r.Body == nil {
		return map[string]any{}, nil
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(&b); err != nil && err != io.EOF {
		return nil, err
	}
	if b == nil {
		b = map[string]any{}
	}
	return b, nil
}
func strField(b map[string]any, k string) string {
	if v, ok := b[k].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
func intField(b map[string]any, k string) int64 {
	switch v := b[k].(type) {
	case float64:
		return int64(v)
	case json.Number:
		n, _ := strconv.ParseInt(string(v), 10, 64)
		return n
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}
func dbAdminError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}

// RegisterDatabaseAdminRoutes 注册数据库用户、授权、变量及配置接口。
func RegisterDatabaseAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/databases/users/search", dbUsersSearch)
	mux.HandleFunc("POST /api/v2/databases/users", dbUsersCreate)
	mux.HandleFunc("POST /api/v2/databases/users/del", dbUsersDelete)
	mux.HandleFunc("POST /api/v2/databases/users/update", dbUsersUpdate)
	mux.HandleFunc("POST /api/v2/databases/users/password", dbUsersPassword)
	mux.HandleFunc("POST /api/v2/databases/users/password/save", dbUsersPassword)
	mux.HandleFunc("POST /api/v2/databases/grants/search", dbGrantsSearch)
	mux.HandleFunc("POST /api/v2/databases/grants/summary", dbGrantsSummary)
	mux.HandleFunc("POST /api/v2/databases/grants", dbGrantsCreate)
	mux.HandleFunc("POST /api/v2/databases/grants/del", dbGrantsDelete)
	mux.HandleFunc("POST /api/v2/databases/description/update", dbDescription)
	mux.HandleFunc("POST /api/v2/databases/variables", dbVariables)
	mux.HandleFunc("POST /api/v2/databases/variables/update", dbVariablesUpdate)
	mux.HandleFunc("POST /api/v2/databases/common/info", dbCommonInfo)
	mux.HandleFunc("POST /api/v2/databases/common/load/file", dbCommonFile)
	mux.HandleFunc("POST /api/v2/databases/common/update/conf", dbCommonUpdate)
	mux.HandleFunc("POST /api/v2/databases/format/options", dbFormatOptions)
	mux.HandleFunc("POST /api/v2/databases/status", dbStatus)
	mux.HandleFunc("POST /api/v2/databases/remote", dbRemote)
}
func dbUsersSearch(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	items := databaseAdmin.ListUsers(r.Context(), strField(b, "database"), strField(b, "username"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": len(items)}})
}
func dbUsersCreate(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	u := service.DatabaseUser{Database: strField(b, "database"), Type: strField(b, "type"), Username: strField(b, "username"), Host: strField(b, "host"), Description: strField(b, "description"), DatabaseID: intField(b, "databaseId")}
	u.PasswordSet = strField(b, "password") != ""
	item, e := databaseAdmin.CreateUser(r.Context(), u)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}
func dbUsersDelete(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	id := intField(b, "id")
	if id == 0 {
		id = intField(b, "userId")
	}
	if e = databaseAdmin.DeleteUser(r.Context(), id); e != nil {
		dbAdminError(w, 404, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": id}})
}
func dbUsersUpdate(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	u := service.DatabaseUser{ID: intField(b, "id"), DatabaseID: intField(b, "databaseId"), Database: strField(b, "database"), Type: strField(b, "type"), Username: strField(b, "username"), Host: strField(b, "host"), Description: strField(b, "description")}
	item, e := databaseAdmin.UpdateUser(r.Context(), u)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}
func dbUsersPassword(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	id := intField(b, "id")
	if e = databaseAdmin.SetPassword(r.Context(), id); e != nil {
		dbAdminError(w, 404, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"id": id, "passwordSet": true}})
}
func dbGrantsSearch(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	items := databaseAdmin.ListGrants(r.Context(), strField(b, "database"), strField(b, "username"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
}
func dbGrantsSummary(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	items := databaseAdmin.ListGrants(r.Context(), strField(b, "database"), "")
	summary := map[string][]service.DatabaseGrant{}
	for _, g := range items {
		summary[g.Database] = append(summary[g.Database], g)
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": summary})
}
func dbGrantsCreate(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	g := service.DatabaseGrant{ID: intField(b, "id"), Database: strField(b, "database"), Username: strField(b, "username"), Host: strField(b, "host")}
	if p, ok := b["privileges"].([]any); ok {
		for _, v := range p {
			if s, ok := v.(string); ok {
				g.Privileges = append(g.Privileges, s)
			}
		}
	}
	if len(g.Privileges) == 0 {
		g.Privileges = []string{"SELECT"}
	}
	item, e := databaseAdmin.UpsertGrant(r.Context(), g)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}
func dbGrantsDelete(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	id := intField(b, "id")
	if e = databaseAdmin.DeleteGrant(r.Context(), id); e != nil {
		dbAdminError(w, 404, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": id}})
}
func dbDescription(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	id := intField(b, "id")
	description := strField(b, "description")
	typ := strField(b, "type")
	if typ == "" {
		typ = "mysql"
	}
	port := int(intField(b, "port"))
	if port == 0 {
		port = databasePort(typ, 0)
	}
	item, e := databaseService.Update(r.Context(), service.Database{ID: id, Name: strField(b, "name"), Type: typ, Host: strField(b, "host"), Port: port, Description: description})
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}
func dbVariables(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	items := databaseAdmin.Variables(r.Context(), strField(b, "database"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
}
func dbVariablesUpdate(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	v := service.DatabaseVariable{Database: strField(b, "database"), Name: strField(b, "name"), Value: strField(b, "value")}
	item, e := databaseAdmin.SetVariable(r.Context(), v)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}
func dbCommonInfo(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	typ := strField(b, "type")
	if typ == "" {
		typ = "mysql"
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"type": typ, "database": strField(b, "database"), "host": defaultHost(strField(b, "host")), "port": databasePort(typ, int(intField(b, "port"))), "configurable": true}})
}
func dbCommonFile(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	content := databaseAdmin.Config(r.Context(), strField(b, "database"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": content})
}
func dbCommonUpdate(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	content := strField(b, "content")
	if content == "" {
		if encoded := strField(b, "file"); encoded != "" {
			raw, er := base64.StdEncoding.DecodeString(encoded)
			if er != nil {
				dbAdminError(w, 400, er)
				return
			}
			content = string(raw)
		}
	}
	if e = databaseAdmin.SetConfig(r.Context(), strField(b, "database"), content); e != nil {
		dbAdminError(w, 400, e)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"saved": true}})
}
func dbFormatOptions(w http.ResponseWriter, r *http.Request) {
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": []map[string]string{{"charset": "utf8mb4", "collation": "utf8mb4_unicode_ci"}, {"charset": "utf8mb4", "collation": "utf8mb4_general_ci"}, {"charset": "latin1", "collation": "latin1_swedish_ci"}}})
}
func dbStatus(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	typ := strField(b, "type")
	ok, msg := checkDatabaseEndpoint(strField(b, "host"), typ, int(intField(b, "port")), 0)
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"type": typ, "available": ok, "message": msg}})
}
func dbRemote(w http.ResponseWriter, r *http.Request) {
	b, e := readDatabaseBody(r)
	if e != nil {
		dbAdminError(w, 400, e)
		return
	}
	ok, msg := checkDatabaseEndpoint(strField(b, "host"), strField(b, "type"), int(intField(b, "port")), 0)
	if !ok {
		wmhttp.JSON(w, 503, map[string]any{"code": "ERR", "message": msg})
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"remote": true}})
}
