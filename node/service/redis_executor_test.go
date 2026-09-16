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

func TestValidateRedisConfigKey(t *testing.T) {
	if err := ValidateRedisConfigKey("timeout"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRedisConfigKey("dir"); err == nil {
		t.Fatal("expected forbidden key")
	}
}

func TestRedisCLIUnavailable(t *testing.T) {
	t.Setenv("WORKMESH_REDIS_CLI_BIN", "workmesh-missing-redis-cli")
	_, err := NewRedisExecutor().Exec(context.Background(), RedisTarget{Host: "127.0.0.1", Port: 6379}, "PING")
	if err == nil || !strings.Contains(err.Error(), "CLI 不可用") {
		t.Fatalf("err=%v", err)
	}
}

func TestRedisCLICommandUsesDockerWithoutPasswordInArguments(t *testing.T) {
	binDir := t.TempDir()
	writeRedisTestExecutable(t, binDir, "docker", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", binDir)

	args, env, err := redisCLICommand(RedisTarget{
		Host:          "127.0.0.1",
		Port:          6379,
		Password:      "secret-password",
		ContainerName: "redis-panel",
	}, "PING")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\x00") + "\x00" + strings.Join(env, "\x00")
	if len(args) < 10 || args[1] != "exec" || args[2] != "-i" || args[3] != "redis-panel" || args[4] != "sh" || args[5] != "-c" || args[6] != redisContainerScript || args[7] != "--" || args[8] != "redis-cli" {
		t.Fatalf("unexpected docker command: %#v", args)
	}
	if args[9] != "--raw" {
		t.Fatalf("container redis CLI arguments missing: %#v", args)
	}
	if strings.Contains(joined, "secret-password") {
		t.Fatalf("password leaked into command or environment: args=%#v env=%#v", args, env)
	}
	if input, inputErr := redisContainerInput("secret-password"); inputErr != nil || !strings.HasPrefix(input, "15\nsecret-password\n") {
		t.Fatalf("password was not routed through stdin protocol: %q err=%v", input, inputErr)
	}
}

func TestRedisContainerDoesNotRequireHostRedisCLI(t *testing.T) {
	binDir := t.TempDir()
	writeRedisTestExecutable(t, binDir, "docker", "#!/bin/sh\nprintf 'PONG\\n'\n")
	t.Setenv("PATH", binDir)
	t.Setenv("WORKMESH_REDIS_CLI_BIN", "workmesh-missing-redis-cli")

	output, err := NewRedisExecutor().Exec(context.Background(), RedisTarget{
		Host:          "127.0.0.1",
		Port:          6379,
		ContainerName: "redis-panel",
	}, "PING")
	if err != nil || output != "PONG" {
		t.Fatalf("output=%q err=%v", output, err)
	}
}

func TestRedisContainerNameValidation(t *testing.T) {
	binDir := t.TempDir()
	writeRedisTestExecutable(t, binDir, "docker", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", binDir)

	_, _, err := redisCLICommand(RedisTarget{ContainerName: "redis panel"}, "PING")
	if err == nil || !strings.Contains(err.Error(), "容器名无效") {
		t.Fatalf("err=%v", err)
	}
}

func TestRedisDockerCLIUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
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

	_, err := NewRedisExecutor().Exec(context.Background(), RedisTarget{ContainerName: "redis-panel"}, "PING")
	if err == nil || !strings.Contains(err.Error(), "Docker CLI 不可用") {
		t.Fatalf("err=%v", err)
	}
}

func TestRedisContainerFailureIsExplicitAndRedactsPassword(t *testing.T) {
	binDir := t.TempDir()
	writeRedisTestExecutable(t, binDir, "docker", "#!/bin/sh\ncat >&2\nexit 17\n")
	t.Setenv("PATH", binDir)

	_, err := NewRedisExecutor().Exec(context.Background(), RedisTarget{
		Password:      "secret-password",
		ContainerName: "redis-panel",
	}, "PING")
	if err == nil || !strings.Contains(err.Error(), "Redis 容器执行失败") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "secret-password") {
		t.Fatalf("password leaked in error: %v", err)
	}
}

func TestRedisContainerComplexPasswordUsesStdinAndNotHostEnvironment(t *testing.T) {
	binDir := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args")
	envFile := filepath.Join(t.TempDir(), "env")
	inputFile := filepath.Join(t.TempDir(), "input")
	writeRedisCaptureDocker(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WORKMESH_CAPTURE_ARGS", argsFile)
	t.Setenv("WORKMESH_CAPTURE_ENV", envFile)
	t.Setenv("WORKMESH_CAPTURE_INPUT", inputFile)
	t.Setenv("MYSQL_PWD", "ambient-host-secret")
	t.Setenv("PGPASSWORD", "ambient-host-secret")
	t.Setenv("REDISCLI_AUTH", "ambient-host-secret")

	password := "p w'\";$()&\nline"
	_, err := NewRedisExecutor().Exec(context.Background(), RedisTarget{
		Password:      password,
		ContainerName: "redis-panel",
	}, "PING")
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
	if !strings.Contains(string(input), password) {
		t.Fatalf("stdin protocol did not carry password: %q", input)
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("password leaked in error: %v", err)
	}
}

func TestRedisContainerScriptConsumesOnlyCredentialPrefix(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}
	password := "quote ' dollar $ slash \\ newline\nend"
	input, err := redisContainerInput(password)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", redisContainerScript, "--", "cat")
	cmd.Stdin = strings.NewReader(input + "PING\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixed container script failed: %v output=%q", err, out)
	}
	if string(out) != "PING\n" {
		t.Fatalf("script did not leave Redis stdin untouched: %q", out)
	}
}

func writeRedisTestExecutable(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
}

func writeRedisCaptureDocker(t *testing.T, dir string) {
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
