// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeploymentManifestVerify(t *testing.T) {
	content := "workmesh-artifact"
	sum := sha256.Sum256([]byte(content))
	mux := http.NewServeMux()
	registerDeploymentAndProcessRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v2/deployment-manifest/verify", bytes.NewBufferString(`{"content":"`+content+`","sha256":"`+hex.EncodeToString(sum[:])+`"}`))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"verified":true`) {
		t.Fatalf("verify response: %d %s", res.Code, res.Body.String())
	}
}

func TestProcessStopRequiresToken(t *testing.T) {
	t.Setenv("WORKMESH_PROCESS_TOKEN", "expected")
	mux := http.NewServeMux()
	registerDeploymentAndProcessRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/process/stop", strings.NewReader(`{"pid":99999}`)))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.Code)
	}
}
