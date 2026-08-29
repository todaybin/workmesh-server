// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestComposeRequiresPath(t *testing.T) {
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/containers/compose/operate", bytes.NewBufferString(`{"operation":"up"}`))
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusInternalServerError || !bytes.Contains(res.Body.Bytes(), []byte("Compose")) {
		t.Fatalf("unexpected response: %d %s", res.Code, res.Body.String())
	}
}
