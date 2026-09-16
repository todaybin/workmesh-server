// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// DatabaseBackupCommandRunner 执行单条数据库备份命令，便于测试时替换进程启动。
type DatabaseBackupCommandRunner interface {
	Run(context.Context, DatabaseBackupCommand) (DatabaseBackupCommandResult, error)
}

// DockerDatabaseBackupExecutor 将备份计划映射为本机 Docker CLI 和容器内 CLI。
//
// 容器密码不会作为 docker 参数或宿主机环境变量传递。对于需要认证的
// 容器命令，执行器向固定脚本的 stdin 写入密码，脚本再设置容器内环境变量。
type DockerDatabaseBackupExecutor struct {
	Runner DatabaseBackupCommandRunner
}

// NewDockerDatabaseBackupExecutor 创建真实 Docker 备份执行器。
func NewDockerDatabaseBackupExecutor() DatabaseBackupCommandExecutor {
	return DockerDatabaseBackupExecutor{}
}

// Execute 执行一条备份命令，并在备份输出成功后原子替换目标文件。
func (e DockerDatabaseBackupExecutor) Execute(ctx context.Context, command DatabaseBackupCommand) (DatabaseBackupCommandResult, error) {
	if e.Runner != nil {
		return e.Runner.Run(ctx, command)
	}
	return runDatabaseBackupCommand(ctx, command)
}

type backupCommandRunner struct{}

func (backupCommandRunner) Run(ctx context.Context, command DatabaseBackupCommand) (DatabaseBackupCommandResult, error) {
	return runDatabaseBackupCommand(ctx, command)
}

func runDatabaseBackupCommand(ctx context.Context, command DatabaseBackupCommand) (DatabaseBackupCommandResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(command.Program) == "" {
		return DatabaseBackupCommandResult{}, errors.New("备份命令程序不能为空")
	}
	if err := ValidateDatabaseBackupArgs(command); err != nil {
		return DatabaseBackupCommandResult{}, err
	}
	if isRedisBackupCopyCommand(command) {
		return runRedisBackupCopyCommand(ctx, command)
	}

	args := append([]string(nil), command.Args...)
	program := command.Program
	if command.Container != "" {
		var err error
		program, args, err = databaseBackupContainerCommand(command)
		if err != nil {
			return DatabaseBackupCommandResult{}, err
		}
	} else if filepath.Base(strings.TrimSpace(program)) == "docker" {
		program = DockerBinary()
	}

	var input io.ReadCloser
	if command.Container != "" {
		password := databaseBackupPassword(command.Environment)
		prefix, err := databaseBackupCredentialPrefix(password)
		if err != nil {
			return DatabaseBackupCommandResult{}, err
		}
		if command.StdinPath != "" {
			file, err := openDatabaseBackupInput(command.StdinPath)
			if err != nil {
				return DatabaseBackupCommandResult{}, err
			}
			input = &multiReaderCloser{
				Reader: io.MultiReader(strings.NewReader(prefix), file),
				close:  file.Close,
			}
		} else {
			input = io.NopCloser(strings.NewReader(prefix))
		}
	} else if command.StdinPath != "" {
		file, err := openDatabaseBackupInput(command.StdinPath)
		if err != nil {
			return DatabaseBackupCommandResult{}, err
		}
		input = file
	}
	if input != nil {
		defer input.Close()
	}

	var stdout bytes.Buffer
	var stderr limitedBackupBuffer
	var outputFile *os.File
	var outputTemp string
	if command.StdoutPath != "" {
		outputFile, outputTemp, err := createDatabaseBackupOutput(command.StdoutPath)
		if err != nil {
			return DatabaseBackupCommandResult{}, err
		}
		defer func() {
			_ = outputFile.Close()
			_ = os.Remove(outputTemp)
		}()
	}

	commandCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	process := exec.CommandContext(commandCtx, program, args...)
	process.Env = os.Environ()
	if command.Container == "" {
		for key, value := range command.Environment {
			if !validBackupEnvironmentKey(key) {
				return DatabaseBackupCommandResult{}, fmt.Errorf("备份环境变量名无效: %q", key)
			}
			process.Env = append(process.Env, key+"="+value)
		}
	}
	if input != nil {
		process.Stdin = input
	}
	if outputFile != nil {
		process.Stdout = outputFile
	} else {
		process.Stdout = &stdout
	}
	process.Stderr = &stderr

	if err := process.Run(); err != nil {
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
			return DatabaseBackupCommandResult{}, context.DeadlineExceeded
		}
		result := DatabaseBackupCommandResult{ExitCode: backupExitCode(err), Stdout: stdout.String(), Stderr: stderr.String()}
		return result, err
	}
	if outputFile != nil {
		if err := outputFile.Close(); err != nil {
			return DatabaseBackupCommandResult{}, fmt.Errorf("关闭备份临时文件失败: %w", err)
		}
		if err := os.Rename(outputTemp, command.StdoutPath); err != nil {
			return DatabaseBackupCommandResult{}, fmt.Errorf("提交备份文件失败: %w", err)
		}
		outputTemp = ""
	}
	return DatabaseBackupCommandResult{ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

func isRedisBackupCopyCommand(command DatabaseBackupCommand) bool {
	if filepath.Base(strings.TrimSpace(command.Program)) != "docker" {
		return false
	}
	if len(command.Args) != 3 || command.Args[0] != "cp" {
		return false
	}
	switch command.Name {
	case "redis-copy-backup", "redis-backup-existing-rdb", "redis-restore-copy", "redis-rollback-copy":
		return true
	default:
		return command.StreamMode == DatabaseBackupStreamStdoutToArtifact && command.StdoutPath != ""
	}
}

func runRedisBackupCopyCommand(ctx context.Context, command DatabaseBackupCommand) (DatabaseBackupCommandResult, error) {
	source, destination := command.Args[1], command.Args[2]
	sourceContainer, sourcePath, sourceIsContainer := parseRedisContainerPath(source)
	destinationContainer, destinationPath, destinationIsContainer := parseRedisContainerPath(destination)
	if sourceIsContainer == destinationIsContainer {
		return DatabaseBackupCommandResult{}, errors.New("Redis 备份复制方向无效")
	}

	if sourceIsContainer {
		if sourcePath != redisDumpPath {
			return DatabaseBackupCommandResult{}, errors.New("Redis 备份源路径无效")
		}
		if err := validateDatabaseBackupContainer(sourceContainer); err != nil {
			return DatabaseBackupCommandResult{}, err
		}
		artifact := strings.TrimSpace(command.StdoutPath)
		if artifact == "" {
			artifact = destination
		}
		if destinationIsContainer || artifact != destination {
			return DatabaseBackupCommandResult{}, errors.New("Redis 备份目标路径无效")
		}
		return runRedisBackupCopyToHost(ctx, command, artifact)
	}

	if destinationPath != redisDumpPath {
		return DatabaseBackupCommandResult{}, errors.New("Redis 恢复目标路径无效")
	}
	if err := validateDatabaseBackupContainer(destinationContainer); err != nil {
		return DatabaseBackupCommandResult{}, err
	}
	input := source
	if command.StdinPath != "" {
		if filepath.Clean(command.StdinPath) != filepath.Clean(source) {
			return DatabaseBackupCommandResult{}, errors.New("Redis 恢复输入路径不一致")
		}
		input = command.StdinPath
	}
	if err := validateDatabaseBackupInputPath(input); err != nil {
		return DatabaseBackupCommandResult{}, err
	}
	rawCommand := command
	rawCommand.Name = command.Name + "-validated"
	rawCommand.StreamMode = DatabaseBackupStreamNone
	rawCommand.StdinPath = ""
	return runDatabaseBackupCommand(ctx, rawCommand)
}

func runRedisBackupCopyToHost(ctx context.Context, command DatabaseBackupCommand, artifact string) (DatabaseBackupCommandResult, error) {
	if err := validateDatabaseBackupOutputPath(artifact); err != nil {
		return DatabaseBackupCommandResult{}, err
	}
	parent := filepath.Dir(artifact)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return DatabaseBackupCommandResult{}, fmt.Errorf("创建 Redis 备份目录失败: %w", err)
	}
	if err := validateDatabaseBackupOutputPath(artifact); err != nil {
		return DatabaseBackupCommandResult{}, err
	}
	stageDir, err := os.MkdirTemp(parent, ".workmesh-redis-copy-*")
	if err != nil {
		return DatabaseBackupCommandResult{}, fmt.Errorf("创建 Redis 备份临时目录失败: %w", err)
	}
	if err := os.Chmod(stageDir, 0o700); err != nil {
		_ = os.RemoveAll(stageDir)
		return DatabaseBackupCommandResult{}, fmt.Errorf("设置 Redis 备份临时目录权限失败: %w", err)
	}
	defer os.RemoveAll(stageDir)

	stagePath := filepath.Join(stageDir, "dump.rdb")
	stagedCommand := command
	stagedCommand.Name = command.Name + "-staged"
	stagedCommand.Args = append([]string(nil), command.Args...)
	stagedCommand.Args[2] = stagePath
	stagedCommand.StreamMode = DatabaseBackupStreamNone
	stagedCommand.StdoutPath = ""
	result, err := runDatabaseBackupCommand(ctx, stagedCommand)
	if err != nil {
		return result, err
	}
	info, err := os.Lstat(stagePath)
	if err != nil {
		return result, errors.New("Redis 备份未生成 RDB 文件")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return result, errors.New("Redis 备份输出不是普通文件")
	}
	if err := os.Chmod(stagePath, 0o600); err != nil {
		return result, fmt.Errorf("设置 Redis 备份文件权限失败: %w", err)
	}
	if info, err := os.Lstat(artifact); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return result, errors.New("Redis 备份不能覆盖符号链接")
		}
		if !info.Mode().IsRegular() {
			return result, errors.New("Redis 备份目标不是普通文件")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, fmt.Errorf("检查 Redis 备份目标失败: %w", err)
	}
	if err := os.Rename(stagePath, artifact); err != nil {
		return result, fmt.Errorf("提交 Redis 备份文件失败: %w", err)
	}
	return result, nil
}

func parseRedisContainerPath(value string) (string, string, bool) {
	index := strings.IndexByte(value, ':')
	if index <= 0 || index == len(value)-1 {
		return "", "", false
	}
	container, path := value[:index], value[index+1:]
	if !databaseBackupContainerPattern.MatchString(container) {
		return "", "", false
	}
	return container, path, true
}

func databaseBackupContainerCommand(command DatabaseBackupCommand) (string, []string, error) {
	if err := validateDatabaseBackupContainer(command.Container); err != nil {
		return "", nil, err
	}
	script, ok := databaseBackupContainerScript(command.Program)
	if !ok {
		return "", nil, fmt.Errorf("不支持的容器备份程序: %s", command.Program)
	}
	args := make([]string, 0, len(command.Args)+6)
	args = append(args, "exec", "-i", command.Container, "sh", "-c", script, "workmesh-db")
	args = append(args, command.Args...)
	return DockerBinary(), args, nil
}

func databaseBackupContainerScript(program string) (string, bool) {
	switch filepath.Base(strings.TrimSpace(program)) {
	case "pg_dump", "pg_restore":
		return postgresContainerScript, true
	case "mariadb-dump", "mysqldump", "mariadb", "mysql":
		return mysqlContainerScript, true
	case "redis-cli":
		return redisContainerScript, true
	default:
		return "", false
	}
}

func databaseBackupPassword(environment map[string]string) string {
	for _, key := range []string{"PGPASSWORD", "MYSQL_PWD", "REDISCLI_AUTH"} {
		if value, ok := environment[key]; ok {
			return value
		}
	}
	return ""
}

func databaseBackupCredentialPrefix(password string) (string, error) {
	if strings.IndexByte(password, 0) >= 0 {
		return "", errors.New("数据库备份密码包含不支持的空字节")
	}
	return strconv.Itoa(len(password)) + "\n" + password + "\n", nil
}

func ValidateDatabaseBackupArgs(command DatabaseBackupCommand) error {
	if command.Container != "" {
		if err := validateDatabaseBackupContainer(command.Container); err != nil {
			return err
		}
		if _, ok := databaseBackupContainerScript(command.Program); !ok {
			return fmt.Errorf("不支持的容器备份程序: %s", command.Program)
		}
		for key := range command.Environment {
			switch key {
			case "PGPASSWORD", "MYSQL_PWD", "REDISCLI_AUTH":
			default:
				return fmt.Errorf("容器备份环境变量不允许: %s", key)
			}
		}
	}
	for _, arg := range command.Args {
		if strings.ContainsAny(arg, "\x00\r\n") {
			return errors.New("备份命令参数包含控制字符")
		}
	}
	return nil
}

func openDatabaseBackupInput(path string) (*os.File, error) {
	if err := validateDatabaseBackupInputPath(path); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("备份文件不存在: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("备份输入必须是普通文件")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开备份文件失败: %w", err)
	}
	return file, nil
}

func createDatabaseBackupOutput(path string) (*os.File, string, error) {
	if err := validateDatabaseBackupOutputPath(path); err != nil {
		return nil, "", err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, "", fmt.Errorf("创建备份目录失败: %w", err)
	}
	if err := validateDatabaseBackupOutputPath(path); err != nil {
		return nil, "", err
	}
	file, err := os.CreateTemp(parent, ".workmesh-backup-*.tmp")
	if err != nil {
		return nil, "", fmt.Errorf("创建备份临时文件失败: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, "", fmt.Errorf("设置备份文件权限失败: %w", err)
	}
	return file, file.Name(), nil
}

func validateDatabaseBackupInputPath(path string) error {
	if err := validateDatabaseBackupOutputPath(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("备份文件不存在: %w", err)
	}
	if err != nil {
		return fmt.Errorf("检查备份输入失败: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("备份输入不能是符号链接")
	}
	if !info.Mode().IsRegular() {
		return errors.New("备份输入必须是普通文件")
	}
	return nil
}

func validateDatabaseBackupOutputPath(path string) error {
	if err := ValidateDatabaseBackupPath(path); err != nil {
		return err
	}
	clean := filepath.Clean(path)
	if err := rejectDatabaseBackupSymlinkComponents(filepath.Dir(clean)); err != nil {
		return err
	}
	info, err := os.Lstat(clean)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("备份输出不能覆盖符号链接")
		}
		if !info.Mode().IsRegular() {
			return errors.New("备份输出必须是普通文件")
		}
	case errors.Is(err, os.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("检查备份输出失败: %w", err)
	}
	return nil
}

func rejectDatabaseBackupSymlinkComponents(path string) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return errors.New("备份路径无效")
	}
	volume := filepath.VolumeName(absolute)
	remainder := strings.TrimPrefix(absolute, volume)
	current := volume
	if strings.HasPrefix(remainder, string(os.PathSeparator)) {
		current += string(os.PathSeparator)
		remainder = strings.TrimPrefix(remainder, string(os.PathSeparator))
	}
	for _, component := range strings.Split(remainder, string(os.PathSeparator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return fmt.Errorf("检查备份目录失败: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("备份路径不能包含符号链接")
		}
		if !info.IsDir() {
			return errors.New("备份路径父项必须是目录")
		}
	}
	return nil
}

func validBackupEnvironmentKey(key string) bool {
	return key != "" && !strings.ContainsAny(key, "=\x00\r\n")
}

func backupExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

type multiReaderCloser struct {
	io.Reader
	close func() error
}

func (m *multiReaderCloser) Close() error {
	if m.close == nil {
		return nil
	}
	return m.close()
}

type limitedBackupBuffer struct {
	data bytes.Buffer
}

func (b *limitedBackupBuffer) Write(p []byte) (int, error) {
	const max = 1 << 20
	remaining := max - b.data.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	_, _ = b.data.Write(p)
	return len(p), nil
}

func (b *limitedBackupBuffer) String() string {
	return b.data.String()
}

var _ DatabaseBackupCommandExecutor = DockerDatabaseBackupExecutor{}
var _ DatabaseBackupCommandRunner = backupCommandRunner{}
