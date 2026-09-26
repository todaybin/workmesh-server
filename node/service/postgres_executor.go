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

type PostgresTarget struct {
	Type          string
	Host          string
	Port          int
	Username      string
	Password      string
	Database      string
	ContainerName string
}

type PostgresExecutor interface {
	Exec(ctx context.Context, target PostgresTarget, statement string) (string, error)
}

type cliPostgresExecutor struct{}

var postgresIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$.-]{0,127}$`)
var postgresContainerIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func NewPostgresExecutor() PostgresExecutor { return cliPostgresExecutor{} }

func ValidatePostgresIdentifier(value string) error {
	if !postgresIdentifier.MatchString(strings.TrimSpace(value)) {
		return errors.New("PostgreSQL 标识符无效")
	}
	return nil
}

func QuotePostgresIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func QuotePostgresString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func (cliPostgresExecutor) Exec(ctx context.Context, target PostgresTarget, statement string) (string, error) {
	if strings.TrimSpace(statement) == "" {
		return "", errors.New("SQL 语句不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	args, env, err := postgresCLICommand(target)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = databaseCLIEnvironment(env)
	if target.ContainerName != "" {
		input, inputErr := postgresContainerInput(target.Password, statement)
		if inputErr != nil {
			return "", inputErr
		}
		cmd.Stdin = strings.NewReader(input)
	} else {
		cmd.Stdin = strings.NewReader(statement + "\n")
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", fmt.Errorf("PostgreSQL 执行超时: %w", ctx.Err())
	}
	message := strings.TrimSpace(string(out))
	if err != nil {
		if message == "" {
			message = err.Error()
		}
		if target.ContainerName != "" {
			return "", fmt.Errorf("PostgreSQL 容器执行失败: %s", redactDatabaseOutput(message, target.Password, statement))
		}
		return "", fmt.Errorf("PostgreSQL 执行失败: %s", redactDatabaseOutput(message, target.Password, statement))
	}
	return message, nil
}

func postgresCLICommand(target PostgresTarget) ([]string, []string, error) {
	bin := strings.TrimSpace(os.Getenv("WORKMESH_PSQL_BIN"))
	if bin == "" {
		bin = "psql"
	}
	host := strings.TrimSpace(target.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	if strings.ContainsAny(host, "\x00\r\n \t") {
		return nil, nil, errors.New("PostgreSQL 主机无效")
	}
	port := target.Port
	if port <= 0 {
		port = 5432
	}
	if port > 65535 {
		return nil, nil, errors.New("PostgreSQL 端口无效")
	}
	// 容器内客户端必须连镜像监听端口，不能使用宿主机映射端口。
	if target.ContainerName != "" {
		host = "127.0.0.1"
		port = 5432
	}
	user := strings.TrimSpace(target.Username)
	if user == "" {
		return nil, nil, errors.New("PostgreSQL 管理用户名不能为空")
	}
	database := strings.TrimSpace(target.Database)
	if database == "" {
		database = "postgres"
	}
	args := []string{bin, "-h", host, "-p", strconv.Itoa(port), "-U", user, "-d", database, "-v", "ON_ERROR_STOP=1", "-Atq"}
	env := []string{"PGPASSWORD=" + target.Password}
	if target.ContainerName != "" {
		if err := validatePostgresContainerName(target.ContainerName); err != nil {
			return nil, nil, err
		}
		docker := dockerBinary()
		if docker == "" {
			return nil, nil, errors.New("PostgreSQL Docker CLI 不可用: docker CLI 未安装")
		}
		env = nil
		return append([]string{docker, "exec", "-i", target.ContainerName, "sh", "-c", postgresContainerScript, "--"}, args...), env, nil
	}
	if _, err := exec.LookPath(bin); err != nil {
		return nil, nil, fmt.Errorf("PostgreSQL CLI 不可用: %w", err)
	}
	return args, env, nil
}

func validatePostgresContainerName(value string) error {
	if err := ValidateDockerIdentifier(value); err != nil {
		return err
	}
	if !postgresContainerIdentifier.MatchString(value) {
		return errors.New("Docker 容器名无效")
	}
	return nil
}

func CreatePostgresDatabase(ctx context.Context, executor PostgresExecutor, target PostgresTarget, name, username, password string, superUser bool) error {
	if err := ValidatePostgresIdentifier(name); err != nil {
		return err
	}
	if err := ValidatePostgresIdentifier(username); err != nil {
		return err
	}
	if err := CreatePostgresDatabaseOnly(ctx, executor, target, name); err != nil {
		return err
	}
	if _, err := executor.Exec(ctx, target, "CREATE ROLE "+QuotePostgresIdentifier(username)+" LOGIN PASSWORD "+QuotePostgresString(password)+";"); err != nil {
		_, _ = executor.Exec(ctx, target, "DROP DATABASE IF EXISTS "+QuotePostgresIdentifier(name)+";")
		return err
	}
	if err := GrantPostgresDatabase(ctx, executor, target, name, username); err != nil {
		_, _ = executor.Exec(ctx, target, "DROP ROLE IF EXISTS "+QuotePostgresIdentifier(username)+";")
		_, _ = executor.Exec(ctx, target, "DROP DATABASE IF EXISTS "+QuotePostgresIdentifier(name)+";")
		return err
	}
	if err := ChangePostgresPrivileges(ctx, executor, target, username, superUser); err != nil {
		_, _ = executor.Exec(ctx, target, "DROP DATABASE IF EXISTS "+QuotePostgresIdentifier(name)+";")
		_, _ = executor.Exec(ctx, target, "DROP ROLE IF EXISTS "+QuotePostgresIdentifier(username)+";")
		return err
	}
	return nil
}

func CreatePostgresDatabaseOnly(ctx context.Context, executor PostgresExecutor, target PostgresTarget, name string) error {
	if err := ValidatePostgresIdentifier(name); err != nil {
		return err
	}
	_, err := executor.Exec(ctx, target, "CREATE DATABASE "+QuotePostgresIdentifier(name)+";")
	return err
}

func GrantPostgresDatabase(ctx context.Context, executor PostgresExecutor, target PostgresTarget, name, username string) error {
	if err := ValidatePostgresIdentifier(name); err != nil {
		return err
	}
	if err := ValidatePostgresIdentifier(username); err != nil {
		return err
	}
	_, err := executor.Exec(ctx, target, "GRANT ALL PRIVILEGES ON DATABASE "+QuotePostgresIdentifier(name)+" TO "+QuotePostgresIdentifier(username)+";")
	return err
}

func RevokePostgresDatabase(ctx context.Context, executor PostgresExecutor, target PostgresTarget, name, username string) error {
	if err := ValidatePostgresIdentifier(name); err != nil {
		return err
	}
	if err := ValidatePostgresIdentifier(username); err != nil {
		return err
	}
	_, err := executor.Exec(ctx, target, "REVOKE ALL PRIVILEGES ON DATABASE "+QuotePostgresIdentifier(name)+" FROM "+QuotePostgresIdentifier(username)+";")
	return err
}

func CreatePostgresRole(ctx context.Context, executor PostgresExecutor, target PostgresTarget, username, password string, superUser bool) error {
	if err := ValidatePostgresIdentifier(username); err != nil {
		return err
	}
	if _, err := executor.Exec(ctx, target, "CREATE ROLE "+QuotePostgresIdentifier(username)+" LOGIN PASSWORD "+QuotePostgresString(password)+";"); err != nil {
		return err
	}
	return ChangePostgresPrivileges(ctx, executor, target, username, superUser)
}

func DropPostgresDatabase(ctx context.Context, executor PostgresExecutor, target PostgresTarget, name, username string) error {
	if err := ValidatePostgresIdentifier(name); err != nil {
		return err
	}
	if _, err := executor.Exec(ctx, target, "DROP DATABASE IF EXISTS "+QuotePostgresIdentifier(name)+";"); err != nil {
		return err
	}
	return nil
}

func DropPostgresRole(ctx context.Context, executor PostgresExecutor, target PostgresTarget, username string) error {
	if err := ValidatePostgresIdentifier(username); err != nil {
		return err
	}
	_, err := executor.Exec(ctx, target, "DROP ROLE IF EXISTS "+QuotePostgresIdentifier(username)+";")
	return err
}

func ChangePostgresPassword(ctx context.Context, executor PostgresExecutor, target PostgresTarget, username, password string) error {
	if err := ValidatePostgresIdentifier(username); err != nil {
		return err
	}
	_, err := executor.Exec(ctx, target, "ALTER ROLE "+QuotePostgresIdentifier(username)+" PASSWORD "+QuotePostgresString(password)+";")
	return err
}

func ChangePostgresPrivileges(ctx context.Context, executor PostgresExecutor, target PostgresTarget, username string, superUser bool) error {
	if err := ValidatePostgresIdentifier(username); err != nil {
		return err
	}
	verb := "NOSUPERUSER"
	if superUser {
		verb = "SUPERUSER"
	}
	_, err := executor.Exec(ctx, target, "ALTER ROLE "+QuotePostgresIdentifier(username)+" "+verb+";")
	return err
}

// ListPostgresDatabases 查询可连接的非模板数据库，供远程同步接口使用。
func ListPostgresDatabases(ctx context.Context, executor PostgresExecutor, target PostgresTarget) ([]string, error) {
	output, err := executor.Exec(ctx, target, `SELECT datname FROM pg_database WHERE datistemplate=false AND datallowconn=true AND datname NOT IN ('postgres');`)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || ValidatePostgresIdentifier(name) != nil {
			// psql 提示或表头不能让整次同步失败。
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, name)
	}
	return result, nil
}

const postgresContainerScript = `set -eu
IFS= read -r _wm_pw_len
case "$_wm_pw_len" in
  ''|*[!0-9]*) exit 64 ;;
esac
_wm_pw=$(dd bs=1 count="$_wm_pw_len" 2>/dev/null; printf '\001')
_wm_pw=${_wm_pw%?}
IFS= read -r _wm_separator
export PGPASSWORD="$_wm_pw"
exec "$@"
`

// postgresContainerInput keeps the credential separate from SQL while
// allowing passwords containing newlines and shell metacharacters.
func postgresContainerInput(password, statement string) (string, error) {
	if strings.IndexByte(password, 0) >= 0 {
		return "", errors.New("PostgreSQL 密码包含不支持的空字节")
	}
	return strconv.Itoa(len(password)) + "\n" + password + "\n" + statement + "\n", nil
}

func redactDatabaseOutput(value string, sensitive ...string) string {
	for _, secret := range sensitive {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	for _, marker := range []string{"PGPASSWORD=", "REDISCLI_AUTH=", "password"} {
		if i := strings.Index(strings.ToLower(value), strings.ToLower(marker)); i >= 0 {
			value = value[:i] + "[redacted]"
		}
	}
	if len(value) > 2048 {
		value = value[:2048]
	}
	return value
}
