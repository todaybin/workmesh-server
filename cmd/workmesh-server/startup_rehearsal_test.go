// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/config"
	_ "modernc.org/sqlite"
)

const (
	migrationRehearsalHelperEnv = "WORKMESH_MIGRATION_REHEARSAL_HELPER"
	migrationRehearsalDataEnv   = "WORKMESH_MIGRATION_REHEARSAL_DATA_DIR"
)

// TestIsolatedMigrationRehearsal 在独立子进程中运行两次实际启动持久化阶段。
// 这样包级 SQLite 服务不会污染其他测试，也不会监听端口或接触生产数据。
func TestIsolatedMigrationRehearsal(t *testing.T) {
	if os.Getenv(migrationRehearsalHelperEnv) == "1" {
		runMigrationRehearsalHelper(t)
		return
	}

	dataDir := filepath.Join(t.TempDir(), "rehearsal-data")
	artifactHash := installRehearsalArtifact(t, dataDir)
	for start := 1; start <= 2; start++ {
		runMigrationRehearsalProcess(t, dataDir, artifactHash, start)
	}
	assertMigrationRehearsal(t, filepath.Join(dataDir, "workmesh.db"), artifactHash)
}

// installRehearsalArtifact 使用与 update CLI 相同的验签和原子安装路径准备临时制品。
func installRehearsalArtifact(t *testing.T, dataDir string) string {
	t.Helper()
	artifact := filepath.Join(t.TempDir(), "workmesh-server-candidate")
	content := []byte("isolated release rehearsal candidate")
	if err := os.WriteFile(artifact, content, 0o700); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	signature := ed25519.Sign(private, []byte(hex.EncodeToString(digest[:])))
	signaturePath := filepath.Join(t.TempDir(), "workmesh-server-candidate.sig")
	if err := os.WriteFile(signaturePath, []byte(base64.RawStdEncoding.EncodeToString(signature)), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"update", artifact, "--signature", signaturePath, "--public-key", base64.RawStdEncoding.EncodeToString(public), "--target", filepath.Join(dataDir, "releases", "current.artifact"), "--sha256", hex.EncodeToString(digest[:]), "--version", "v2.rehearsal"}
	if handled, err := runCLI(args, dataDir); !handled || err != nil {
		t.Fatalf("临时制品安装失败: handled=%v err=%v", handled, err)
	}
	return hex.EncodeToString(digest[:])
}

// runMigrationRehearsalHelper 在单独的测试进程中执行服务的真实启动存储顺序。
func runMigrationRehearsalHelper(t *testing.T) {
	t.Helper()
	dataDir := strings.TrimSpace(os.Getenv(migrationRehearsalDataEnv))
	if dataDir == "" {
		t.Fatal("隔离迁移演练缺少数据目录")
	}
	cfg := config.Config{DataDir: dataDir, NodeID: "migration-rehearsal", Role: "secondary", ListenAddr: "127.0.0.1:0"}
	if err := cfg.ApplyRuntimeEnvironment(); err != nil {
		t.Fatal(err)
	}
	if err := initializeDataDir(cfg.DataDir); err != nil {
		t.Fatal(err)
	}
	store, err := initializeServerState(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

// runMigrationRehearsalProcess 启动受控测试子进程，避免公共服务单例跨启动复用。
func runMigrationRehearsalProcess(t *testing.T, dataDir, artifactHash string, start int) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestIsolatedMigrationRehearsal$")
	command.Env = replaceEnvironment(os.Environ(), map[string]string{
		migrationRehearsalHelperEnv:      "1",
		migrationRehearsalDataEnv:        dataDir,
		"WORKMESH_ARTIFACT_SHA256":       artifactHash,
		"WORKMESH_MIGRATION_BACKUP_PATH": filepath.Join(dataDir, "backups"),
	})
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("第 %d 次隔离启动失败: %v\n%s", start, err, output)
	}
}

// assertMigrationRehearsal 验证首次迁移只应用一次，第二次启动只写关键迁移的 noop 审计。
func assertMigrationRehearsal(t *testing.T, path, artifactHash string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var schemaCount, failedRuns int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&schemaCount); err != nil {
		t.Fatal(err)
	}
	if schemaCount != 18 {
		t.Fatalf("隔离库迁移数量 = %d, want 18", schemaCount)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM migration_runs WHERE status='failed'").Scan(&failedRuns); err != nil {
		t.Fatal(err)
	}
	if failedRuns != 0 {
		t.Fatalf("隔离迁移不应失败，实际失败数 = %d", failedRuns)
	}
	for _, version := range []string{"0001-migration-metadata", "0012-runtime-task-state", "0013-log-audit-v2", "0016-gateway-machine-identity"} {
		assertMigrationRunCount(t, db, version, "applied", 1)
		assertMigrationRunCount(t, db, version, "noop", 1)
	}
	assertMigrationRunCount(t, db, "0015-website-template-relational", "applied", 1)
	for _, version := range []string{"0008-database-backup-metadata", "0009-database-runtime-states"} {
		assertMigrationRunCount(t, db, version, "applied", 1)
	}
	var auditedHash string
	if err := db.QueryRow("SELECT artifact_sha256 FROM migration_runs ORDER BY id DESC LIMIT 1").Scan(&auditedHash); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(auditedHash, artifactHash) {
		t.Fatalf("迁移审计制品摘要 = %s, want %s", auditedHash, artifactHash)
	}
}

// assertMigrationRunCount 验证指定版本和状态的迁移审计数量。
func assertMigrationRunCount(t *testing.T, db *sql.DB, version, status string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM migration_runs WHERE to_version=? AND status=?", version, status).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("迁移 %s 状态 %s 数量 = %d, want %d", version, status, count, want)
	}
}

// replaceEnvironment 为隔离子进程替换指定环境变量，避免继承生产数据目录。
func replaceEnvironment(base []string, values map[string]string) []string {
	result := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		if _, replaced := values[name]; !replaced {
			result = append(result, entry)
		}
	}
	for name, value := range values {
		result = append(result, fmt.Sprintf("%s=%s", name, value))
	}
	return result
}
