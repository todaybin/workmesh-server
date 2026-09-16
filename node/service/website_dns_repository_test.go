// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestDNSAccountRepositoryPreservesCredentialsAndReferenceGuard(t *testing.T) {
	root := t.TempDir()
	svc := NewWebsiteService(root)
	created, err := svc.UpsertDNSAccount(DNSAccount{
		Name:        "test-dns",
		Type:        "TencentCloud",
		Credentials: map[string]any{"secretID": "id", "secretKey": "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Authorization != nil || created.Credentials != nil {
		t.Fatalf("credentials leaked from response: %#v", created)
	}

	updated, err := svc.UpsertDNSAccount(DNSAccount{ID: created.ID, Name: "test-dns-renamed", Provider: "TencentCloud"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CreatedAt.IsZero() || updated.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not restored: %#v", updated)
	}
	var credentials string
	if err := svc.db.QueryRow(`SELECT credentials FROM website_dns_accounts WHERE id=?`, created.ID).Scan(&credentials); err != nil {
		t.Fatal(err)
	}
	if credentials != `{"secretID":"id","secretKey":"secret"}` {
		t.Fatalf("credentials overwritten by metadata-only update: %s", credentials)
	}

	if _, err := svc.db.Exec(`INSERT INTO website_ssls(id,primary_domain,dns_account_id,created_at,updated_at) VALUES(?,?,?,?,?)`, 901, "dns-ref.example", created.ID, "now", "now"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteDNSAccount(created.ID); err == nil || err.Error() != "DNS 账户已被证书引用，不能删除" {
		t.Fatalf("referenced DNS account delete error=%v", err)
	}
	if _, err := svc.db.Exec(`DELETE FROM website_ssls WHERE id=?`, 901); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteDNSAccount(created.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteDNSAccount(created.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing DNS account delete error=%v", err)
	}
}

func TestWebsiteSecurityRepositoryPersistsACMEAccount(t *testing.T) {
	root := t.TempDir()
	svc := NewWebsiteSecurityService(root)
	svc.SetACMERegistrar(fakeACMERegistrar{})
	item, err := svc.CreateACME("repo@example.com", "letsencrypt", "EC256", "", "", "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := NewWebsiteSecurityService(root)
	total, accounts := reloaded.ListACME("repo@example.com", 1, 20)
	if total != 1 || len(accounts) != 1 || accounts[0].ID != item.ID || accounts[0].PrivateKey != "" {
		t.Fatalf("ACME repository reload=%d %#v", total, accounts)
	}
}

type fakeACMERegistrar struct{}

func (fakeACMERegistrar) Register(_ context.Context, item WebsiteACMEAccount) (WebsiteACMEAccount, error) {
	item.URL = "https://acme.example/directory"
	item.PrivateKey = "private-key"
	return item, nil
}
