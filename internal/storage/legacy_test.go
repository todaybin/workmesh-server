// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestImportLegacyJSONIsIdempotentAndPreservesSource(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "groups.json")
	content := []byte(`[{"id":1,"name":"Default","type":"website"}]`)
	if err := os.WriteFile(sourcePath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(t.TempDir(), "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.DB().Exec("CREATE TABLE imported_groups(id INTEGER PRIMARY KEY, name TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	handler := LegacyJSONHandlerFunc{DomainName: LegacyDomainGroups, ImportFunc: func(ctx context.Context, tx *sql.Tx, source LegacyJSONSource) (int64, error) {
		var groups []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(source.Data, &groups); err != nil {
			return 0, err
		}
		for _, group := range groups {
			if _, err := tx.ExecContext(ctx, "INSERT INTO imported_groups(id, name) VALUES(?, ?)", group.ID, group.Name); err != nil {
				return 0, err
			}
		}
		return int64(len(groups)), nil
	}}
	options := LegacyJSONImportOptions{SourceDir: root, Sources: []LegacyJSONSourceSpec{{Domain: LegacyDomainGroups, FileName: "groups.json"}}, Handlers: []LegacyJSONHandler{handler}}
	report, err := store.ImportLegacyJSON(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 || report.Items[0].Status != LegacyImportImported || report.Items[0].ImportedCount != 1 {
		t.Fatalf("首次导入报告异常: %+v", report)
	}
	report, err = store.ImportLegacyJSON(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Items[0].Status != LegacyImportSkipped {
		t.Fatalf("重复导入状态 = %s, want skipped", report.Items[0].Status)
	}
	options.ArchiveImported = true
	if _, err := store.ImportLegacyJSON(context.Background(), options); err != nil { t.Fatal(err) }
	if got, err := os.ReadFile(sourcePath); err == nil || !os.IsNotExist(err) {
		t.Fatalf("源文件未归档: content=%q err=%v", got, err)
	}
	entries, err := filepath.Glob(filepath.Join(root, "backups", "legacy-*", "groups.json"))
	if err != nil || len(entries) != 1 { t.Fatalf("旧 JSON 未归档: %v %v", entries, err) }
}
