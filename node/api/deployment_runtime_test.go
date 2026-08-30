// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestRecoverDeploymentStateRestoresValidArtifact(t *testing.T) {
	dataDir := t.TempDir()
	target := filepath.Join(dataDir, "releases", "current.artifact")
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		t.Fatal(err)
	}
	content := []byte("persisted release")
	if err := os.WriteFile(target, content, 0o750); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	state := map[string]any{"mode": "update", "target": target, "sha256": hex.EncodeToString(sum[:]), "at": time.Now().UTC()}
	raw, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(dataDir, "deployment-artifact.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	localDeployment.mu.Lock()
	localDeployment.Active, localDeployment.Status, localDeployment.LastVerify = "", "idle", ""
	localDeployment.mu.Unlock()
	if err := RecoverDeploymentState(dataDir); err != nil {
		t.Fatalf("恢复有效制品失败: %v", err)
	}
	snapshot := deploymentStateSnapshot()
	if snapshot.Active != target || snapshot.Status != "active" || snapshot.LastVerify != hex.EncodeToString(sum[:]) {
		t.Fatalf("恢复状态错误: %#v", snapshot)
	}
}

func TestRecoverDeploymentStateRejectsInvalidArtifact(t *testing.T) {
	dataDir := t.TempDir()
	target := filepath.Join(dataDir, "releases", "current.artifact")
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(dataDir), "outside.artifact")
	for _, state := range []map[string]any{
		{"target": target, "sha256": strings.Repeat("0", 64)},
		{"target": outside, "sha256": strings.Repeat("0", 64)},
	} {
		raw, _ := json.Marshal(state)
		if err := os.WriteFile(filepath.Join(dataDir, "deployment-artifact.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RecoverDeploymentState(dataDir); err == nil {
			t.Fatalf("无效部署状态应拒绝: %#v", state)
		}
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
