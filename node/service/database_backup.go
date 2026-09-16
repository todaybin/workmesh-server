// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// DatabaseBackupType identifies the database CLI used by a backup plan.
type DatabaseBackupType string

const (
	DatabaseBackupPostgres DatabaseBackupType = "postgres"
	DatabaseBackupMariaDB  DatabaseBackupType = "mariadb"
	DatabaseBackupRedis    DatabaseBackupType = "redis"
)

const databaseBackupSchemaSQL = `
	CREATE TABLE IF NOT EXISTS database_backups (
		id TEXT PRIMARY KEY,
		database_id INTEGER NOT NULL DEFAULT 0,
		database_name TEXT NOT NULL,
		database_type TEXT NOT NULL,
		container_name TEXT NOT NULL,
		artifact_path TEXT NOT NULL,
		artifact_size INTEGER NOT NULL DEFAULT 0,
		sha256 TEXT NOT NULL DEFAULT '',
		operation TEXT NOT NULL,
		status TEXT NOT NULL,
		error TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL,
		finished_at TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_database_backups_database ON database_backups(database_id,started_at DESC);
	CREATE INDEX IF NOT EXISTS idx_database_backups_status ON database_backups(status,started_at DESC);`

// DatabaseBackupSchemaMigration creates the durable backup metadata table.
func DatabaseBackupSchemaMigration() storage.Migration {
	return storage.SQLMigration("0008-database-backup-metadata", databaseBackupSchemaSQL)
}

// DatabaseBackupOperation identifies whether a plan creates or consumes an artifact.
type DatabaseBackupOperation string

const (
	DatabaseBackupOperationBackup  DatabaseBackupOperation = "backup"
	DatabaseBackupOperationRestore DatabaseBackupOperation = "restore"
)

const (
	defaultDatabaseBackupTimeout = 15 * time.Minute
	redisDumpPath                = "/data/dump.rdb"
	redisBGSAVEWaitTimeout       = 2 * time.Minute
	redisBGSAVEWaitAttempts      = 120
	redisBGSAVEWaitInterval      = time.Second
	redisHealthCheckTimeout      = 30 * time.Second
	redisHealthCheckAttempts     = 30
	redisHealthCheckInterval     = time.Second
)

// DatabaseBackupRequest contains only the information needed to build a plan.
// Password is kept in the command environment and is never placed in Args.
type DatabaseBackupRequest struct {
	Type          DatabaseBackupType
	ContainerName string
	DatabaseName  string
	Username      string
	Password      string
	ArtifactPath  string
}

// DatabaseBackupCommand is a logical command. Container is intentionally
// separate from Args so an executor can use a Docker API exec session without
// exposing container credentials through a docker CLI argument.
type DatabaseBackupCommand struct {
	Name            string
	Program         string
	Args            []string
	Container       string
	Environment     map[string]string
	StdinPath       string
	StdoutPath      string
	StreamMode      DatabaseBackupStreamMode
	Timeout         time.Duration
	Retry           DatabaseBackupRetryPolicy
	Verification    DatabaseBackupVerification
	ContinueOnError bool
	Description     string
}

// DatabaseBackupStreamMode describes how a command moves the backup
// artifact. The executor still uses StdinPath and StdoutPath as the concrete
// paths; this field makes the direction explicit in the plan.
type DatabaseBackupStreamMode string

const (
	DatabaseBackupStreamNone             DatabaseBackupStreamMode = "none"
	DatabaseBackupStreamStdoutToArtifact DatabaseBackupStreamMode = "stdout_to_artifact"
	DatabaseBackupStreamArtifactToStdin  DatabaseBackupStreamMode = "artifact_to_stdin"
)

// DatabaseBackupRetryPolicy describes retries for a command, including
// output-verification failures. MaxAttempts is the total number of attempts.
type DatabaseBackupRetryPolicy struct {
	MaxAttempts int
	Delay       time.Duration
}

// DatabaseBackupVerification describes output that must be present before a
// command is considered successful.
type DatabaseBackupVerification struct {
	Kind           string
	RequiredOutput []string
	Description    string
}

// DatabaseBackupCompensation is executed only when one of TriggerCommands
// fails. It is part of the plan rather than the normal command sequence.
type DatabaseBackupCompensation struct {
	Name            string
	TriggerCommands []string
	Commands        []DatabaseBackupCommand
}

// DatabaseBackupPlan is a side-effect-free description of the operation.
type DatabaseBackupPlan struct {
	Type                 DatabaseBackupType
	Operation            DatabaseBackupOperation
	ArtifactPath         string
	RollbackArtifactPath string
	Commands             []DatabaseBackupCommand
	Compensations        []DatabaseBackupCompensation
}

// DatabaseBackupCommandResult is deliberately small so API adapters can map
// it to their own task/result contract later.
type DatabaseBackupCommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// DatabaseBackupExecution contains the plan and the sanitized executor output.
type DatabaseBackupExecution struct {
	Plan                DatabaseBackupPlan
	Results             []DatabaseBackupCommandResult
	CompensationResults []DatabaseBackupCommandResult
}

// DatabaseBackupCommandExecutor is injected by the runtime integration.
// This package does not execute Docker or database commands by itself.
type DatabaseBackupCommandExecutor interface {
	Execute(context.Context, DatabaseBackupCommand) (DatabaseBackupCommandResult, error)
}

// DatabaseBackupService builds plans and optionally executes them through an
// injected executor. A nil executor is valid for plan-only use.
type DatabaseBackupService struct {
	executor DatabaseBackupCommandExecutor
	timeout  time.Duration
}

// NewDatabaseBackupService creates a service without selecting a Docker
// implementation. The executor must be injected before execution.
func NewDatabaseBackupService(executor DatabaseBackupCommandExecutor, timeout time.Duration) DatabaseBackupService {
	if timeout <= 0 {
		timeout = defaultDatabaseBackupTimeout
	}
	return DatabaseBackupService{executor: executor, timeout: timeout}
}

// BuildBackupPlan creates a side-effect-free backup plan.
func (s DatabaseBackupService) BuildBackupPlan(request DatabaseBackupRequest) (DatabaseBackupPlan, error) {
	return buildDatabaseBackupPlan(DatabaseBackupOperationBackup, request)
}

// BuildRestorePlan creates a side-effect-free restore plan.
func (s DatabaseBackupService) BuildRestorePlan(request DatabaseBackupRequest) (DatabaseBackupPlan, error) {
	return buildDatabaseBackupPlan(DatabaseBackupOperationRestore, request)
}

// Backup builds and executes a plan using only the injected executor.
func (s DatabaseBackupService) Backup(ctx context.Context, request DatabaseBackupRequest) (DatabaseBackupExecution, error) {
	plan, err := s.BuildBackupPlan(request)
	if err != nil {
		return DatabaseBackupExecution{}, err
	}
	return s.execute(ctx, plan, request.Password)
}

// Restore builds and executes a plan using only the injected executor.
func (s DatabaseBackupService) Restore(ctx context.Context, request DatabaseBackupRequest) (DatabaseBackupExecution, error) {
	plan, err := s.BuildRestorePlan(request)
	if err != nil {
		return DatabaseBackupExecution{}, err
	}
	return s.execute(ctx, plan, request.Password)
}

func (s DatabaseBackupService) execute(ctx context.Context, plan DatabaseBackupPlan, password string) (DatabaseBackupExecution, error) {
	if s.executor == nil {
		return DatabaseBackupExecution{}, errors.New("数据库备份执行器未注入；当前仅支持计划构造")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := DatabaseBackupExecution{
		Plan:    plan,
		Results: make([]DatabaseBackupCommandResult, 0, len(plan.Commands)),
	}
	for _, command := range plan.Commands {
		commandResult, err := s.executeCommand(ctx, command, password)
		result.Results = append(result.Results, commandResult)
		if err != nil {
			compensationErr := s.executeFailureCompensations(ctx, plan, command.Name, password, &result)
			if compensationErr != nil {
				return result, fmt.Errorf("%s: %w; 失败补偿失败: %v", command.Name, err, compensationErr)
			}
			return result, fmt.Errorf("%s: %w", command.Name, err)
		}
	}
	return result, nil
}

func (s DatabaseBackupService) executeCommand(ctx context.Context, command DatabaseBackupCommand, password string) (DatabaseBackupCommandResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := s.timeout
	if timeout <= 0 {
		timeout = defaultDatabaseBackupTimeout
	}
	if command.Timeout > 0 && command.Timeout < timeout {
		timeout = command.Timeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	attempts := command.Retry.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	var lastResult DatabaseBackupCommandResult
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		rawResult, err := s.executor.Execute(commandCtx, command)
		result := rawResult
		result.Stdout = RedactDatabaseBackupOutput(result.Stdout, password)
		result.Stderr = RedactDatabaseBackupOutput(result.Stderr, password)
		lastResult = result

		if commandCtx.Err() != nil {
			lastErr = commandCtx.Err()
		} else if err != nil {
			lastErr = errors.New(RedactDatabaseBackupOutput(err.Error(), password))
		} else if rawResult.ExitCode != 0 {
			message := strings.TrimSpace(rawResult.Stderr)
			if message == "" {
				message = fmt.Sprintf("命令退出码 %d", rawResult.ExitCode)
			}
			lastErr = errors.New(RedactDatabaseBackupOutput(message, password))
		} else if verificationErr := verifyDatabaseBackupOutput(command.Verification, rawResult.Stdout); verificationErr != nil {
			lastErr = verificationErr
		} else {
			return result, nil
		}

		if attempt == attempts {
			break
		}
		if err := waitDatabaseBackupRetry(commandCtx, command.Retry.Delay); err != nil {
			lastErr = err
			break
		}
	}
	if lastErr == nil {
		lastErr = errors.New("数据库备份命令执行失败")
	}
	if commandCtx.Err() != nil {
		lastErr = commandCtx.Err()
	}
	return lastResult, lastErr
}

func (s DatabaseBackupService) executeFailureCompensations(ctx context.Context, plan DatabaseBackupPlan, failedCommand string, password string, execution *DatabaseBackupExecution) error {
	var firstErr error
	for _, compensation := range plan.Compensations {
		if !containsDatabaseBackupCommand(compensation.TriggerCommands, failedCommand) {
			continue
		}
		for _, command := range compensation.Commands {
			result, err := s.executeCommand(ctx, command, password)
			execution.CompensationResults = append(execution.CompensationResults, result)
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", command.Name, err)
			}
			if err != nil && !command.ContinueOnError {
				break
			}
		}
	}
	return firstErr
}

func containsDatabaseBackupCommand(commands []string, target string) bool {
	for _, command := range commands {
		if command == target {
			return true
		}
	}
	return false
}

func waitDatabaseBackupRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func verifyDatabaseBackupOutput(verification DatabaseBackupVerification, stdout string) error {
	for _, required := range verification.RequiredOutput {
		if !strings.Contains(stdout, required) {
			if verification.Kind == "" {
				return fmt.Errorf("命令输出缺少必要内容: %q", required)
			}
			return fmt.Errorf("%s 校验未通过，缺少 %q", verification.Kind, required)
		}
	}
	return nil
}

func buildDatabaseBackupPlan(operation DatabaseBackupOperation, request DatabaseBackupRequest) (DatabaseBackupPlan, error) {
	if operation != DatabaseBackupOperationBackup && operation != DatabaseBackupOperationRestore {
		return DatabaseBackupPlan{}, errors.New("数据库备份操作类型不支持")
	}
	normalizedType, err := normalizeDatabaseBackupType(request.Type)
	if err != nil {
		return DatabaseBackupPlan{}, err
	}
	if err := validateDatabaseBackupContainer(request.ContainerName); err != nil {
		return DatabaseBackupPlan{}, err
	}
	if err := ValidateDatabaseBackupPath(request.ArtifactPath); err != nil {
		return DatabaseBackupPlan{}, err
	}

	request.Type = normalizedType
	switch request.Type {
	case DatabaseBackupPostgres:
		if err := validateDatabaseBackupName(request.DatabaseName, "名称"); err != nil {
			return DatabaseBackupPlan{}, err
		}
		if err := validateDatabaseBackupName(request.Username, "用户名"); err != nil {
			return DatabaseBackupPlan{}, err
		}
		return buildPostgresBackupPlan(operation, request), nil
	case DatabaseBackupMariaDB:
		if err := validateDatabaseBackupName(request.DatabaseName, "名称"); err != nil {
			return DatabaseBackupPlan{}, err
		}
		if err := validateDatabaseBackupName(request.Username, "用户名"); err != nil {
			return DatabaseBackupPlan{}, err
		}
		return buildMariaDBBackupPlan(operation, request), nil
	case DatabaseBackupRedis:
		plan, err := buildRedisBackupPlan(operation, request)
		if err != nil {
			return DatabaseBackupPlan{}, err
		}
		return plan, nil
	default:
		return DatabaseBackupPlan{}, fmt.Errorf("未知数据库类型: %q", request.Type)
	}
}

func buildPostgresBackupPlan(operation DatabaseBackupOperation, request DatabaseBackupRequest) DatabaseBackupPlan {
	env := map[string]string{"PGPASSWORD": request.Password}
	if operation == DatabaseBackupOperationBackup {
		return DatabaseBackupPlan{
			Type:         DatabaseBackupPostgres,
			Operation:    operation,
			ArtifactPath: request.ArtifactPath,
			Commands: []DatabaseBackupCommand{{
				Name:        "pg_dump",
				Program:     "pg_dump",
				Args:        []string{"--format=custom", "--no-owner", "--dbname", request.DatabaseName, "--username", request.Username, "--file=-"},
				Container:   request.ContainerName,
				Environment: env,
				StdoutPath:  request.ArtifactPath,
				StreamMode:  DatabaseBackupStreamStdoutToArtifact,
				Timeout:     defaultDatabaseBackupTimeout,
				Description: "容器内 pg_dump 的 stdout 流入受控备份文件；超时取消整个导出并清理临时文件",
			}},
		}
	}
	return DatabaseBackupPlan{
		Type:         DatabaseBackupPostgres,
		Operation:    operation,
		ArtifactPath: request.ArtifactPath,
		Commands: []DatabaseBackupCommand{{
			Name:        "pg_restore",
			Program:     "pg_restore",
			Args:        []string{"--clean", "--if-exists", "--no-owner", "--dbname", request.DatabaseName, "--username", request.Username, "-"},
			Container:   request.ContainerName,
			Environment: env,
			StdinPath:   request.ArtifactPath,
			StreamMode:  DatabaseBackupStreamArtifactToStdin,
			Timeout:     defaultDatabaseBackupTimeout,
			Description: "受控备份文件通过 stdin 流入容器内 pg_restore；超时取消整个恢复",
		}},
	}
}

func buildMariaDBBackupPlan(operation DatabaseBackupOperation, request DatabaseBackupRequest) DatabaseBackupPlan {
	env := map[string]string{"MYSQL_PWD": request.Password}
	if operation == DatabaseBackupOperationBackup {
		return DatabaseBackupPlan{
			Type:         DatabaseBackupMariaDB,
			Operation:    operation,
			ArtifactPath: request.ArtifactPath,
			Commands: []DatabaseBackupCommand{{
				Name:        "mariadb-dump",
				Program:     "mariadb-dump",
				Args:        []string{"--single-transaction", "--routines", "--events", "--triggers", "--databases", request.DatabaseName, "--user", request.Username},
				Container:   request.ContainerName,
				Environment: env,
				StdoutPath:  request.ArtifactPath,
				StreamMode:  DatabaseBackupStreamStdoutToArtifact,
				Timeout:     defaultDatabaseBackupTimeout,
				Description: "容器内 mariadb-dump 的 stdout 流入受控 SQL 文件；超时取消整个导出并清理临时文件",
			}},
		}
	}
	return DatabaseBackupPlan{
		Type:         DatabaseBackupMariaDB,
		Operation:    operation,
		ArtifactPath: request.ArtifactPath,
		Commands: []DatabaseBackupCommand{{
			Name:        "mariadb",
			Program:     "mariadb",
			Args:        []string{"--database", request.DatabaseName, "--user", request.Username},
			Container:   request.ContainerName,
			Environment: env,
			StdinPath:   request.ArtifactPath,
			StreamMode:  DatabaseBackupStreamArtifactToStdin,
			Timeout:     defaultDatabaseBackupTimeout,
			Description: "受控 SQL 文件通过 stdin 流入容器内 mariadb；超时取消整个恢复",
		}},
	}
}

func buildRedisBackupPlan(operation DatabaseBackupOperation, request DatabaseBackupRequest) (DatabaseBackupPlan, error) {
	auth := map[string]string{"REDISCLI_AUTH": request.Password}
	if operation == DatabaseBackupOperationBackup {
		return DatabaseBackupPlan{
			Type:         DatabaseBackupRedis,
			Operation:    operation,
			ArtifactPath: request.ArtifactPath,
			Commands: []DatabaseBackupCommand{
				{
					Name:        "redis-bgsave",
					Program:     "redis-cli",
					Args:        []string{"--raw", "BGSAVE"},
					Container:   request.ContainerName,
					Environment: auth,
					Timeout:     defaultDatabaseBackupTimeout,
					Description: "通过容器内 redis-cli 触发 Redis BGSAVE；完成状态由后续持久化检查确认",
				},
				{
					Name:        "redis-wait-bgsave",
					Program:     "redis-cli",
					Args:        []string{"--raw", "INFO", "persistence"},
					Container:   request.ContainerName,
					Environment: auth,
					Timeout:     redisBGSAVEWaitTimeout,
					Retry: DatabaseBackupRetryPolicy{
						MaxAttempts: redisBGSAVEWaitAttempts,
						Delay:       redisBGSAVEWaitInterval,
					},
					Verification: DatabaseBackupVerification{
						Kind:           "redis_bgsave_complete",
						RequiredOutput: []string{"rdb_bgsave_in_progress:0", "rdb_last_bgsave_status:ok"},
						Description:    "只有确认后台保存已结束且最后一次保存成功，才允许复制 dump.rdb",
					},
					Description: "轮询 Redis INFO persistence，验证 BGSAVE 完成后再继续",
				},
				{
					Name:        "redis-copy-backup",
					Program:     "docker",
					Args:        []string{"cp", request.ContainerName + ":" + redisDumpPath, request.ArtifactPath},
					Timeout:     defaultDatabaseBackupTimeout,
					Description: "复制 Redis RDB 文件到受控备份路径",
				},
			},
		}, nil
	}
	rollbackPath := request.ArtifactPath + ".pre-restore"
	if err := ValidateDatabaseBackupPath(rollbackPath); err != nil {
		return DatabaseBackupPlan{}, fmt.Errorf("Redis 恢复回滚路径无效: %w", err)
	}
	healthCheck := func(name string) DatabaseBackupCommand {
		return DatabaseBackupCommand{
			Name:        name,
			Program:     "redis-cli",
			Args:        []string{"--raw", "PING"},
			Container:   request.ContainerName,
			Environment: auth,
			Timeout:     redisHealthCheckTimeout,
			Retry: DatabaseBackupRetryPolicy{
				MaxAttempts: redisHealthCheckAttempts,
				Delay:       redisHealthCheckInterval,
			},
			Verification: DatabaseBackupVerification{
				Kind:           "redis_health",
				RequiredOutput: []string{"PONG"},
				Description:    "Redis 启动后必须通过 PING 健康检查",
			},
			Description: "启动 Redis 后轮询 PING，确认容器内服务可用",
		}
	}
	return DatabaseBackupPlan{
		Type:                 DatabaseBackupRedis,
		Operation:            operation,
		ArtifactPath:         request.ArtifactPath,
		RollbackArtifactPath: rollbackPath,
		Commands: []DatabaseBackupCommand{
			{
				Name:        "redis-stop-for-restore",
				Program:     "docker",
				Args:        []string{"stop", request.ContainerName},
				Timeout:     defaultDatabaseBackupTimeout,
				Description: "停止 Redis 容器以保证 RDB 恢复一致性",
			},
			{
				Name:        "redis-backup-existing-rdb",
				Program:     "docker",
				Args:        []string{"cp", request.ContainerName + ":" + redisDumpPath, rollbackPath},
				Timeout:     defaultDatabaseBackupTimeout,
				Description: "恢复前先把当前 RDB 复制到独立回滚文件",
			},
			{
				Name:        "redis-restore-copy",
				Program:     "docker",
				Args:        []string{"cp", request.ArtifactPath, request.ContainerName + ":" + redisDumpPath},
				Timeout:     defaultDatabaseBackupTimeout,
				Description: "将待恢复 RDB 复制到 Redis 数据目录",
			},
			{
				Name:        "redis-start-after-restore",
				Program:     "docker",
				Args:        []string{"start", request.ContainerName},
				Timeout:     defaultDatabaseBackupTimeout,
				Description: "启动 Redis 容器以加载恢复后的 RDB",
			},
			healthCheck("redis-health-check"),
		},
		Compensations: []DatabaseBackupCompensation{
			{
				Name:            "redis-restart-after-restore-preparation-failure",
				TriggerCommands: []string{"redis-backup-existing-rdb"},
				Commands: []DatabaseBackupCommand{
					{
						Name:        "redis-start-after-pre-restore-failure",
						Program:     "docker",
						Args:        []string{"start", request.ContainerName},
						Timeout:     defaultDatabaseBackupTimeout,
						Description: "恢复准备失败时重新启动原 Redis 容器",
					},
					healthCheck("redis-health-check-after-pre-restore-failure"),
				},
			},
			{
				Name:            "redis-rollback-after-restore-failure",
				TriggerCommands: []string{"redis-restore-copy", "redis-start-after-restore", "redis-health-check"},
				Commands: []DatabaseBackupCommand{
					{
						Name:            "redis-rollback-stop",
						Program:         "docker",
						Args:            []string{"stop", request.ContainerName},
						Timeout:         defaultDatabaseBackupTimeout,
						ContinueOnError: true,
						Description:     "恢复失败时停止可能已加载不完整 RDB 的 Redis",
					},
					{
						Name:        "redis-rollback-copy",
						Program:     "docker",
						Args:        []string{"cp", rollbackPath, request.ContainerName + ":" + redisDumpPath},
						Timeout:     defaultDatabaseBackupTimeout,
						Description: "把恢复前保存的旧 RDB 写回 Redis 数据目录",
					},
					{
						Name:        "redis-rollback-start",
						Program:     "docker",
						Args:        []string{"start", request.ContainerName},
						Timeout:     defaultDatabaseBackupTimeout,
						Description: "启动回滚后的 Redis 容器",
					},
					healthCheck("redis-rollback-health-check"),
				},
			},
		},
	}, nil
}

var databaseBackupTypePattern = regexp.MustCompile(`^(postgres|postgresql|mariadb|mysql|redis)$`)
var databaseBackupContainerPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
var databaseBackupNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$.-]{0,127}$`)

func normalizeDatabaseBackupType(value DatabaseBackupType) (DatabaseBackupType, error) {
	normalized := strings.ToLower(strings.TrimSpace(string(value)))
	if !databaseBackupTypePattern.MatchString(normalized) {
		return "", fmt.Errorf("未知数据库类型: %q", value)
	}
	switch normalized {
	case "postgres", "postgresql":
		return DatabaseBackupPostgres, nil
	case "mariadb", "mysql":
		return DatabaseBackupMariaDB, nil
	default:
		return DatabaseBackupRedis, nil
	}
}

func validateDatabaseBackupType(value DatabaseBackupType) error {
	if _, err := normalizeDatabaseBackupType(value); err != nil {
		return fmt.Errorf("未知数据库类型: %q", value)
	}
	return nil
}

func validateDatabaseBackupContainer(value string) error {
	if !databaseBackupContainerPattern.MatchString(strings.TrimSpace(value)) {
		return errors.New("数据库备份容器名无效")
	}
	return nil
}

func validateDatabaseBackupName(value, field string) error {
	if !databaseBackupNamePattern.MatchString(strings.TrimSpace(value)) {
		return fmt.Errorf("数据库%s无效", field)
	}
	return nil
}

// ValidateDatabaseBackupPath validates an artifact path before it reaches an
// injected executor. It is lexical by design; filesystem ownership and
// symlink checks belong to the later storage layer.
func ValidateDatabaseBackupPath(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || !filepath.IsAbs(value) || strings.ContainsAny(value, "\x00\r\n") {
		return errors.New("数据库备份路径必须是绝对路径且不含控制字符")
	}
	clean := filepath.Clean(value)
	if clean != value || clean == string(filepath.Separator) || strings.Contains(value, string(filepath.Separator)+".."+string(filepath.Separator)) || strings.HasSuffix(value, string(filepath.Separator)+"..") {
		return errors.New("数据库备份路径不安全")
	}
	return nil
}

// RedactDatabaseBackupOutput removes configured secrets and common CLI
// password forms before output is exposed to a caller or task log.
func RedactDatabaseBackupOutput(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	for _, marker := range []string{"PGPASSWORD=", "MYSQL_PWD=", "REDISCLI_AUTH=", "password=", "Password="} {
		if index := strings.Index(value, marker); index >= 0 {
			end := strings.IndexAny(value[index+len(marker):], " \t\r\n,;")
			if end < 0 {
				value = value[:index] + marker + "[redacted]"
			} else {
				end += index + len(marker)
				value = value[:index] + marker + "[redacted]" + value[end:]
			}
		}
	}
	if len(value) > 4096 {
		value = value[:4096] + " [truncated]"
	}
	return value
}
