// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MongoDBTarget describes the administrative connection used by mongosh.
// Passwords are supplied through stdin to avoid exposing them in argv.
type MongoDBTarget struct {
	Type          string
	Host          string
	Port          int
	Username      string
	Password      string
	AuthDatabase  string
	ContainerName string
}

type MongoDBExecutor interface {
	Exec(context.Context, MongoDBTarget, string) (string, error)
}

type cliMongoDBExecutor struct{}

func NewMongoDBExecutor() MongoDBExecutor { return cliMongoDBExecutor{} }

var mongoIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,63}$`)

func ValidateMongoDBIdentifier(value string) error {
	value = strings.TrimSpace(value)
	if !mongoIdentifierPattern.MatchString(value) || strings.HasPrefix(value, "system.") {
		return errors.New("MongoDB 标识符无效")
	}
	return nil
}

func ValidateMongoDBRole(value string) error {
	switch strings.TrimSpace(value) {
	case "dbOwner", "read", "readWrite", "userAdmin":
		return nil
	default:
		return errors.New("MongoDB 权限无效")
	}
}

func mongoJSString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func (cliMongoDBExecutor) Exec(ctx context.Context, target MongoDBTarget, script string) (string, error) {
	if strings.TrimSpace(script) == "" {
		return "", errors.New("MongoDB 脚本不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	args, passwordPrompt, err := mongoDBCLICommand(target)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if passwordPrompt {
		cmd.Stdin = strings.NewReader(target.Password + "\n" + script + "\n")
	} else {
		cmd.Stdin = strings.NewReader(script + "\n")
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", fmt.Errorf("MongoDB 执行超时: %w", ctx.Err())
	}
	message := strings.TrimSpace(string(out))
	if err != nil {
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("MongoDB 执行失败: %s", redactMongoDBOutput(message))
	}
	return normalizeMongoDBOutput(message), nil
}

// normalizeMongoDBOutput 移除通过 stdin 输入密码和脚本时 mongosh 产生的
// 交互提示，只保留脚本真实输出，避免权限读取把 `Enter password` 当成角色。
func normalizeMongoDBOutput(value string) string {
	lines := make([]string, 0)
	for _, raw := range strings.Split(strings.ReplaceAll(value, "\r", ""), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "Enter password:") {
			continue
		}
		if index := strings.Index(line, ">"); index > 0 && index <= 128 {
			prompt := line[:index]
			// mongosh 的提示符可能包含 replica-set 状态和数据库名，但不会
			// 包含 JSON/数组起始符；只对符合提示符形态的前缀做裁剪。
			if !strings.ContainsAny(prompt, "{}[]") {
				line = strings.TrimSpace(line[index+1:])
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func mongoDBCLICommand(target MongoDBTarget) ([]string, bool, error) {
	bin := strings.TrimSpace(os.Getenv("WORKMESH_MONGOSH_BIN"))
	if bin == "" {
		bin = "mongosh"
	}
	host := strings.TrimSpace(target.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	if strings.ContainsAny(host, "\x00\r\n \t") {
		return nil, false, errors.New("MongoDB 主机无效")
	}
	port := target.Port
	if port <= 0 {
		port = 27017
	}
	if port > 65535 {
		return nil, false, errors.New("MongoDB 端口无效")
	}
	args := []string{bin, "--quiet", "--host", host, "--port", strconv.Itoa(port)}
	if target.ContainerName != "" {
		if err := ValidateDockerIdentifier(target.ContainerName); err != nil {
			return nil, false, err
		}
	}
	authDatabase := strings.TrimSpace(target.AuthDatabase)
	if authDatabase == "" {
		authDatabase = "admin"
	}
	if err := ValidateMongoDBIdentifier(authDatabase); err != nil {
		return nil, false, err
	}
	username := strings.TrimSpace(target.Username)
	if username == "" {
		if target.ContainerName == "" {
			if _, err := exec.LookPath(bin); err != nil {
				return nil, false, fmt.Errorf("MongoDB CLI 不可用: %w", err)
			}
		} else {
			return mongoDBContainerCommand(args, target.ContainerName), false, nil
		}
		if target.ContainerName != "" {
			return mongoDBContainerCommand(args, target.ContainerName), false, nil
		}
		return args, false, nil
	}
	args = append(args, "--username", username, "--authenticationDatabase", authDatabase, "--password")
	if target.ContainerName != "" {
		return mongoDBContainerCommand(args, target.ContainerName), true, nil
	}
	if _, err := exec.LookPath(bin); err != nil {
		return nil, false, fmt.Errorf("MongoDB CLI 不可用: %w", err)
	}
	return args, true, nil
}

func mongoDBContainerCommand(args []string, container string) []string {
	return append([]string{DockerBinary(), "exec", "-i", container}, args...)
}

func redactMongoDBOutput(value string) string {
	for _, marker := range []string{"password", "pwd", "MONGODB_PASSWORD"} {
		lower := strings.ToLower(value)
		if index := strings.Index(lower, strings.ToLower(marker)); index >= 0 {
			value = value[:index] + "[redacted]"
		}
	}
	if len(value) > 2048 {
		value = value[:2048]
	}
	return value
}

func CreateMongoDatabase(ctx context.Context, executor MongoDBExecutor, target MongoDBTarget, database, username, password, role string) error {
	if err := ValidateMongoDBIdentifier(database); err != nil {
		return err
	}
	if err := ValidateMongoDBIdentifier(username); err != nil {
		return err
	}
	if err := ValidateMongoDBRole(role); err != nil {
		return err
	}
	if strings.TrimSpace(password) == "" {
		return errors.New("MongoDB 密码不能为空")
	}
	// MongoDB 不允许 collection 名以 '.' 开头；使用原系统同样的
	// `_init` 占位 collection，确保空数据库可以被创建并持久化。
	if _, err := executor.Exec(ctx, target, "db = db.getSiblingDB("+mongoJSString(database)+"); db.createCollection(\"_init\");"); err != nil {
		return err
	}
	statement := "db = db.getSiblingDB(" + mongoJSString(database) + "); db.createUser({user:" + mongoJSString(username) + ",pwd:" + mongoJSString(password) + ",roles:[{role:" + mongoJSString(role) + ",db:" + mongoJSString(database) + "}]});"
	if _, err := executor.Exec(ctx, target, statement); err != nil {
		_, _ = executor.Exec(ctx, target, "db = db.getSiblingDB("+mongoJSString(database)+"); db.dropDatabase();")
		return err
	}
	return nil
}

func BindMongoUser(ctx context.Context, executor MongoDBExecutor, target MongoDBTarget, database, username, password string) error {
	if err := ValidateMongoDBIdentifier(database); err != nil {
		return err
	}
	if err := ValidateMongoDBIdentifier(username); err != nil {
		return err
	}
	if strings.TrimSpace(password) == "" {
		return errors.New("MongoDB 密码不能为空")
	}
	statement := "db = db.getSiblingDB(" + mongoJSString(database) + "); db.createUser({user:" + mongoJSString(username) + ",pwd:" + mongoJSString(password) + ",roles:[{role:\"readWrite\",db:" + mongoJSString(database) + "}]});"
	_, err := executor.Exec(ctx, target, statement)
	return err
}

func ChangeMongoPassword(ctx context.Context, executor MongoDBExecutor, target MongoDBTarget, database, username, password string) error {
	if err := ValidateMongoDBIdentifier(database); err != nil {
		return err
	}
	if err := ValidateMongoDBIdentifier(username); err != nil {
		return err
	}
	if strings.TrimSpace(password) == "" {
		return errors.New("MongoDB 密码不能为空")
	}
	_, err := executor.Exec(ctx, target, "db = db.getSiblingDB("+mongoJSString(database)+"); db.changeUserPassword("+mongoJSString(username)+","+mongoJSString(password)+");")
	return err
}

func ChangeMongoPrivileges(ctx context.Context, executor MongoDBExecutor, target MongoDBTarget, database, username, role string) error {
	if err := ValidateMongoDBIdentifier(database); err != nil {
		return err
	}
	if err := ValidateMongoDBIdentifier(username); err != nil {
		return err
	}
	if err := ValidateMongoDBRole(role); err != nil {
		return err
	}
	_, err := executor.Exec(ctx, target, "db = db.getSiblingDB("+mongoJSString(database)+"); db.updateUser("+mongoJSString(username)+",{roles:[{role:"+mongoJSString(role)+",db:"+mongoJSString(database)+"}]});")
	return err
}

func MongoPrivilegesScript(database, username string) (string, error) {
	if err := ValidateMongoDBIdentifier(database); err != nil {
		return "", err
	}
	if err := ValidateMongoDBIdentifier(username); err != nil {
		return "", err
	}
	return "db = db.getSiblingDB(" + mongoJSString(database) + "); var u=db.getUser(" + mongoJSString(username) + "); print(u && u.roles && u.roles.length ? u.roles[0].role : \"\");", nil
}

// ListMongoDatabases 返回实例中的非系统数据库名称，供“从服务器同步”使用。
func ListMongoDatabases(ctx context.Context, executor MongoDBExecutor, target MongoDBTarget) ([]string, error) {
	script := `var r=db.adminCommand({listDatabases:1,nameOnly:true}); print(JSON.stringify(r.databases.map(function(x){return x.name;}).filter(function(x){return x!=="admin"&&x!=="config"&&x!=="local";})));`
	output, err := executor.Exec(ctx, target, script)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if !strings.HasPrefix(line, "[") {
			continue
		}
		var names []string
		if err := json.Unmarshal([]byte(line), &names); err != nil {
			continue
		}
		result := make([]string, 0, len(names))
		seen := make(map[string]struct{}, len(names))
		for _, name := range names {
			if err := ValidateMongoDBIdentifier(name); err != nil {
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
	return nil, errors.New("MongoDB 数据库列表响应无效")
}

func DropMongoDatabase(ctx context.Context, executor MongoDBExecutor, target MongoDBTarget, database, username string) error {
	if err := ValidateMongoDBIdentifier(database); err != nil {
		return err
	}
	if username != "" {
		if err := ValidateMongoDBIdentifier(username); err != nil {
			return err
		}
	}
	// 用户属于目标数据库。原系统使用 dropAllUsersFromDatabase 后再删库，
	// 避免切到 admin 后删除不到目标库用户，也覆盖 bind 创建的多个用户。
	statement := "db = db.getSiblingDB(" + mongoJSString(database) + "); db.runCommand({dropAllUsersFromDatabase:1}); db.dropDatabase();"
	_, err := executor.Exec(ctx, target, statement)
	return err
}

func DropMongoUser(ctx context.Context, executor MongoDBExecutor, target MongoDBTarget, database, username string) error {
	if err := ValidateMongoDBIdentifier(database); err != nil {
		return err
	}
	if err := ValidateMongoDBIdentifier(username); err != nil {
		return err
	}
	_, err := executor.Exec(ctx, target, "db = db.getSiblingDB("+mongoJSString(database)+"); db.dropUser("+mongoJSString(username)+");")
	return err
}
