// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MySQLTarget 描述执行管理 SQL 所需的真实连接信息。
type MySQLTarget struct {
	Type          string
	Host          string
	Port          int
	Username      string
	Password      string
	ContainerName string
}

// MySQLExecutor 执行不依赖 shell 的 MySQL/MariaDB CLI 命令。
type MySQLExecutor interface {
	Exec(ctx context.Context, target MySQLTarget, statement string) error
}

// MySQLQueryExecutor 扩展返回查询结果的只读执行能力，供远程同步使用。
type MySQLQueryExecutor interface {
	MySQLExecutor
	ExecOutput(ctx context.Context, target MySQLTarget, statement string) (string, error)
}

type cliMySQLExecutor struct{}

var mysqlIdentifier = regexp.MustCompile(`^[A-Za-z0-9_$.-]+$`)
var mysqlVariableName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
var numericMySQLValue = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

// NewMySQLExecutor 创建默认的真实 CLI 执行器。
func NewMySQLExecutor() MySQLExecutor { return cliMySQLExecutor{} }

// ValidateMySQLIdentifier 拒绝不可安全拼入 SQL 标识符的用户输入。
func ValidateMySQLIdentifier(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 || !mysqlIdentifier.MatchString(value) {
		return fmt.Errorf("MySQL 标识符无效: %q", value)
	}
	return nil
}

// ValidateMySQLHost 校验连接主机和用户来源主机，拒绝控制字符和参数边界。
func ValidateMySQLHost(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 || strings.ContainsAny(value, " \t\r\n\x00") {
		return errors.New("MySQL 主机无效")
	}
	if net.ParseIP(strings.Trim(value, "[]")) != nil {
		return nil
	}
	if strings.Contains(value, "/") || strings.Contains(value, "\\") {
		return errors.New("MySQL 主机无效")
	}
	return nil
}

// QuoteMySQLIdentifier 使用反引号转义 SQL 标识符，调用方应先执行校验。
func QuoteMySQLIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

// QuoteMySQLString 转义 SQL 字符串字面量，不接受未经处理的密码拼接。
func QuoteMySQLString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	value = strings.ReplaceAll(value, "\x00", "\\0")
	return "'" + value + "'"
}

// Exec 在超时上下文中执行单条 SQL。
func (cliMySQLExecutor) Exec(ctx context.Context, target MySQLTarget, statement string) error {
	_, err := cliMySQLExecutor{}.ExecOutput(ctx, target, statement)
	return err
}

// ExecOutput 执行 SQL 并返回 CLI 的表格/行输出。
func (cliMySQLExecutor) ExecOutput(ctx context.Context, target MySQLTarget, statement string) (string, error) {
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
	args, env, err := mysqlCLICommand(target)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = databaseCLIEnvironment(env)
	if target.ContainerName != "" {
		input, inputErr := mysqlContainerInput(target.Password, statement)
		if inputErr != nil {
			return "", inputErr
		}
		cmd.Stdin = strings.NewReader(input)
	} else {
		cmd.Stdin = strings.NewReader(statement + "\n")
	}
	out, err := cmd.CombinedOutput()
	message := strings.TrimSpace(string(out))
	if ctx.Err() != nil {
		return "", fmt.Errorf("MySQL 执行超时: %w", ctx.Err())
	}
	if err != nil {
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("MySQL 执行失败: %s", redactMySQLOutput(message, target.Password, statement))
	}
	return message, nil
}

// mysqlCLICommand 组装本机或 Docker 容器内的 CLI 参数，禁止 shell 解释。
func mysqlCLICommand(target MySQLTarget) ([]string, []string, error) {
	typ := strings.ToLower(strings.TrimSpace(target.Type))
	bin := strings.TrimSpace(os.Getenv("WORKMESH_MYSQL_BIN"))
	if bin == "" && typ == "mariadb" {
		bin = strings.TrimSpace(os.Getenv("WORKMESH_MARIADB_BIN"))
	}
	if bin == "" {
		bin = "mysql"
		if typ == "mariadb" {
			bin = "mariadb"
		}
	}
	host := strings.TrimSpace(target.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	if err := ValidateMySQLHost(host); err != nil {
		return nil, nil, err
	}
	port := target.Port
	if port <= 0 {
		port = 3306
	}
	if port > 65535 {
		return nil, nil, errors.New("MySQL 端口无效")
	}
	user := strings.TrimSpace(target.Username)
	if user == "" {
		return nil, nil, errors.New("MySQL 管理用户名不能为空")
	}
	args := []string{bin, "--protocol=tcp", "-h", host, "-P", strconv.Itoa(port), "-u", user, "--batch", "--skip-column-names"}
	env := []string{"MYSQL_PWD=" + target.Password}
	if target.ContainerName != "" {
		if err := ValidateDockerIdentifier(target.ContainerName); err != nil {
			return nil, nil, err
		}
		docker := dockerBinary()
		if docker == "" {
			return nil, nil, errors.New("MySQL Docker CLI 不可用: docker CLI 未安装")
		}
		args = append([]string{docker, "exec", "-i", target.ContainerName, "sh", "-c", mysqlContainerScript, "--"}, args...)
		env = nil
	} else if _, err := exec.LookPath(bin); err != nil {
		return nil, nil, fmt.Errorf("MySQL CLI 不可用: %w", err)
	}
	return args, env, nil
}

// databaseCLIEnvironment removes ambient database credentials before starting
// either a local client or Docker. Local compatibility credentials are then
// appended explicitly; container credentials are delivered by stdin.
func databaseCLIEnvironment(extra []string) []string {
	blocked := map[string]struct{}{
		"MYSQL_PWD":     {},
		"PGPASSWORD":    {},
		"REDISCLI_AUTH": {},
	}
	result := make([]string, 0, len(os.Environ())+len(extra))
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, blockedKey := blocked[key]; blockedKey {
				continue
			}
		}
		result = append(result, item)
	}
	return append(result, extra...)
}

// ValidateDockerIdentifier 校验容器名，避免参数边界被伪造。
func ValidateDockerIdentifier(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > 128 || strings.ContainsAny(value, " \t\r\n\x00") {
		return errors.New("Docker 容器名无效")
	}
	return nil
}

const mysqlContainerScript = `set -eu
IFS= read -r _wm_pw_len
case "$_wm_pw_len" in
  ''|*[!0-9]*) exit 64 ;;
esac
_wm_pw=$(dd bs=1 count="$_wm_pw_len" 2>/dev/null; printf '\001')
_wm_pw=${_wm_pw%?}
IFS= read -r _wm_separator
export MYSQL_PWD="$_wm_pw"
exec "$@"
`

// mysqlContainerInput implements a length-prefixed stdin protocol. The
// password is never part of the Docker argv or the host process environment;
// the fixed container script reads it and leaves the remaining bytes for SQL.
func mysqlContainerInput(password, statement string) (string, error) {
	if strings.IndexByte(password, 0) >= 0 {
		return "", errors.New("MySQL 密码包含不支持的空字节")
	}
	return strconv.Itoa(len(password)) + "\n" + password + "\n" + statement + "\n", nil
}

func redactMySQLOutput(value string, sensitive ...string) string {
	for _, secret := range sensitive {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	for _, marker := range []string{"MYSQL_PWD=", "using password", "-p"} {
		if index := strings.Index(strings.ToLower(value), strings.ToLower(marker)); index >= 0 {
			value = value[:index] + "[redacted]"
		}
	}
	if len(value) > 2048 {
		value = value[:2048]
	}
	return value
}

// CreateMySQLDatabase 在目标实例创建数据库，必要时同时创建用户并授权。
func CreateMySQLDatabase(ctx context.Context, executor MySQLExecutor, target MySQLTarget, name, format, collation, username, password, host string) error {
	if err := ValidateMySQLIdentifier(name); err != nil {
		return err
	}
	sql := "CREATE DATABASE " + QuoteMySQLIdentifier(name)
	if format != "" {
		if err := ValidateMySQLIdentifier(format); err != nil {
			return err
		}
		sql += " DEFAULT CHARACTER SET " + QuoteMySQLIdentifier(format)
	}
	if collation != "" {
		if err := ValidateMySQLIdentifier(collation); err != nil {
			return err
		}
		sql += " COLLATE " + QuoteMySQLIdentifier(collation)
	}
	if err := executor.Exec(ctx, target, sql+";"); err != nil {
		return err
	}
	if strings.TrimSpace(username) == "" {
		return nil
	}
	if err := createMySQLUserAndGrant(ctx, executor, target, name, username, password, host); err != nil {
		_ = executor.Exec(ctx, target, "DROP DATABASE IF EXISTS "+QuoteMySQLIdentifier(name)+";")
		return err
	}
	return nil
}

func createMySQLUserAndGrant(ctx context.Context, executor MySQLExecutor, target MySQLTarget, database, username, password, host string) error {
	if err := ValidateMySQLIdentifier(username); err != nil {
		return err
	}
	if strings.TrimSpace(host) == "" {
		host = "%"
	}
	if err := ValidateMySQLHost(host); err != nil {
		return err
	}
	identity := QuoteMySQLString(username) + "@" + QuoteMySQLString(host)
	if err := executor.Exec(ctx, target, "CREATE USER "+identity+" IDENTIFIED BY "+QuoteMySQLString(password)+";"); err != nil {
		return err
	}
	grant := "GRANT ALL PRIVILEGES ON " + QuoteMySQLIdentifier(database) + ".* TO " + identity + " WITH GRANT OPTION;"
	return executor.Exec(ctx, target, grant)
}

// CreateMySQLUser 创建真实用户，可选地为多个数据库授权。
func CreateMySQLUser(ctx context.Context, executor MySQLExecutor, target MySQLTarget, username, password, host string, databases []string) error {
	if err := createMySQLUserAndGrantNoDatabase(ctx, executor, target, username, password, host); err != nil {
		return err
	}
	for _, database := range databases {
		if err := GrantMySQLUser(ctx, executor, target, database, username, host); err != nil {
			_ = DropMySQLUser(ctx, executor, target, username, host)
			return err
		}
	}
	return nil
}

func createMySQLUserAndGrantNoDatabase(ctx context.Context, executor MySQLExecutor, target MySQLTarget, username, password, host string) error {
	if err := ValidateMySQLIdentifier(username); err != nil {
		return err
	}
	if strings.TrimSpace(host) == "" {
		host = "%"
	}
	if err := ValidateMySQLHost(host); err != nil {
		return err
	}
	identity := QuoteMySQLString(username) + "@" + QuoteMySQLString(host)
	return executor.Exec(ctx, target, "CREATE USER "+identity+" IDENTIFIED BY "+QuoteMySQLString(password)+";")
}

// DropMySQLUser 删除真实用户。
func DropMySQLUser(ctx context.Context, executor MySQLExecutor, target MySQLTarget, username, host string) error {
	if err := ValidateMySQLIdentifier(username); err != nil {
		return err
	}
	if strings.TrimSpace(host) == "" {
		host = "%"
	}
	if err := ValidateMySQLHost(host); err != nil {
		return err
	}
	return executor.Exec(ctx, target, "DROP USER IF EXISTS "+QuoteMySQLString(username)+"@"+QuoteMySQLString(host)+";")
}

// AlterMySQLUserHost 修改真实用户的主机来源。
func AlterMySQLUserHost(ctx context.Context, executor MySQLExecutor, target MySQLTarget, username, oldHost, newHost string) error {
	if err := ValidateMySQLIdentifier(username); err != nil {
		return err
	}
	if err := ValidateMySQLHost(oldHost); err != nil {
		return err
	}
	if err := ValidateMySQLHost(newHost); err != nil {
		return err
	}
	oldIdentity := QuoteMySQLString(username) + "@" + QuoteMySQLString(oldHost)
	newIdentity := QuoteMySQLString(username) + "@" + QuoteMySQLString(newHost)
	return executor.Exec(ctx, target, "RENAME USER "+oldIdentity+" TO "+newIdentity+";")
}

// ChangeMySQLUserPassword 修改真实用户密码。
func ChangeMySQLUserPassword(ctx context.Context, executor MySQLExecutor, target MySQLTarget, username, host, password string) error {
	if err := ValidateMySQLIdentifier(username); err != nil {
		return err
	}
	if host == "" {
		host = "%"
	}
	if err := ValidateMySQLHost(host); err != nil {
		return err
	}
	identity := QuoteMySQLString(username) + "@" + QuoteMySQLString(host)
	return executor.Exec(ctx, target, "ALTER USER "+identity+" IDENTIFIED BY "+QuoteMySQLString(password)+";")
}

// GrantMySQLUser 授予真实数据库权限。
func GrantMySQLUser(ctx context.Context, executor MySQLExecutor, target MySQLTarget, database, username, host string) error {
	if err := ValidateMySQLIdentifier(database); err != nil {
		return err
	}
	if err := ValidateMySQLIdentifier(username); err != nil {
		return err
	}
	if host == "" {
		host = "%"
	}
	if err := ValidateMySQLHost(host); err != nil {
		return err
	}
	identity := QuoteMySQLString(username) + "@" + QuoteMySQLString(host)
	return executor.Exec(ctx, target, "GRANT ALL PRIVILEGES ON "+QuoteMySQLIdentifier(database)+".* TO "+identity+" WITH GRANT OPTION;")
}

// RevokeMySQLGrant 撤销真实数据库权限。
func RevokeMySQLGrant(ctx context.Context, executor MySQLExecutor, target MySQLTarget, database, username, host string) error {
	if err := ValidateMySQLIdentifier(database); err != nil {
		return err
	}
	if err := ValidateMySQLIdentifier(username); err != nil {
		return err
	}
	if host == "" {
		host = "%"
	}
	if err := ValidateMySQLHost(host); err != nil {
		return err
	}
	identity := QuoteMySQLString(username) + "@" + QuoteMySQLString(host)
	return executor.Exec(ctx, target, "REVOKE ALL PRIVILEGES, GRANT OPTION FROM "+identity+";")
}

// DropMySQLDatabase 删除真实数据库。
func DropMySQLDatabase(ctx context.Context, executor MySQLExecutor, target MySQLTarget, name string) error {
	if err := ValidateMySQLIdentifier(name); err != nil {
		return err
	}
	return executor.Exec(ctx, target, "DROP DATABASE IF EXISTS "+QuoteMySQLIdentifier(name)+";")
}

// ListMySQLDatabases 查询非系统 schema，供从远程实例同步数据库登记。
func ListMySQLDatabases(ctx context.Context, executor MySQLQueryExecutor, target MySQLTarget) ([]string, error) {
	output, err := executor.ExecOutput(ctx, target, "SELECT SCHEMA_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME NOT IN ('information_schema','mysql','performance_schema','sys');")
	if err != nil {
		return nil, err
	}
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		// MariaDB CLI may emit a connection warning on stdout before tabular
		// output; it is diagnostic text, not a schema name.
		lowerName := strings.ToLower(name)
		if strings.HasPrefix(lowerName, "warning:") || strings.Contains(lowerName, "[warning]") {
			continue
		}
		if err := ValidateMySQLIdentifier(name); err != nil {
			return nil, err
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

// ValidateMySQLVariableName 校验可用于 SET GLOBAL 的变量名。
func ValidateMySQLVariableName(value string) error {
	if !mysqlVariableName.MatchString(strings.TrimSpace(value)) || len(value) > 128 {
		return fmt.Errorf("MySQL 变量名无效: %q", value)
	}
	return nil
}

// ListMySQLVariables 查询 MySQL 全局变量并返回键值对。
func ListMySQLVariables(ctx context.Context, executor MySQLQueryExecutor, target MySQLTarget) (map[string]string, error) {
	output, err := executor.ExecOutput(ctx, target, "SHOW GLOBAL VARIABLES;")
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r", ""), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		if err := ValidateMySQLVariableName(parts[0]); err != nil {
			continue
		}
		result[parts[0]] = parts[1]
	}
	return result, nil
}

// SetMySQLVariable 修改单个 MySQL 全局变量。
func SetMySQLVariable(ctx context.Context, executor MySQLExecutor, target MySQLTarget, name, value string) error {
	if err := ValidateMySQLVariableName(name); err != nil {
		return err
	}
	if strings.ContainsAny(value, "\x00\r\n") || len(value) > 2048 {
		return errors.New("MySQL 变量值无效")
	}
	literal := QuoteMySQLString(value)
	if numericMySQLValue.MatchString(strings.TrimSpace(value)) {
		literal = strings.TrimSpace(value)
	}
	return executor.Exec(ctx, target, "SET GLOBAL "+QuoteMySQLIdentifier(name)+" = "+literal+";")
}

// ChangeMySQLRootAccess 修改 root 的远程访问来源，并在开启时授予完整权限。
func ChangeMySQLRootAccess(ctx context.Context, executor MySQLExecutor, target MySQLTarget, host string) error {
	if err := ValidateMySQLHost(host); err != nil {
		return err
	}
	if host == "" {
		return errors.New("MySQL root 访问主机不能为空")
	}
	if host == "%" {
		identity := QuoteMySQLString("root") + "@" + QuoteMySQLString(host)
		if err := executor.Exec(ctx, target, "CREATE USER IF NOT EXISTS "+identity+" IDENTIFIED BY "+QuoteMySQLString(target.Password)+";"); err != nil {
			return err
		}
		return executor.Exec(ctx, target, "GRANT ALL PRIVILEGES ON *.* TO "+identity+" WITH GRANT OPTION;")
	}
	return executor.Exec(ctx, target, "DROP USER IF EXISTS "+QuoteMySQLString("root")+"@'%';")
}
