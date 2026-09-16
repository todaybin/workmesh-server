// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabaseBackupContainerCommandKeepsPasswordOutOfArgs(t *testing.T) {
	command := DatabaseBackupCommand{
		Program:     "pg_dump",
		Args:        []string{"--dbname", "workmesh"},
		Container:   "workmesh-panel-postgres",
		Environment: map[string]string{"PGPASSWORD": "secret with spaces"},
	}
	program, args, err := databaseBackupContainerCommand(command)
	if err != nil {
		t.Fatalf("databaseBackupContainerCommand() error = %v", err)
	}
	if program != DockerBinary() {
		t.Fatalf("program = %q, want %q", program, DockerBinary())
	}
	joined := strings.Join(args, "\x00")
	if strings.Contains(joined, "secret with spaces") {
		t.Fatalf("password leaked into docker arguments: %q", joined)
	}
	if !strings.Contains(joined, `export PGPASSWORD=`) {
		t.Fatalf("container script does not set PGPASSWORD: %q", joined)
	}
}

func TestDatabaseBackupOutputUsesPrivateAtomicTempFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "nested", "backup.dump")
	file, temp, err := createDatabaseBackupOutput(target)
	if err != nil {
		t.Fatalf("createDatabaseBackupOutput() error = %v", err)
	}
	if _, err := file.WriteString("backup"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := os.Rename(temp, target); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode = %o, want 600", info.Mode().Perm())
	}
}

func TestDatabaseBackupInputRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.dump")
	if err := os.WriteFile(target, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.dump")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := openDatabaseBackupInput(link); err == nil {
		t.Fatal("expected symlink input to be rejected")
	}
}

func TestDatabaseBackupInputRejectsSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "target.dump")
	if err := os.WriteFile(target, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(root, "linked")
	if err := os.Symlink(outside, linkDir); err != nil {
		t.Fatal(err)
	}
	if _, err := openDatabaseBackupInput(filepath.Join(linkDir, "target.dump")); err == nil {
		t.Fatal("expected symlinked input parent to be rejected")
	}
}

func TestDatabaseBackupOutputRejectsSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	linkDir := filepath.Join(root, "linked")
	if err := os.Symlink(outside, linkDir); err != nil {
		t.Fatal(err)
	}
	if _, _, err := createDatabaseBackupOutput(filepath.Join(linkDir, "backup.dump")); err == nil {
		t.Fatal("expected symlinked output parent to be rejected")
	}
}

func TestRedisBackupCopyUsesTemporaryFileAndAtomicCommit(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "docker")
	script := `#!/bin/sh
if [ "$1" != "cp" ]; then
  exit 2
fi
case "$2" in
  *:*)
    printf 'new-rdb' > "$3"
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(fakeBin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", root+string(os.PathListSeparator)+oldPath)

	artifact := filepath.Join(root, "backup.rdb")
	if err := os.WriteFile(artifact, []byte("old-rdb"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := DockerDatabaseBackupExecutor{}
	result, err := executor.Execute(context.Background(), DatabaseBackupCommand{
		Name:       "redis-copy-backup",
		Program:    "docker",
		Args:       []string{"cp", "workmesh-panel-redis:/data/dump.rdb", artifact},
		StreamMode: DatabaseBackupStreamStdoutToArtifact,
		StdoutPath: artifact,
	})
	if err != nil {
		t.Fatalf("Redis copy error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Redis copy exit code = %d", result.ExitCode)
	}
	content, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new-rdb" {
		t.Fatalf("artifact content = %q", content)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".workmesh-redis-copy-") {
			t.Fatalf("temporary Redis copy directory was not removed: %s", entry.Name())
		}
	}
}

func TestRedisBackupCopyRejectsSymlinkArtifact(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.rdb")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(root, "backup.rdb")
	if err := os.Symlink(outside, artifact); err != nil {
		t.Fatal(err)
	}
	_, err := runRedisBackupCopyCommand(context.Background(), DatabaseBackupCommand{
		Name:    "redis-copy-backup",
		Program: "docker",
		Args:    []string{"cp", "workmesh-panel-redis:/data/dump.rdb", artifact},
	})
	if err == nil {
		t.Fatal("expected symlink artifact to be rejected")
	}
}

func TestRedisRestoreCopyAcceptsArtifactToContainerPlan(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "docker")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	artifact := filepath.Join(root, "restore.rdb")
	if err := os.WriteFile(artifact, []byte("restore-rdb"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := (DockerDatabaseBackupExecutor{}).Execute(context.Background(), DatabaseBackupCommand{
		Name:       "redis-restore-copy",
		Program:    "docker",
		Args:       []string{"cp", artifact, "workmesh-panel-redis:/data/dump.rdb"},
		StdinPath:  artifact,
		StreamMode: DatabaseBackupStreamArtifactToStdin,
	})
	if err != nil {
		t.Fatalf("Redis restore copy error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Redis restore copy exit code = %d", result.ExitCode)
	}
}

type databaseBackupFakeCommandRunner struct {
	commands []DatabaseBackupCommand
}

func (f *databaseBackupFakeCommandRunner) Run(_ context.Context, command DatabaseBackupCommand) (DatabaseBackupCommandResult, error) {
	f.commands = append(f.commands, command)
	return DatabaseBackupCommandResult{ExitCode: 0}, nil
}

func TestDockerDatabaseBackupExecutorSupportsFakeRunner(t *testing.T) {
	runner := &databaseBackupFakeCommandRunner{}
	executor := DockerDatabaseBackupExecutor{Runner: runner}
	result, err := executor.Execute(context.Background(), DatabaseBackupCommand{
		Name:        "redis-wait-bgsave",
		Program:     "redis-cli",
		Container:   "workmesh-panel-redis",
		Environment: map[string]string{"REDISCLI_AUTH": "fake-secret"},
		Args:        []string{"--raw", "INFO", "persistence"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || len(runner.commands) != 1 {
		t.Fatalf("fake runner result=%+v calls=%d", result, len(runner.commands))
	}
	if strings.Contains(strings.Join(runner.commands[0].Args, "\x00"), "fake-secret") {
		t.Fatal("fake runner observed password in command arguments")
	}
}
