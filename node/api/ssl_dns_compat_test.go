package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/node/service"
)

func TestSSLLogReadUsesCertificateID(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	RegisterSSLRoutes(mux)
	registerFileRoutes(mux)
	create := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/websites/ssl", bytes.NewBufferString(`{"primaryDomain":"ssl-log.example","provider":"letsencrypt"}`))
	mux.ServeHTTP(create, req)
	if create.Code != http.StatusOK {
		t.Fatalf("create ssl: %d %s", create.Code, create.Body.String())
	}
	var envelope struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &envelope); err != nil || envelope.Data.ID == 0 {
		t.Fatalf("ssl id: %v %s", err, create.Body.String())
	}
	path, err := service.NewSSLService().LogPath(context.Background(), envelope.Data.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("申请开始\n申请完成\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	read := httptest.NewRecorder()
	readReq := httptest.NewRequest(http.MethodPost, "/api/v2/files/read/ssl?operateNode=primary-main", bytes.NewBufferString(`{"ID":1,"page":1,"pageSize":1}`))
	mux.ServeHTTP(read, readReq)
	if read.Code != http.StatusOK || strings.Contains(read.Body.String(), "文件路径不能为空") {
		t.Fatalf("ssl log read: %d %s", read.Code, read.Body.String())
	}
	if !strings.Contains(read.Body.String(), "申请开始") || !strings.Contains(read.Body.String(), `"totalLines":2`) {
		t.Fatalf("ssl log payload: %s", read.Body.String())
	}
}

func TestDNSAccountAcceptsOnePanelFields(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/websites/dns", bytes.NewBufferString(`{"id":0,"name":"tx","type":"TencentCloud","authorization":{"secretID":"redacted","secretKey":"redacted"}}`))
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("dns create: %d %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"type":"TencentCloud"`) || strings.Contains(res.Body.String(), "secretKey") {
		t.Fatalf("dns response compatibility/secrecy: %s", res.Body.String())
	}
	search := httptest.NewRecorder()
	mux.ServeHTTP(search, httptest.NewRequest(http.MethodPost, "/api/v2/websites/dns/search", bytes.NewBufferString(`{"page":1,"pageSize":10}`)))
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), `"type":"TencentCloud"`) {
		t.Fatalf("dns search: %d %s", search.Code, search.Body.String())
	}
}
