// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDatabaseCreateAndSearch(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostContainerCronRoutes(mux)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/databases/db", bytes.NewBufferString(`{"name":"local","type":"sqlite","host":"127.0.0.1","port":1}`))
	mux.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("create status=%d body=%s", res.Code, res.Body.String())
	}
	res = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v2/databases/db/search", bytes.NewBufferString(`{"type":"sqlite"}`))
	mux.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("search status=%d", res.Code)
	}
}
