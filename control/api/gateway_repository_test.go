// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/runtime/gateway"
	_ "modernc.org/sqlite"
)

func TestGatewayBindingRuntimeUsesRepository(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "gateway.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE gateway_binding (id INTEGER PRIMARY KEY CHECK(id=1), status BLOB NOT NULL, auth BLOB NOT NULL, gateway_url TEXT NOT NULL DEFAULT '', account TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := ensureGatewayBindingSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	repository, err := storage.NewSQLiteRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	first := &GatewayStateStore{
		status:     gateway.Status{Registration: gateway.RegistrationRegistered, NodeID: "node-repository", Role: "primary", Connected: true},
		auth:       gateway.Authorization{BindingID: "binding-repository", Scopes: []string{"websites"}, Refreshable: true, AccessToken: "secret-token"},
		gatewayURL: "https://gateway.example.test",
		account:    "operator",
		repository: repository,
	}
	if err := first.persistSnapshot(first.status, first.auth); err != nil {
		t.Fatal(err)
	}

	second := &GatewayStateStore{repository: repository}
	second.load()
	if second.status.NodeID != first.status.NodeID || second.status.Registration != gateway.RegistrationRegistered {
		t.Fatalf("repository 未恢复 Gateway 状态: %+v", second.status)
	}
	if second.auth.BindingID != first.auth.BindingID || second.auth.AccessToken != first.auth.AccessToken || second.account != first.account {
		t.Fatalf("repository 未恢复 Gateway 授权摘要: auth=%+v account=%q", second.auth, second.account)
	}
	if second.gatewayURL != first.gatewayURL {
		t.Fatalf("repository 未恢复 Gateway 地址: got=%q want=%q", second.gatewayURL, first.gatewayURL)
	}
	if err := second.removePersisted(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gateway_binding`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("解绑后 Gateway repository 记录数=%d", count)
	}
}
