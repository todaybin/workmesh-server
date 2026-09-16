// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
)

func TestSSHCertLifecycleUsesIsolatedFiles(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("WORKMESH_HOST_CREDENTIAL_KEY", "test-key")
	store, err := storage.Open(filepath.Join(root, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := SetSharedStore(store); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerHostSSHCertRoutes(mux)
	public := "ssh-ed25519 AAAATEST workmesh"
	private := "-----BEGIN OPENSSH PRIVATE KEY-----\nTEST\n-----END OPENSSH PRIVATE KEY-----\n"
	createBody := `{"name":"acceptance-key","mode":"input","encryptionMode":"ed25519","publicKey":"` + base64.StdEncoding.EncodeToString([]byte(public)) + `","privateKey":"` + base64.StdEncoding.EncodeToString([]byte(private)) + `","description":"test"}`
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/cert", bytes.NewBufferString(createBody)))
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "acceptance-key")); err != nil {
		t.Fatal(err)
	}
	authorized, _ := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if !strings.Contains(string(authorized), public) {
		t.Fatalf("public key was not authorized: %s", authorized)
	}

	search := httptest.NewRecorder()
	mux.ServeHTTP(search, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/cert/search", bytes.NewBufferString(`{"page":1,"pageSize":20}`)))
	var payload struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if search.Code != http.StatusOK || json.Unmarshal(search.Body.Bytes(), &payload) != nil || len(payload.Data.Items) != 1 {
		t.Fatalf("search status=%d body=%s", search.Code, search.Body.String())
	}
	id := int(payload.Data.Items[0]["id"].(float64))
	update := httptest.NewRecorder()
	updateBody := `{"id":0,"name":"acceptance-key-2","encryptionMode":"ed25519","publicKey":"` + base64.StdEncoding.EncodeToString([]byte(public+"-updated")) + `","privateKey":"` + base64.StdEncoding.EncodeToString([]byte(private+"updated")) + `"}`
	// Use JSON encoding to avoid assumptions about the SQLite autoincrement width.
	var updateMap map[string]any
	_ = json.Unmarshal([]byte(updateBody), &updateMap)
	updateMap["id"] = id
	encodedUpdate, _ := json.Marshal(updateMap)
	mux.ServeHTTP(update, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/cert/update", bytes.NewReader(encodedUpdate)))
	if update.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", update.Code, update.Body.String())
	}
	remove := httptest.NewRecorder()
	mux.ServeHTTP(remove, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/ssh/cert/delete", bytes.NewBufferString(`{"ids":[`+itoa(id)+`]}`)))
	if remove.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "acceptance-key-2")); !os.IsNotExist(err) {
		t.Fatalf("private key remains: %v", err)
	}
}
