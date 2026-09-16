// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/node/service"
)

func TestResolveDatabaseBackupArtifactStaysUnderControlledRoot(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	path, err := resolveDatabaseBackupArtifact(map[string]any{"fileName": "postgres.dump"}, service.DatabaseBackupOperationBackup)
	if err != nil {
		t.Fatalf("resolveDatabaseBackupArtifact() error = %v", err)
	}
	root := filepath.Join(os.Getenv("WORKMESH_DATA_DIR"), "backups", "databases")
	if !databaseBackupPathWithin(root, path) {
		t.Fatalf("artifact path escaped root: %q", path)
	}
	if !strings.HasSuffix(path, ".dump") {
		t.Fatalf("unexpected artifact path: %q", path)
	}
	for _, name := range []string{"../escape", "/tmp/escape", "sub/escape", "bad\x00name"} {
		if _, err := resolveDatabaseBackupArtifact(map[string]any{"fileName": name}, service.DatabaseBackupOperationBackup); err == nil {
			t.Fatalf("expected unsafe artifact %q to fail", name)
		}
	}
}

func TestResolveDatabaseBackupArtifactRejectsRealSymlinkEscapes(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	root := filepath.Join(dataDir, "backups", "databases")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.dump")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.dump")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveDatabaseBackupArtifact(map[string]any{"fileName": "escape.dump"}, service.DatabaseBackupOperationBackup); err == nil {
		t.Fatal("expected existing symlink artifact to be rejected")
	}

	linkedDataDir := filepath.Join(t.TempDir(), "data-link")
	if err := os.Symlink(t.TempDir(), linkedDataDir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_DATA_DIR", linkedDataDir)
	if _, err := resolveDatabaseBackupArtifact(map[string]any{"fileName": "new.dump"}, service.DatabaseBackupOperationBackup); err == nil {
		t.Fatal("expected symlinked backup root to be rejected")
	}
}

func TestDatabaseBackupOperationLockReleasesAfterUse(t *testing.T) {
	container := "workmesh-test-lock"
	release, ok := acquireDatabaseBackupOperationLock(container)
	if !ok {
		t.Fatal("first lock acquisition failed")
	}
	if _, secondOK := acquireDatabaseBackupOperationLock(container); secondOK {
		t.Fatal("same container lock was acquired twice")
	}
	release()
	thirdRelease, ok := acquireDatabaseBackupOperationLock(container)
	if !ok {
		t.Fatal("lock was not released")
	}
	thirdRelease()
}

type databaseBackupRouteFakeExecutor struct {
	commands []service.DatabaseBackupCommand
}

func (f *databaseBackupRouteFakeExecutor) Execute(_ context.Context, command service.DatabaseBackupCommand) (service.DatabaseBackupCommandResult, error) {
	f.commands = append(f.commands, command)
	return service.DatabaseBackupCommandResult{
		ExitCode: 0,
		Stdout:   "password=route-secret docker cp secret",
		Stderr:   "PGPASSWORD=route-secret",
	}, nil
}

func TestSanitizeDatabaseBackupExecutionRemovesSecretsAndCommands(t *testing.T) {
	fake := &databaseBackupRouteFakeExecutor{}
	backupService := service.NewDatabaseBackupService(fake, time.Second)
	execution, err := backupService.Backup(context.Background(), service.DatabaseBackupRequest{
		Type:          service.DatabaseBackupPostgres,
		ContainerName: "workmesh-panel-postgres",
		DatabaseName:  "workmesh",
		Username:      "postgres",
		Password:      "route-secret",
		ArtifactPath:  filepath.Join(t.TempDir(), "backup.dump"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.commands) != 1 {
		t.Fatalf("fake executor calls=%d", len(fake.commands))
	}
	sanitized := sanitizeDatabaseBackupExecution(execution, "route-secret")
	if strings.Contains(sanitized.Results[0].Stdout, "route-secret") ||
		strings.Contains(sanitized.Results[0].Stderr, "route-secret") ||
		sanitized.Results[0].Stdout != "" || sanitized.Results[0].Stderr != "" {
		t.Fatalf("execution output was not removed: %+v", sanitized.Results[0])
	}
	command := sanitized.Plan.Commands[0]
	if command.Program != "" || len(command.Args) != 0 || command.Environment != nil ||
		command.StdinPath != "" || command.StdoutPath != "" {
		t.Fatalf("sanitized command still exposes execution details: %+v", command)
	}
	if sanitized.Plan.ArtifactPath != "" {
		t.Fatalf("sanitized plan exposes artifact path: %q", sanitized.Plan.ArtifactPath)
	}
}

func TestDatabaseBackupTypeRejectsUnknownByPlan(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	plan, err := databaseBackupService.BuildBackupPlan(service.DatabaseBackupRequest{
		Type:          service.DatabaseBackupType("oracle"),
		ContainerName: "workmesh-panel-db",
		DatabaseName:  "workmesh",
		Username:      "root",
		ArtifactPath:  filepath.Join(os.Getenv("WORKMESH_DATA_DIR"), "backups", "databases", "db.dump"),
	})
	if err == nil || plan.Commands != nil {
		t.Fatalf("unknown database type should be rejected: plan=%+v err=%v", plan, err)
	}
}
