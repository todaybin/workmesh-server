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

type recordingMySQLExecutor struct {
	statements []string
	failed     bool
	output     string
}

func (r *recordingMySQLExecutor) Exec(_ context.Context, _ MySQLTarget, statement string) error {
	r.statements = append(r.statements, statement)
	if r.failed {
		return os.ErrPermission
	}
	return nil
}

func (r *recordingMySQLExecutor) ExecOutput(_ context.Context, _ MySQLTarget, statement string) (string, error) {
	r.statements = append(r.statements, statement)
	if r.failed {
		return "", os.ErrPermission
	}
	return r.output, nil
}

func TestCreateMySQLDatabaseRollsBackAfterUserFailure(t *testing.T) {
	exec := &recordingMySQLExecutor{}
	if err := CreateMySQLDatabase(context.Background(), exec, MySQLTarget{}, "app", "utf8mb4", "", "alice", "secret", "%"); err != nil {
		t.Fatal(err)
	}
	if len(exec.statements) != 3 || !strings.HasPrefix(exec.statements[0], "CREATE DATABASE") {
		t.Fatalf("unexpected statements: %#v", exec.statements)
	}
	if strings.Contains(strings.Join(exec.statements, " "), "secret") == false {
		t.Fatal("executor should receive password in SQL only through test fake")
	}
}

func TestMySQLExecutorRejectsInvalidIdentifiers(t *testing.T) {
	exec := &recordingMySQLExecutor{}
	if err := CreateMySQLDatabase(context.Background(), exec, MySQLTarget{}, "bad;drop", "utf8mb4", "", "", "", "%"); err == nil {
		t.Fatal("expected invalid database identifier")
	}
	if len(exec.statements) != 0 {
		t.Fatalf("invalid input reached executor: %#v", exec.statements)
	}
}

func TestMySQLCLICommandUsesDockerArguments(t *testing.T) {
	t.Setenv("PATH", filepath.Dir(os.Args[0]))
	args, env, err := mysqlCLICommand(MySQLTarget{Type: "mariadb", Host: "127.0.0.1", Port: 3306, Username: "root", Password: "complex pw", ContainerName: "db-container"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\x00") + "\x00" + strings.Join(env, "\x00")
	if len(env) != 0 || len(args) < 12 || args[1] != "exec" || args[2] != "-i" || args[3] != "db-container" || args[4] != "sh" || args[5] != "-c" || args[6] != mysqlContainerScript {
		t.Fatalf("unexpected docker command: args=%#v env=%#v", args, env)
	}
	if strings.Contains(joined, "complex pw") {
		t.Fatalf("password leaked into docker argv or environment: args=%#v env=%#v", args, env)
	}
	if !strings.Contains(strings.Join(args, "\x00"), "mariadb") {
		t.Fatalf("container CLI was not selected: args=%#v", args)
	}
}

func TestMySQLCLICommandKeepsLocalPasswordCompatibility(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "mysql")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_MYSQL_BIN", bin)
	args, env, err := mysqlCLICommand(MySQLTarget{Host: "127.0.0.1", Port: 3306, Username: "root", Password: "local-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 1 || env[0] != "MYSQL_PWD=local-secret" {
		t.Fatalf("local CLI password compatibility broken: args=%#v env=%#v", args, env)
	}
	if strings.Contains(strings.Join(args, "\x00"), "local-secret") {
		t.Fatalf("local password should remain out of argv: args=%#v", args)
	}
}

func TestMySQLContainerPasswordUsesStdinAndErrorsAreRedacted(t *testing.T) {
	binDir := t.TempDir()
	writeMySQLCaptureDocker(t, binDir)
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
	_, err := cliMySQLExecutor{}.ExecOutput(context.Background(), MySQLTarget{
		Type:          "mariadb",
		Username:      "root",
		Password:      password,
		ContainerName: "mariadb-test",
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

func TestMySQLContainerScriptConsumesOnlyCredentialPrefix(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}
	password := "quote ' dollar $ slash \\ newline\nend"
	statement := "SELECT 42;\n"
	input, err := mysqlContainerInput(password, statement)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", mysqlContainerScript, "--", "cat")
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixed container script failed: %v output=%q", err, out)
	}
	if string(out) != statement+"\n" {
		t.Fatalf("script did not leave SQL stdin untouched: %q", out)
	}
}

func writeMySQLCaptureDocker(t *testing.T, dir string) {
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
}

func TestListMySQLDatabasesParsesRows(t *testing.T) {
	exec := &recordingMySQLExecutor{output: "app\nmetrics\napp\n"}
	names, err := ListMySQLDatabases(context.Background(), exec, MySQLTarget{})
	if err != nil || len(names) != 2 || names[0] != "app" || names[1] != "metrics" {
		t.Fatalf("MySQL 数据库列表解析错误: names=%v err=%v", names, err)
	}
}

func TestMySQLVariableAndRootAccessStatementsAreValidated(t *testing.T) {
	exec := &recordingMySQLExecutor{}
	if err := SetMySQLVariable(context.Background(), exec, MySQLTarget{}, "max_connections", "512"); err != nil {
		t.Fatal(err)
	}
	if err := ChangeMySQLRootAccess(context.Background(), exec, MySQLTarget{Password: "secret"}, "%"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(exec.statements, "\n")
	if !strings.Contains(joined, "SET GLOBAL") || !strings.Contains(joined, "CREATE USER IF NOT EXISTS") || strings.Contains(joined, "bad") {
		t.Fatalf("unexpected MySQL statements: %s", joined)
	}
	if err := SetMySQLVariable(context.Background(), exec, MySQLTarget{}, "bad;drop", "1"); err == nil {
		t.Fatal("非法变量名应被拒绝")
	}
}
