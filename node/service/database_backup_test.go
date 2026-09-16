// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type databaseBackupRecordingExecutor struct {
	commands    []DatabaseBackupCommand
	hasDeadline bool
}

func (e *databaseBackupRecordingExecutor) Execute(ctx context.Context, command DatabaseBackupCommand) (DatabaseBackupCommandResult, error) {
	e.commands = append(e.commands, command)
	_, e.hasDeadline = ctx.Deadline()
	return DatabaseBackupCommandResult{ExitCode: 0, Stdout: "password=super-secret"}, nil
}

type databaseBackupSequenceExecutor struct {
	commands []DatabaseBackupCommand
	results  map[string][]DatabaseBackupCommandResult
	errors   map[string][]error
}

func (e *databaseBackupSequenceExecutor) Execute(_ context.Context, command DatabaseBackupCommand) (DatabaseBackupCommandResult, error) {
	e.commands = append(e.commands, command)
	if queued := e.results[command.Name]; len(queued) > 0 {
		result := queued[0]
		e.results[command.Name] = queued[1:]
		return result, nil
	}
	if queued := e.errors[command.Name]; len(queued) > 0 {
		err := queued[0]
		e.errors[command.Name] = queued[1:]
		return DatabaseBackupCommandResult{}, err
	}
	return DatabaseBackupCommandResult{ExitCode: 0, Stdout: "PONG"}, nil
}

func assertDatabaseBackupPlanArgsDoNotContain(t *testing.T, plan DatabaseBackupPlan, password string) {
	t.Helper()
	assertCommandsArgsDoNotContain(t, plan.Commands, password)
	for _, compensation := range plan.Compensations {
		assertCommandsArgsDoNotContain(t, compensation.Commands, password)
	}
}

func assertCommandsArgsDoNotContain(t *testing.T, commands []DatabaseBackupCommand, password string) {
	t.Helper()
	for _, command := range commands {
		if strings.Contains(strings.Join(command.Args, "\x00"), password) {
			t.Fatalf("password leaked into %s args: %#v", command.Name, command.Args)
		}
	}
}

func TestDatabaseBackupPlanPostgresDoesNotPutPasswordInArgs(t *testing.T) {
	service := NewDatabaseBackupService(nil, 0)
	plan, err := service.BuildBackupPlan(DatabaseBackupRequest{
		Type:          DatabaseBackupPostgres,
		ContainerName: "workmesh-panel-postgres",
		DatabaseName:  "appdb",
		Username:      "postgres",
		Password:      "super-secret",
		ArtifactPath:  "/var/lib/workmesh/backups/app.dump",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 1 || plan.Commands[0].Program != "pg_dump" {
		t.Fatalf("unexpected PostgreSQL plan: %+v", plan)
	}
	command := plan.Commands[0]
	if strings.Contains(strings.Join(command.Args, "\x00"), "super-secret") {
		t.Fatalf("password leaked into args: %#v", command.Args)
	}
	if command.Environment["PGPASSWORD"] != "super-secret" {
		t.Fatalf("password was not isolated in environment: %#v", command.Environment)
	}
	if command.StdoutPath != "/var/lib/workmesh/backups/app.dump" {
		t.Fatalf("stdout artifact path=%q", command.StdoutPath)
	}
	if command.StreamMode != DatabaseBackupStreamStdoutToArtifact {
		t.Fatalf("PostgreSQL backup stream mode=%q", command.StreamMode)
	}
	if command.Timeout <= 0 {
		t.Fatalf("PostgreSQL backup timeout=%s", command.Timeout)
	}
}

func TestDatabaseBackupPlanCoversMariaDBAndRedisCommands(t *testing.T) {
	service := NewDatabaseBackupService(nil, 0)
	maria, err := service.BuildRestorePlan(DatabaseBackupRequest{
		Type:          DatabaseBackupMariaDB,
		ContainerName: "workmesh-panel-mariadb",
		DatabaseName:  "appdb",
		Username:      "root",
		Password:      "maria-secret",
		ArtifactPath:  "/var/lib/workmesh/backups/app.sql",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(maria.Commands) != 1 || maria.Commands[0].Program != "mariadb" {
		t.Fatalf("unexpected MariaDB restore plan: %+v", maria)
	}
	if strings.Contains(strings.Join(maria.Commands[0].Args, "\x00"), "maria-secret") {
		t.Fatalf("MariaDB password leaked into args: %#v", maria.Commands[0].Args)
	}
	if maria.Commands[0].StdinPath != "/var/lib/workmesh/backups/app.sql" {
		t.Fatalf("MariaDB stdin path=%q", maria.Commands[0].StdinPath)
	}
	if maria.Commands[0].StreamMode != DatabaseBackupStreamArtifactToStdin {
		t.Fatalf("MariaDB restore stream mode=%q", maria.Commands[0].StreamMode)
	}
	if maria.Commands[0].Timeout <= 0 {
		t.Fatalf("MariaDB restore timeout=%s", maria.Commands[0].Timeout)
	}

	redis, err := service.BuildBackupPlan(DatabaseBackupRequest{
		Type:          DatabaseBackupRedis,
		ContainerName: "workmesh-panel-redis",
		Password:      "redis-secret",
		ArtifactPath:  "/var/lib/workmesh/backups/dump.rdb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(redis.Commands) != 3 || redis.Commands[0].Program != "redis-cli" || redis.Commands[1].Program != "redis-cli" || redis.Commands[2].Program != "docker" {
		t.Fatalf("unexpected Redis backup plan: %+v", redis)
	}
	assertDatabaseBackupPlanArgsDoNotContain(t, redis, "redis-secret")
	if !strings.Contains(strings.Join(redis.Commands[0].Args, " "), "BGSAVE") {
		t.Fatalf("Redis plan does not trigger BGSAVE: %+v", redis.Commands[0].Args)
	}
	wait := redis.Commands[1]
	if !strings.Contains(strings.Join(wait.Args, " "), "INFO persistence") {
		t.Fatalf("Redis plan does not inspect persistence: %+v", wait.Args)
	}
	if wait.Retry.MaxAttempts < 2 || wait.Retry.Delay <= 0 {
		t.Fatalf("Redis BGSAVE wait has no retry policy: %+v", wait.Retry)
	}
	if wait.Verification.Kind != "redis_bgsave_complete" ||
		len(wait.Verification.RequiredOutput) != 2 ||
		wait.Verification.RequiredOutput[0] != "rdb_bgsave_in_progress:0" ||
		wait.Verification.RequiredOutput[1] != "rdb_last_bgsave_status:ok" {
		t.Fatalf("Redis BGSAVE completion verification=%+v", wait.Verification)
	}
	if !strings.Contains(strings.Join(redis.Commands[2].Args, " "), "dump.rdb") {
		t.Fatalf("Redis plan does not copy RDB after verification: %+v", redis.Commands[2].Args)
	}
}

func TestDatabaseBackupPlansDeclareSQLStreamDirectionAndTimeout(t *testing.T) {
	service := NewDatabaseBackupService(nil, 0)
	tests := []struct {
		name       string
		request    DatabaseBackupRequest
		operation  DatabaseBackupOperation
		streamMode DatabaseBackupStreamMode
		stdinPath  bool
		stdoutPath bool
	}{
		{
			name: "postgres backup",
			request: DatabaseBackupRequest{
				Type:          DatabaseBackupPostgres,
				ContainerName: "workmesh-panel-postgres",
				DatabaseName:  "appdb",
				Username:      "postgres",
				Password:      "postgres-secret",
				ArtifactPath:  "/var/lib/workmesh/backups/app.dump",
			},
			operation:  DatabaseBackupOperationBackup,
			streamMode: DatabaseBackupStreamStdoutToArtifact,
			stdoutPath: true,
		},
		{
			name: "postgres restore",
			request: DatabaseBackupRequest{
				Type:          DatabaseBackupPostgres,
				ContainerName: "workmesh-panel-postgres",
				DatabaseName:  "appdb",
				Username:      "postgres",
				Password:      "postgres-secret",
				ArtifactPath:  "/var/lib/workmesh/backups/app.dump",
			},
			operation:  DatabaseBackupOperationRestore,
			streamMode: DatabaseBackupStreamArtifactToStdin,
			stdinPath:  true,
		},
		{
			name: "mariadb backup",
			request: DatabaseBackupRequest{
				Type:          DatabaseBackupMariaDB,
				ContainerName: "workmesh-panel-mariadb",
				DatabaseName:  "appdb",
				Username:      "root",
				Password:      "maria-secret",
				ArtifactPath:  "/var/lib/workmesh/backups/app.sql",
			},
			operation:  DatabaseBackupOperationBackup,
			streamMode: DatabaseBackupStreamStdoutToArtifact,
			stdoutPath: true,
		},
		{
			name: "mariadb restore",
			request: DatabaseBackupRequest{
				Type:          DatabaseBackupMariaDB,
				ContainerName: "workmesh-panel-mariadb",
				DatabaseName:  "appdb",
				Username:      "root",
				Password:      "maria-secret",
				ArtifactPath:  "/var/lib/workmesh/backups/app.sql",
			},
			operation:  DatabaseBackupOperationRestore,
			streamMode: DatabaseBackupStreamArtifactToStdin,
			stdinPath:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var plan DatabaseBackupPlan
			var err error
			if test.operation == DatabaseBackupOperationBackup {
				plan, err = service.BuildBackupPlan(test.request)
			} else {
				plan, err = service.BuildRestorePlan(test.request)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Commands) != 1 {
				t.Fatalf("commands=%+v", plan.Commands)
			}
			command := plan.Commands[0]
			if command.StreamMode != test.streamMode {
				t.Fatalf("stream mode=%q want %q", command.StreamMode, test.streamMode)
			}
			if command.Timeout <= 0 {
				t.Fatalf("command timeout=%s", command.Timeout)
			}
			if (command.StdinPath != "") != test.stdinPath || (command.StdoutPath != "") != test.stdoutPath {
				t.Fatalf("stdin=%q stdout=%q", command.StdinPath, command.StdoutPath)
			}
			assertDatabaseBackupPlanArgsDoNotContain(t, plan, test.request.Password)
		})
	}
}

func TestDatabaseRedisRestorePlanIncludesRollbackAndHealthValidation(t *testing.T) {
	service := NewDatabaseBackupService(nil, 0)
	plan, err := service.BuildRestorePlan(DatabaseBackupRequest{
		Type:          DatabaseBackupRedis,
		ContainerName: "workmesh-panel-redis",
		Password:      "redis-secret",
		ArtifactPath:  "/var/lib/workmesh/backups/dump.rdb",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RollbackArtifactPath != "/var/lib/workmesh/backups/dump.rdb.pre-restore" {
		t.Fatalf("rollback artifact path=%q", plan.RollbackArtifactPath)
	}
	wantNames := []string{
		"redis-stop-for-restore",
		"redis-backup-existing-rdb",
		"redis-restore-copy",
		"redis-start-after-restore",
		"redis-health-check",
	}
	if len(plan.Commands) != len(wantNames) {
		t.Fatalf("restore command count=%d commands=%+v", len(plan.Commands), plan.Commands)
	}
	for index, want := range wantNames {
		if plan.Commands[index].Name != want {
			t.Fatalf("command[%d]=%q want %q", index, plan.Commands[index].Name, want)
		}
	}
	if !strings.Contains(strings.Join(plan.Commands[1].Args, " "), plan.RollbackArtifactPath) {
		t.Fatalf("old RDB is not backed up: %+v", plan.Commands[1].Args)
	}
	health := plan.Commands[4]
	if health.Verification.Kind != "redis_health" ||
		len(health.Verification.RequiredOutput) != 1 ||
		health.Verification.RequiredOutput[0] != "PONG" ||
		health.Retry.MaxAttempts < 2 {
		t.Fatalf("Redis health validation=%+v", health)
	}
	if len(plan.Compensations) != 2 {
		t.Fatalf("compensation count=%d plan=%+v", len(plan.Compensations), plan.Compensations)
	}
	if !containsDatabaseBackupCommand(plan.Compensations[1].TriggerCommands, "redis-health-check") {
		t.Fatalf("health failure does not trigger rollback: %+v", plan.Compensations[1].TriggerCommands)
	}
	rollbackNames := make([]string, 0, len(plan.Compensations[1].Commands))
	for _, command := range plan.Compensations[1].Commands {
		rollbackNames = append(rollbackNames, command.Name)
	}
	if strings.Join(rollbackNames, ",") != "redis-rollback-stop,redis-rollback-copy,redis-rollback-start,redis-rollback-health-check" {
		t.Fatalf("unexpected rollback order=%v", rollbackNames)
	}
	assertDatabaseBackupPlanArgsDoNotContain(t, plan, "redis-secret")
}

func TestDatabaseBackupWaitVerificationRetriesBeforeSuccess(t *testing.T) {
	executor := &databaseBackupSequenceExecutor{
		results: map[string][]DatabaseBackupCommandResult{
			"redis-wait-bgsave": {
				{ExitCode: 0, Stdout: "rdb_bgsave_in_progress:1\nrdb_last_bgsave_status:ok\n"},
				{ExitCode: 0, Stdout: "rdb_bgsave_in_progress:0\nrdb_last_bgsave_status:ok\n"},
			},
		},
	}
	service := NewDatabaseBackupService(executor, time.Second)
	plan, err := service.BuildBackupPlan(DatabaseBackupRequest{
		Type:          DatabaseBackupRedis,
		ContainerName: "workmesh-panel-redis",
		Password:      "redis-secret",
		ArtifactPath:  "/var/lib/workmesh/backups/dump.rdb",
	})
	if err != nil {
		t.Fatal(err)
	}
	wait := plan.Commands[1]
	wait.Retry.Delay = 0
	_, err = service.executeCommand(context.Background(), wait, "redis-secret")
	if err != nil {
		t.Fatalf("BGSAVE wait verification failed: %v", err)
	}
	if len(executor.commands) != 2 {
		t.Fatalf("wait attempts=%d commands=%+v", len(executor.commands), executor.commands)
	}
	assertCommandsArgsDoNotContain(t, executor.commands, "redis-secret")
}

func TestDatabaseRedisRestoreFailureRunsRollbackInOrder(t *testing.T) {
	executor := &databaseBackupSequenceExecutor{
		results: map[string][]DatabaseBackupCommandResult{
			"redis-health-check": {{ExitCode: 1, Stderr: "restore failed"}},
		},
	}
	service := NewDatabaseBackupService(executor, 10*time.Millisecond)
	execution, err := service.Restore(context.Background(), DatabaseBackupRequest{
		Type:          DatabaseBackupRedis,
		ContainerName: "workmesh-panel-redis",
		Password:      "redis-secret",
		ArtifactPath:  "/var/lib/workmesh/backups/dump.rdb",
	})
	if err == nil || !strings.Contains(err.Error(), "redis-health-check") {
		t.Fatalf("restore error=%v", err)
	}
	if len(execution.CompensationResults) != 4 {
		t.Fatalf("compensation results=%d results=%+v", len(execution.CompensationResults), execution.CompensationResults)
	}
	got := make([]string, 0, len(executor.commands))
	for _, command := range executor.commands {
		got = append(got, command.Name)
	}
	want := "redis-stop-for-restore,redis-backup-existing-rdb,redis-restore-copy,redis-start-after-restore,redis-health-check,redis-rollback-stop,redis-rollback-copy,redis-rollback-start,redis-rollback-health-check"
	if strings.Join(got, ",") != want {
		t.Fatalf("execution order=%v want %s", got, want)
	}
	assertCommandsArgsDoNotContain(t, executor.commands, "redis-secret")
}

type databaseBackupBlockingExecutor struct{}

func (databaseBackupBlockingExecutor) Execute(ctx context.Context, _ DatabaseBackupCommand) (DatabaseBackupCommandResult, error) {
	<-ctx.Done()
	return DatabaseBackupCommandResult{}, ctx.Err()
}

func TestDatabaseBackupCommandTimeoutIsAnOverallAttemptBudget(t *testing.T) {
	service := NewDatabaseBackupService(databaseBackupBlockingExecutor{}, time.Second)
	_, err := service.executeCommand(context.Background(), DatabaseBackupCommand{
		Name:    "timeout-test",
		Program: "docker",
		Timeout: 5 * time.Millisecond,
	}, "")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error=%v", err)
	}
}

func TestDatabaseBackupRejectsUnknownTypeAndUnsafePath(t *testing.T) {
	service := NewDatabaseBackupService(nil, 0)
	base := DatabaseBackupRequest{
		Type:          DatabaseBackupPostgres,
		ContainerName: "workmesh-panel-postgres",
		DatabaseName:  "appdb",
		Username:      "postgres",
		ArtifactPath:  "/var/lib/workmesh/backups/app.dump",
	}
	base.Type = DatabaseBackupType("oracle")
	if _, err := service.BuildBackupPlan(base); err == nil || !strings.Contains(err.Error(), "未知数据库类型") {
		t.Fatalf("unknown type error=%v", err)
	}
	for _, path := range []string{"relative.dump", "/tmp/../etc/passwd", "/tmp/backup/../dump", "/"} {
		if err := ValidateDatabaseBackupPath(path); err == nil {
			t.Fatalf("unsafe path accepted: %q", path)
		}
	}
}

func TestDatabaseBackupExecutionUsesTimeoutAndRedactsOutput(t *testing.T) {
	executor := &databaseBackupRecordingExecutor{}
	service := NewDatabaseBackupService(executor, 50*time.Millisecond)
	execution, err := service.Backup(context.Background(), DatabaseBackupRequest{
		Type:          DatabaseBackupMariaDB,
		ContainerName: "workmesh-panel-mariadb",
		DatabaseName:  "appdb",
		Username:      "root",
		Password:      "super-secret",
		ArtifactPath:  "/var/lib/workmesh/backups/app.sql",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(executor.commands) != 1 || len(execution.Results) != 1 {
		t.Fatalf("execution commands=%+v results=%+v", executor.commands, execution.Results)
	}
	if !executor.hasDeadline {
		t.Fatal("executor context has no deadline")
	}
	if strings.Contains(execution.Results[0].Stdout, "super-secret") || strings.Contains(execution.Results[0].Stdout, "password=super-secret") {
		t.Fatalf("secret was not redacted: %q", execution.Results[0].Stdout)
	}
	if execution.Results[0].Stdout != "password=[redacted]" {
		t.Fatalf("unexpected redacted output: %q", execution.Results[0].Stdout)
	}
}

func TestDatabaseBackupRequiresInjectedExecutorForExecution(t *testing.T) {
	service := NewDatabaseBackupService(nil, time.Second)
	_, err := service.Restore(context.Background(), DatabaseBackupRequest{
		Type:          DatabaseBackupRedis,
		ContainerName: "workmesh-panel-redis",
		ArtifactPath:  "/var/lib/workmesh/backups/dump.rdb",
	})
	if err == nil || !strings.Contains(err.Error(), "执行器未注入") {
		t.Fatalf("missing executor error=%v", err)
	}
}
