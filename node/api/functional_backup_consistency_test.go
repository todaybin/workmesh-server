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
)

func TestBackupAccountMutationsRestoreMemoryOnSaveFailure(t *testing.T) {
	tests := []struct {
		name  string
		call  func(http.ResponseWriter, *http.Request, *domainStore)
		body  string
		state domainState
		check func(t *testing.T, got domainState)
	}{
		{
			name:  "create",
			call:  handleBackupAccountCreate,
			body:  `{"name":"new-account","type":"s3"}`,
			state: domainState{BackupAccounts: []backupAccount{{ID: "existing", Name: "existing", Type: "s3"}}},
			check: func(t *testing.T, got domainState) {
				if len(got.BackupAccounts) != 1 || got.BackupAccounts[0].ID != "existing" {
					t.Fatalf("create left memory mutation: %#v", got.BackupAccounts)
				}
			},
		},
		{
			name:  "update",
			call:  handleBackupAccountUpdate,
			body:  `{"id":"existing","name":"updated"}`,
			state: domainState{BackupAccounts: []backupAccount{{ID: "existing", Name: "existing", Type: "s3"}}},
			check: func(t *testing.T, got domainState) {
				if len(got.BackupAccounts) != 1 || got.BackupAccounts[0].Name != "existing" {
					t.Fatalf("update left memory mutation: %#v", got.BackupAccounts)
				}
			},
		},
		{
			name:  "delete",
			call:  handleBackupAccountDelete,
			body:  `{"id":"existing"}`,
			state: domainState{BackupAccounts: []backupAccount{{ID: "existing", Name: "existing", Type: "s3"}}},
			check: func(t *testing.T, got domainState) {
				if len(got.BackupAccounts) != 1 || got.BackupAccounts[0].ID != "existing" {
					t.Fatalf("delete left memory mutation: %#v", got.BackupAccounts)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			blocker := filepath.Join(root, "save-blocker")
			if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
				t.Fatal(err)
			}
			store := &domainStore{
				path:  filepath.Join(blocker, "state.json"),
				state: tc.state,
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v2/backups", strings.NewReader(tc.body))
			tc.call(recorder, request, store)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			tc.check(t, store.state)
		})
	}
}

func TestBackupRecordDeleteSaveFailureKeepsRecordAndFile(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	backupPath := filepath.Join(backupDataDir(), "snapshot.txt")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "save-blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	record := backupItem{ID: "record-1", Path: backupPath, FileName: "snapshot.txt"}
	store := &domainStore{
		path:  filepath.Join(blocker, "state.json"),
		state: domainState{Backups: []backupItem{record}},
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/backups/record/del", strings.NewReader(`{"id":"record-1"}`))
	handleBackupRecordDelete(response, request, store)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(store.state.Backups) != 1 || store.state.Backups[0].ID != record.ID {
		t.Fatalf("record was not restored: %#v", store.state.Backups)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file was removed before save succeeded: %v", err)
	}
}

func TestBackupRecordDeleteRemoteFailureRestoresRecordBeforeLocalDelete(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	backupPath := filepath.Join(backupDataDir(), "snapshot.txt")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	vars, err := json.Marshal(map[string]any{"delete_url": "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	account := backupAccount{ID: "account-1", Name: "remote", Type: "s3", Vars: string(vars), BackupPath: "remote-backups"}
	record := backupItem{
		ID:                "record-1",
		Path:              backupPath,
		FileName:          "snapshot.txt",
		DownloadAccountID: account.ID,
	}
	store := &domainStore{
		path:  filepath.Join(t.TempDir(), "state.json"),
		state: domainState{Backups: []backupItem{record}, BackupAccounts: []backupAccount{account}},
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v2/backups/record/del", bytes.NewBufferString(`{"id":"record-1"}`))
	handleBackupRecordDelete(response, request, store)

	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "BACKUP_PROVIDER_DELETE") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(store.state.Backups) != 1 || store.state.Backups[0].ID != record.ID {
		t.Fatalf("record was not restored after remote failure: %#v", store.state.Backups)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("local backup file was removed before remote deletion succeeded: %v", err)
	}
	persisted, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(persisted, []byte(`"id":"record-1"`)) {
		t.Fatalf("restored record was not persisted: %s", persisted)
	}
}
