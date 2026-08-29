// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebsiteWAFRoutesCRUD(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	create := httptest.NewRequest(http.MethodPost, "/api/v2/websites", bytes.NewBufferString(`{"primaryDomain":"example.com","alias":"站点"}`))
	create.Header.Set("Content-Type", "application/json")
	record := httptest.NewRecorder()
	mux.ServeHTTP(record, create)
	if record.Code != http.StatusOK {
		t.Fatalf("创建网站状态码: %d %s", record.Code, record.Body.String())
	}
	var envelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(record.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ID == 0 {
		t.Fatal("创建响应缺少 ID")
	}
	rulesBody := bytes.NewBufferString(`{"websiteID":1,"name":"阻止管理路径","location":"uri","operator":"contains","value":"/admin","action":"block","enabled":true}`)
	rules := httptest.NewRequest(http.MethodPost, "/api/v2/websites/waf/rules", rulesBody)
	rules.Header.Set("Content-Type", "application/json")
	rulesRecord := httptest.NewRecorder()
	mux.ServeHTTP(rulesRecord, rules)
	if rulesRecord.Code != http.StatusOK {
		t.Fatalf("新增规则状态码: %d %s", rulesRecord.Code, rulesRecord.Body.String())
	}
	get := httptest.NewRequest(http.MethodGet, "/api/v2/websites/waf/sites/1/rules", nil)
	getRecord := httptest.NewRecorder()
	mux.ServeHTTP(getRecord, get)
	if getRecord.Code != http.StatusOK || !bytes.Contains(getRecord.Body.Bytes(), []byte("阻止管理路径")) {
		t.Fatalf("规则查询失败: %d %s", getRecord.Code, getRecord.Body.String())
	}
}
