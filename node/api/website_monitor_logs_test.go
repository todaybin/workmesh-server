// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/node/service"
)

// TestWebsiteMonitorLogsReadSiteFile 验证 websiteID 会隔离站点日志来源，且清理操作落到真实文件。
func TestWebsiteMonitorLogsReadSiteFile(t *testing.T) {
	dataRoot := t.TempDir()
	websiteRoot := filepath.Join(dataRoot, "wwwroot")
	t.Setenv("WORKMESH_DATA_DIR", dataRoot)
	t.Setenv("WORKMESH_WEBSITE_ROOT", websiteRoot)
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	call := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	if res := call("/api/v2/websites", `{"primaryDomain":"monitor-site.example"}`); res.Code != http.StatusOK {
		t.Fatalf("创建网站失败: %d %s", res.Code, res.Body.String())
	}
	svc := service.NewWebsiteService("")
	site, err := svc.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UTC().Format("02/Jan/2006:15:04:05 +0000")
	access := "198.51.100.20 - - [" + stamp + "] \"GET /site-only HTTP/1.1\" 200 42 \"-\" \"monitor-test\"\n"
	logPath := filepath.Join(svc.SitePath(site, "logs"), "access.log")
	if err := os.WriteFile(logPath, []byte(access), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataRoot, "logs"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "logs", "access.log"), []byte("203.0.113.9 - - ["+stamp+"] \"GET /other HTTP/1.1\" 200 10 \"-\" \"global\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	search := call("/api/v2/websites/monitor/logs/search", `{"websiteID":1,"page":1,"pageSize":20}`)
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), "/site-only") || strings.Contains(search.Body.String(), "/other") {
		t.Fatalf("未按站点读取真实日志: %d %s", search.Code, search.Body.String())
	}
	var envelope struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &envelope); err != nil || envelope.Data.Total != 1 {
		t.Fatalf("站点日志总数错误: %d %v", envelope.Data.Total, err)
	}
	xpackSearch := call("/api/v2/xpack/monitor/logs/search", `{"websiteID":1,"page":1,"pageSize":20}`)
	if xpackSearch.Code != http.StatusOK || !strings.Contains(xpackSearch.Body.String(), "/site-only") || !strings.Contains(xpackSearch.Body.String(), `"items"`) {
		t.Fatalf("xpack 监控日志别名未转发到真实日志处理器: %d %s", xpackSearch.Code, xpackSearch.Body.String())
	}
	wafLogDir := filepath.Join(svc.SitePath(site, "waf"), "logs")
	if err := os.MkdirAll(wafLogDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wafLogDir, "audit.jsonl"), []byte(`{"timestamp":"2026-09-05T01:02:03Z","client_ip":"198.51.100.20","rule":"SQL-1"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wafSearch := call("/api/v2/xpack/waf/log/search", `{"websiteID":1,"page":1,"pageSize":20}`)
	if wafSearch.Code != http.StatusOK || !strings.Contains(wafSearch.Body.String(), "SQL-1") || !strings.Contains(wafSearch.Body.String(), `"items"`) {
		t.Fatalf("xpack WAF 日志别名未读取文件日志: %d %s", wafSearch.Code, wafSearch.Body.String())
	}
	clear := call("/api/v2/websites/monitor/logs/clear", `{"websiteID":1}`)
	if clear.Code != http.StatusOK || !strings.Contains(clear.Body.String(), `"cleared":true`) {
		t.Fatalf("真实日志清理失败: %d %s", clear.Code, clear.Body.String())
	}
	data, err := os.ReadFile(logPath)
	if err != nil || len(data) != 0 {
		t.Fatalf("日志文件未清空: len=%d err=%v", len(data), err)
	}
}
