// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

func TestFileAuxAndSharesLegacyImportToSQLite(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	aux := fileAuxState{Favorites: []fileFavorite{{ID: "fav-1", Path: "/tmp/demo", Name: "demo", CreatedAt: time.Now().UTC()}}, Recycle: []fileRecycleItem{}, Uploads: []fileUploadItem{}, Remarks: map[string]string{}, History: []fileHistoryItem{}, ConvertLogs: []fileConvertLog{}}
	auxRaw, err := json.Marshal(aux)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "files.json"), auxRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	shares := map[string]fileShare{"tok-1": {Token: "tok-1", Path: "/tmp/demo", CreatedAt: time.Now().UTC()}}
	shareRaw, err := json.Marshal(shares)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "file-shares.json"), shareRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		resetSharedStoreForTest()
		_ = store.Close()
		fileAux.Lock()
		fileAux.loaded, fileAux.path = false, ""
		fileAux.Unlock()
		fileShareState.Lock()
		fileShareState.loaded, fileShareState.items = false, nil
		fileShareState.Unlock()
	}()
	if err := SetSharedStore(store); err != nil {
		t.Fatal(err)
	}
	fileAux.Lock()
	loadFileAuxLocked()
	if len(fileAux.data.Favorites) != 1 || fileAux.data.Favorites[0].ID != "fav-1" {
		fileAux.Unlock()
		t.Fatalf("文件辅助状态导入失败: %#v", fileAux.data.Favorites)
	}
	if err := saveFileAuxLocked(); err != nil {
		fileAux.Unlock()
		t.Fatal(err)
	}
	fileAux.Unlock()
	fileShareState.Lock()
	loadFileSharesLocked()
	if _, ok := fileShareState.items["tok-1"]; !ok {
		fileShareState.Unlock()
		t.Fatalf("文件分享状态导入失败: %#v", fileShareState.items)
	}
	if err := saveFileSharesLocked(); err != nil {
		fileShareState.Unlock()
		t.Fatal(err)
	}
	fileShareState.Unlock()
	if _, err := os.Stat(filepath.Join(dataDir, "files.json")); !os.IsNotExist(err) {
		t.Fatalf("旧 files.json 未归档: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "file-shares.json")); !os.IsNotExist(err) {
		t.Fatalf("旧 file-shares.json 未归档: %v", err)
	}
	var count int
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM file_aux_state`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("file_aux_state 行数=%d", count)
	}
	if err := store.DB().QueryRow(`SELECT COUNT(*) FROM file_shares_state`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("file_shares_state 行数=%d", count)
	}
}
