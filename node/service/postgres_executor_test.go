// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type postgresMockExecutor struct {
	statements []string
	failAt     int
	output     string
}

func (m *postgresMockExecutor) Exec(_ context.Context, _ PostgresTarget, statement string) (string, error) {
	m.statements = append(m.statements, statement)
	if m.failAt > 0 && len(m.statements) == m.failAt {
		return "", context.Canceled
	}
	return m.output, nil
}

func TestCreatePostgresDatabaseRollback(t *testing.T) {
	m := &postgresMockExecutor{failAt: 2}
	err := CreatePostgresDatabase(context.Background(), m, PostgresTarget{Username: "postgres"}, "appdb", "appuser", "secret", false)
	if err == nil || len(m.statements) != 3 {
		t.Fatalf("err=%v statements=%v", err, m.statements)
	}
	if !strings.HasPrefix(m.statements[2], "DROP DATABASE") {
		t.Fatalf("rollback order=%v", m.statements)
	}
}

func TestPostgresIdentifierValidation(t *testing.T) {
	if err := ValidatePostgresIdentifier("bad name"); err == nil {
		t.Fatal("expected invalid identifier")
	}
}

func TestPostgresCLIUnavailable(t *testing.T) {
	t.Setenv("WORKMESH_PSQL_BIN", "workmesh-missing-psql")
	_, err := NewPostgresExecutor().Exec(context.Background(), PostgresTarget{Host: "127.0.0.1", Port: 5432, Username: "postgres"}, "SELECT 1;")
	if err == nil || !strings.Contains(err.Error(), "CLI 不可用") {
		t.Fatalf("err=%v", err)
	}
}

func TestPostgresCLICommandUsesDockerWithoutHostPSQL(t *testing.T) {
	dockerPath := writePostgresTestExecutable(t, "exit 0\n")
	t.Setenv("PATH", filepath.Dir(dockerPath))
	t.Setenv("WORKMESH_PSQL_BIN", "workmesh-missing-psql")

	args, env, err := postgresCLICommand(PostgresTarget{
		Host:          "127.0.0.1",
		Port:          5432,
		Username:      "postgres",
		Password:      "secret",
		ContainerName: "postgres-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) < 8 || args[0] != dockerPath || args[1] != "exec" || args[2] != "-i" || args[3] != "postgres-test" || args[4] != "sh" || args[5] != "-c" {
		t.Fatalf("unexpected docker command: %#v", args)
	}
	if args[6] != postgresContainerScript || args[7] != "--" || args[8] != "workmesh-missing-psql" {
		t.Fatalf("container password script missing: %#v", args)
	}
	if len(env) != 0 {
		t.Fatalf("container password must not be placed in host environment: %#v", env)
	}
	if strings.Contains(strings.Join(args, "\x00"), "secret") {
		t.Fatalf("password leaked into argv: %#v", args)
	}
}

func TestPostgresContainerPasswordUsesStdinAndErrorsAreRedacted(t *testing.T) {
	binDir := t.TempDir()
	writePostgresCaptureDocker(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	argsFile := filepath.Join(t.TempDir(), "args")
	envFile := filepath.Join(t.TempDir(), "env")
	inputFile := filepath.Join(t.TempDir(), "input")
	t.Setenv("WORKMESH_CAPTURE_ARGS", argsFile)
	t.Setenv("WORKMESH_CAPTURE_ENV", envFile)
	t.Setenv("WORKMESH_CAPTURE_INPUT", inputFile)
	t.Setenv("MYSQL_PWD", "ambient-host-secret")
	t.Setenv("PGPASSWORD", "ambient-host-secret")
	t.Setenv("REDISCLI_AUTH", "ambient-host-secret")

	password := "p w'\";$()&\nline"
	statement := "SELECT 1;"
	_, err := NewPostgresExecutor().Exec(context.Background(), PostgresTarget{
		Username:      "postgres",
		Password:      password,
		ContainerName: "postgres-test",
	}, statement)
	if err == nil {
		t.Fatal("expected fake Docker failure")
	}
	args, readErr := os.ReadFile(argsFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	env, readErr := os.ReadFile(envFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	input, readErr := os.ReadFile(inputFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(args), password) || strings.Contains(string(env), password) || strings.Contains(string(env), "ambient-host-secret") {
		t.Fatalf("password leaked into argv or host environment: args=%q env=%q", args, env)
	}
	if !strings.Contains(string(input), password) || !strings.Contains(string(input), statement) {
		t.Fatalf("stdin protocol did not carry password and SQL: %q", input)
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("password leaked in error: %v", err)
	}
}

func TestPostgresContainerScriptConsumesOnlyCredentialPrefix(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}
	password := "quote ' dollar $ slash \\ newline\nend"
	statement := "SELECT 42;\n"
	input, err := postgresContainerInput(password, statement)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", postgresContainerScript, "--", "cat")
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixed container script failed: %v output=%q", err, out)
	}
	if string(out) != statement+"\n" {
		t.Fatalf("script did not leave SQL stdin untouched: %q", out)
	}
}

func TestPostgresContainerNameValidation(t *testing.T) {
	for _, name := range []string{"bad name", "--privileged", "/var/run/docker.sock", "db\nname"} {
		if _, _, err := postgresCLICommand(PostgresTarget{Username: "postgres", ContainerName: name}); err == nil || !strings.Contains(err.Error(), "Docker 容器名无效") {
			t.Fatalf("container name %q should be rejected: %v", name, err)
		}
	}
}

func TestPostgresDockerCLIUnavailable(t *testing.T) {
	originalLookPath := dockerLookPath
	originalStandardPaths := dockerStandardPaths
	t.Cleanup(func() {
		dockerLookPath = originalLookPath
		dockerStandardPaths = originalStandardPaths
	})
	dockerLookPath = func(string) (string, error) {
		return "", os.ErrNotExist
	}
	dockerStandardPaths = func() []string {
		return nil
	}

	_, _, err := postgresCLICommand(PostgresTarget{Username: "postgres", ContainerName: "postgres-test"})
	if err == nil || !strings.Contains(err.Error(), "Docker CLI 不可用") {
		t.Fatalf("err=%v", err)
	}
}

func TestPostgresContainerFailureIsExplicit(t *testing.T) {
	dockerPath := writePostgresTestExecutable(t, "printf '%s' 'No such container: postgres-test' >&2\nexit 1\n")
	t.Setenv("PATH", filepath.Dir(dockerPath))

	_, err := NewPostgresExecutor().Exec(context.Background(), PostgresTarget{
		Username:      "postgres",
		Password:      "secret",
		ContainerName: "postgres-test",
	}, "SELECT 1;")
	if err == nil || !strings.Contains(err.Error(), "PostgreSQL 容器执行失败") || !strings.Contains(err.Error(), "No such container") {
		t.Fatalf("err=%v", err)
	}
}

func writePostgresTestExecutable(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "docker")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writePostgresCaptureDocker(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "docker")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$WORKMESH_CAPTURE_ARGS"
env > "$WORKMESH_CAPTURE_ENV"
cat > "$WORKMESH_CAPTURE_INPUT"
cat "$WORKMESH_CAPTURE_INPUT" >&2
exit 17
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestListPostgresDatabasesParsesRows(t *testing.T) {
	mock := &postgresMockExecutor{output: "app\nmetrics\napp\n"}
	names, err := ListPostgresDatabases(context.Background(), mock, PostgresTarget{})
	if err != nil || len(names) != 2 || names[0] != "app" || names[1] != "metrics" {
		t.Fatalf("PostgreSQL 数据库列表解析错误: names=%v err=%v", names, err)
	}
}
