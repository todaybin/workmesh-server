package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
)

type testACMERegistrar struct {
	account WebsiteACMEAccount
	err     error
}

func (r testACMERegistrar) Register(_ context.Context, account WebsiteACMEAccount) (WebsiteACMEAccount, error) {
	if r.err != nil {
		return account, r.err
	}
	account.URL = "https://acme.test/acct/1"
	account.PrivateKey = "-----BEGIN RSA PRIVATE KEY-----\nredacted\n-----END RSA PRIVATE KEY-----"
	return account, nil
}

func TestACMERegistrationPersistsOnlyAfterExternalSuccess(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(filepath.Join(root, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	t.Cleanup(func() {
		websiteSecurityDBMu.Lock()
		websiteSecurityDB = nil
		websiteSecurityDBMu.Unlock()
	})
	if err := SetWebsiteSecurityDB(store.DB()); err != nil {
		t.Fatal(err)
	}
	security := NewWebsiteSecurityService(root)
	security.SetACMERegistrar(testACMERegistrar{err: errors.New("ca unavailable")})
	if _, err := security.CreateACMEContext(context.Background(), "rollback@example.net", "custom", "RSA2048", "", "", "https://acme.test/directory", false, false); err == nil {
		t.Fatal("external registration failure should be returned")
	}
	var count int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM website_acme_accounts WHERE email=?`, "rollback@example.net").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed ACME registration left SQLite row: %d", count)
	}

	security.SetACMERegistrar(testACMERegistrar{})
	created, err := security.CreateACMEContext(context.Background(), "persist@example.net", "custom", "RSA2048", "", "", "https://acme.test/directory", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.URL == "" || created.PrivateKey != "" {
		t.Fatalf("ACME response should contain public fields only: %+v", created)
	}
	_, items := security.ListACME("persist@example.net", 1, 10)
	if len(items) != 1 || items[0].PrivateKey != "" || items[0].EabHmacKey != "" {
		t.Fatalf("ACME list leaked or omitted account: %+v", items)
	}
}
