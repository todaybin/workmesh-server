// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type RedisTarget struct {
	Host          string
	Port          int
	Password      string
	ContainerName string
}

type RedisExecutor interface {
	Exec(ctx context.Context, target RedisTarget, args ...string) (string, error)
}

type cliRedisExecutor struct{}

func NewRedisExecutor() RedisExecutor { return cliRedisExecutor{} }

func (cliRedisExecutor) Exec(ctx context.Context, target RedisTarget, args ...string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("Redis 命令不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}
	command, env, err := redisCLICommand(target, args...)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = databaseCLIEnvironment(env)
	if target.ContainerName != "" {
		input, inputErr := redisContainerInput(target.Password)
		if inputErr != nil {
			return "", inputErr
		}
		cmd.Stdin = strings.NewReader(input)
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", fmt.Errorf("Redis 执行超时: %w", ctx.Err())
	}
	message := strings.TrimSpace(string(out))
	if target.ContainerName != "" && target.Password != "" {
		message = stripRedisAuthReply(message)
	}
	upperMessage := strings.ToUpper(message)
	if err == nil && (strings.HasPrefix(upperMessage, "ERR ") || strings.HasPrefix(upperMessage, "NOAUTH ") || strings.HasPrefix(upperMessage, "WRONGPASS ")) {
		err = errors.New("redis command rejected")
	}
	if err != nil {
		if message == "" {
			message = err.Error()
		}
		message = redactRedisOutput(message, append([]string{target.Password}, args...)...)
		if target.ContainerName != "" {
			return "", fmt.Errorf("Redis 容器执行失败: %s", message)
		}
		return "", fmt.Errorf("Redis 执行失败: %s", message)
	}
	return message, nil
}

func redisCLICommand(target RedisTarget, args ...string) ([]string, []string, error) {
	bin := strings.TrimSpace(os.Getenv("WORKMESH_REDIS_CLI_BIN"))
	if bin == "" {
		bin = "redis-cli"
	}
	host := strings.TrimSpace(target.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	if strings.ContainsAny(host, "\x00\r\n \t") {
		return nil, nil, errors.New("Redis 主机无效")
	}
	port := target.Port
	if port <= 0 {
		port = 6379
	}
	if port > 65535 {
		return nil, nil, errors.New("Redis 端口无效")
	}
	// 容器内客户端必须连镜像监听端口，不能使用宿主机映射端口。
	if target.ContainerName != "" {
		host = "127.0.0.1"
		port = 6379
	}
	command := []string{bin, "--raw", "-h", host, "-p", strconv.Itoa(port)}
	if target.ContainerName != "" {
		if err := validateRedisContainerName(target.ContainerName); err != nil {
			return nil, nil, err
		}
		docker := dockerBinary()
		if docker == "" {
			return nil, nil, errors.New("Docker CLI 不可用: docker CLI 未安装")
		}
		command = append([]string{docker, "exec", "-i", target.ContainerName, "sh", "-c", redisContainerScript, "--"}, command...)
		command = append(command, args...)
		return command, nil, nil
	}
	if _, err := exec.LookPath(bin); err != nil {
		return nil, nil, fmt.Errorf("Redis CLI 不可用: %w", err)
	}
	command = append(command, args...)
	return command, []string{"REDISCLI_AUTH=" + target.Password}, nil
}

var redisContainerIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func validateRedisContainerName(value string) error {
	if !redisContainerIdentifier.MatchString(strings.TrimSpace(value)) {
		return errors.New("Docker 容器名无效")
	}
	return nil
}

const redisContainerScript = `set -eu
IFS= read -r _wm_pw_len
case "$_wm_pw_len" in
  ''|*[!0-9]*) exit 64 ;;
esac
_wm_pw=$(dd bs=1 count="$_wm_pw_len" 2>/dev/null; printf '\001')
_wm_pw=${_wm_pw%?}
IFS= read -r _wm_separator
export REDISCLI_AUTH="$_wm_pw"
exec "$@"
`

// redisContainerInput sends only the credential prefix. Redis commands remain
// ordinary argv values, so user input is never interpreted by a shell.
func redisContainerInput(password string) (string, error) {
	if strings.IndexByte(password, 0) >= 0 {
		return "", errors.New("Redis 密码包含不支持的空字节")
	}
	return strconv.Itoa(len(password)) + "\n" + password + "\n", nil
}

func stripRedisAuthReply(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.TrimSpace(value)
	if value == "OK" {
		return ""
	}
	if strings.HasPrefix(value, "OK\n") {
		return strings.TrimSpace(strings.TrimPrefix(value, "OK\n"))
	}
	return value
}

func redactRedisOutput(value string, sensitive ...string) string {
	for _, secret := range sensitive {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	for _, marker := range []string{"REDISCLI_AUTH=", "password", "AUTH "} {
		if index := strings.Index(strings.ToLower(value), strings.ToLower(marker)); index >= 0 {
			value = value[:index] + "[redacted]"
		}
	}
	if len(value) > 2048 {
		value = value[:2048]
	}
	return value
}

func ValidateRedisConfigKey(key string) error {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "timeout", "maxclients", "maxmemory", "appendonly", "appendfsync", "save", "requirepass":
		return nil
	default:
		return errors.New("Redis 配置项不允许修改")
	}
}
