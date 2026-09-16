// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var databaseBackupService = service.NewDatabaseBackupService(service.NewDockerDatabaseBackupExecutor(), 30*time.Minute)

var (
	databaseBackupOperationLocksMu sync.Mutex
	databaseBackupOperationLocks   = make(map[string]*sync.Mutex)
)

type databaseBackupRecord struct {
	ID            string    `json:"id"`
	DatabaseID    int64     `json:"databaseId"`
	DatabaseName  string    `json:"databaseName"`
	DatabaseType  string    `json:"databaseType"`
	ContainerName string    `json:"containerName"`
	ArtifactPath  string    `json:"artifactPath"`
	ArtifactSize  int64     `json:"artifactSize"`
	SHA256        string    `json:"sha256"`
	Operation     string    `json:"operation"`
	Status        string    `json:"status"`
	Error         string    `json:"error,omitempty"`
	StartedAt     time.Time `json:"startedAt"`
	FinishedAt    time.Time `json:"finishedAt,omitempty"`
}

func registerDatabaseBackupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/databases/backup", handleDatabaseBackup)
	mux.HandleFunc("POST /api/v2/databases/restore", handleDatabaseRestore)
	mux.HandleFunc("GET /api/v2/databases/backups", handleDatabaseBackupList)
}

func handleDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	handleDatabaseBackupOperation(w, r, service.DatabaseBackupOperationBackup)
}

func handleDatabaseRestore(w http.ResponseWriter, r *http.Request) {
	handleDatabaseBackupOperation(w, r, service.DatabaseBackupOperationRestore)
}

func handleDatabaseBackupOperation(w http.ResponseWriter, r *http.Request, operation service.DatabaseBackupOperation) {
	body, err := readDatabaseBody(r)
	if err != nil {
		writeDBError(w, http.StatusBadRequest, err)
		return
	}
	item, err := databaseBackupTarget(r.Context(), body)
	if err != nil {
		writeDBError(w, http.StatusBadRequest, err)
		return
	}
	if item.ContainerName == "" {
		writeDBError(w, http.StatusServiceUnavailable, errors.New("数据库资源未登记容器名，不能执行容器备份"))
		return
	}
	artifact, err := resolveDatabaseBackupArtifact(body, operation)
	if err != nil {
		writeDBError(w, http.StatusBadRequest, err)
		return
	}
	backupType, err := databaseBackupType(item.Type)
	if err != nil {
		writeDBError(w, http.StatusBadRequest, err)
		return
	}
	release, ok := acquireDatabaseBackupOperationLock(item.ContainerName)
	if !ok {
		writeDBError(w, http.StatusConflict, errors.New("目标数据库容器正在执行备份或恢复"))
		return
	}
	defer release()
	request := service.DatabaseBackupRequest{
		Type:          backupType,
		ContainerName: item.ContainerName,
		DatabaseName:  item.InitialDB,
		Username:      item.Username,
		Password:      item.Password,
		ArtifactPath:  artifact,
	}
	if request.DatabaseName == "" {
		request.DatabaseName = item.Name
	}
	started := time.Now().UTC()
	record := databaseBackupRecord{
		ID:            fmt.Sprintf("database-backup-%d", started.UnixNano()),
		DatabaseID:    item.ID,
		DatabaseName:  item.Name,
		DatabaseType:  string(request.Type),
		ContainerName: item.ContainerName,
		ArtifactPath:  artifact,
		Operation:     string(operation),
		Status:        "running",
		StartedAt:     started,
	}
	if err := saveDatabaseBackupRecord(record); err != nil {
		writeDBError(w, http.StatusInternalServerError, err)
		return
	}

	var execution service.DatabaseBackupExecution
	if operation == service.DatabaseBackupOperationBackup {
		execution, err = databaseBackupService.Backup(r.Context(), request)
	} else {
		execution, err = databaseBackupService.Restore(r.Context(), request)
	}
	record.FinishedAt = time.Now().UTC()
	if err != nil {
		record.Status = "failed"
		record.Error = databaseBackupOperationError(operation)
		_ = saveDatabaseBackupRecord(record)
		writeDBError(w, http.StatusServiceUnavailable, errors.New(record.Error))
		return
	}
	record.Status = "completed"
	backupRoot := filepath.Join(databaseBackupRoot(), "databases")
	if err := validateDatabaseBackupArtifactPath(backupRoot, artifact); err != nil {
		record.Status = "failed"
		record.Error = "数据库备份文件校验失败"
		_ = saveDatabaseBackupRecord(record)
		writeDBError(w, http.StatusServiceUnavailable, errors.New(record.Error))
		return
	}
	if info, statErr := os.Lstat(artifact); statErr == nil && info.Mode().IsRegular() {
		record.ArtifactSize = info.Size()
		record.SHA256, _ = databaseBackupSHA256(artifact)
	}
	if err := saveDatabaseBackupRecord(record); err != nil {
		writeDBError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{
		"code": 200,
		"data": map[string]any{
			"record":    record,
			"execution": sanitizeDatabaseBackupExecution(execution, item.Password),
		},
	})
}

func handleDatabaseBackupList(w http.ResponseWriter, r *http.Request) {
	repository, err := SharedRepository()
	if err != nil {
		writeDBError(w, http.StatusInternalServerError, err)
		return
	}
	if err := ensureDatabaseBackupTable(repository); err != nil {
		writeDBError(w, http.StatusInternalServerError, err)
		return
	}
	rows, err := repository.QueryContext(r.Context(), `SELECT id,database_id,database_name,database_type,container_name,artifact_path,artifact_size,sha256,operation,status,error,started_at,finished_at FROM database_backups ORDER BY started_at DESC LIMIT 200`)
	if err != nil {
		writeDBError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	items := make([]databaseBackupRecord, 0)
	for rows.Next() {
		var item databaseBackupRecord
		var started, finished string
		if err := rows.Scan(&item.ID, &item.DatabaseID, &item.DatabaseName, &item.DatabaseType, &item.ContainerName, &item.ArtifactPath, &item.ArtifactSize, &item.SHA256, &item.Operation, &item.Status, &item.Error, &started, &finished); err != nil {
			writeDBError(w, http.StatusInternalServerError, err)
			return
		}
		item.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
		if finished != "" {
			item.FinishedAt, _ = time.Parse(time.RFC3339Nano, finished)
		}
		if item.Error != "" {
			item.Error = databaseBackupOperationError(service.DatabaseBackupOperation(item.Operation))
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeDBError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items)}})
}

func databaseBackupTarget(ctx context.Context, body map[string]any) (service.Database, error) {
	if id := databaseBackupIntField(body, "databaseId", "id"); id > 0 {
		item, ok := databaseService.Find(ctx, int64(id))
		if !ok {
			return service.Database{}, errors.New("数据库资源不存在")
		}
		return item, nil
	}
	name := databaseBackupStringField(body, "database", "databaseName", "name")
	if name == "" {
		return service.Database{}, errors.New("数据库名称不能为空")
	}
	typ := strings.ToLower(databaseBackupStringField(body, "type", "databaseType"))
	if typ == "" {
		typ = "mysql"
	}
	if item, ok := databaseService.FindByName(ctx, typ, name); ok {
		return item, nil
	}
	for _, candidate := range []string{"mysql", "mariadb", "postgresql", "postgres", "redis"} {
		if item, ok := databaseService.FindByName(ctx, candidate, name); ok {
			return item, nil
		}
	}
	return service.Database{}, errors.New("数据库资源不存在")
}

func databaseBackupType(value string) (service.DatabaseBackupType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "postgres", "postgresql", "pg":
		return service.DatabaseBackupPostgres, nil
	case "redis", "redis-cluster":
		return service.DatabaseBackupRedis, nil
	case "mysql", "mariadb":
		return service.DatabaseBackupMariaDB, nil
	default:
		return "", fmt.Errorf("数据库类型 %q 暂不支持备份恢复", value)
	}
}

func databaseBackupStringField(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strField(body, key); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func databaseBackupIntField(body map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if value := intField(body, key); value > 0 {
			return value
		}
	}
	return 0
}

func resolveDatabaseBackupArtifact(body map[string]any, operation service.DatabaseBackupOperation) (string, error) {
	root, err := ensureDatabaseBackupRoot()
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(databaseBackupStringField(body, "artifactPath", "path", "filePath"))
	if value == "" {
		name := strings.TrimSpace(databaseBackupStringField(body, "fileName", "name"))
		if name == "" {
			name = "database-" + time.Now().UTC().Format("20060102T150405.000000000Z")
		}
		value = name
	}
	if filepath.IsAbs(value) {
		return "", errors.New("数据库备份路径必须使用受控备份目录")
	}
	if value != filepath.Base(value) || strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("数据库备份文件名无效")
	}
	if operation == service.DatabaseBackupOperationBackup && filepath.Ext(value) == "" {
		value += ".backup"
	}
	path := filepath.Join(root, value)
	absolute, err := filepath.Abs(path)
	if err != nil || !databaseBackupPathWithin(root, absolute) {
		return "", errors.New("数据库备份路径越界")
	}
	if err := validateDatabaseBackupArtifactPath(root, absolute); err != nil {
		return "", err
	}
	return absolute, nil
}

func databaseBackupRoot() string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	absolute, err := filepath.Abs(filepath.Join(root, "backups"))
	if err != nil {
		return filepath.Clean(filepath.Join(root, "backups"))
	}
	return absolute
}

func databaseBackupPathWithin(root, candidate string) bool {
	root, _ = filepath.Abs(filepath.Clean(root))
	candidate, _ = filepath.Abs(filepath.Clean(candidate))
	return candidate == root || strings.HasPrefix(candidate, root+string(os.PathSeparator))
}

func ensureDatabaseBackupRoot() (string, error) {
	root := filepath.Join(databaseBackupRoot(), "databases")
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", errors.New("数据库备份根目录无效")
	}
	if err := rejectDatabaseBackupSymlinkComponents(absolute); err != nil {
		return "", err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return "", fmt.Errorf("创建数据库备份目录失败: %w", err)
	}
	if err := rejectDatabaseBackupSymlinkComponents(absolute); err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("解析数据库备份根目录失败: %w", err)
	}
	if !databaseBackupPathWithin(absolute, realRoot) || absolute != realRoot {
		return "", errors.New("数据库备份根目录不能是符号链接或越界路径")
	}
	return absolute, nil
}

func validateDatabaseBackupArtifactPath(root, candidate string) error {
	if !databaseBackupPathWithin(root, candidate) {
		return errors.New("数据库备份路径越界")
	}
	if err := rejectDatabaseBackupSymlinkComponents(filepath.Dir(candidate)); err != nil {
		return err
	}
	info, err := os.Lstat(candidate)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("数据库备份路径不能是符号链接")
		}
		if !info.Mode().IsRegular() {
			return errors.New("数据库备份路径必须是普通文件")
		}
	case errors.Is(err, os.ErrNotExist):
		// A new artifact is valid as long as every existing parent is safe.
	default:
		return fmt.Errorf("检查数据库备份路径失败: %w", err)
	}

	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("解析数据库备份根目录失败: %w", err)
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(candidate))
	if err != nil {
		return fmt.Errorf("解析数据库备份文件目录失败: %w", err)
	}
	realCandidate := filepath.Join(realParent, filepath.Base(candidate))
	if info, statErr := os.Lstat(candidate); statErr == nil && info.Mode()&os.ModeSymlink == 0 {
		if resolved, resolveErr := filepath.EvalSymlinks(candidate); resolveErr == nil {
			realCandidate = resolved
		}
	}
	if !databaseBackupPathWithin(realRoot, realCandidate) {
		return errors.New("数据库备份路径解析后越界")
	}
	return nil
}

func rejectDatabaseBackupSymlinkComponents(path string) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return errors.New("数据库备份路径无效")
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
			return fmt.Errorf("检查数据库备份目录失败: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("数据库备份目录不能包含符号链接")
		}
		if !info.IsDir() {
			return errors.New("数据库备份目录路径包含非目录项")
		}
	}
	return nil
}

func ensureDatabaseBackupTable(executor storage.SQLExecutor) error {
	if executor == nil {
		return errors.New("公共数据库未初始化")
	}
	_, err := executor.Exec(`CREATE TABLE IF NOT EXISTS database_backups (
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
	)`)
	return err
}

func saveDatabaseBackupRecord(item databaseBackupRecord) error {
	repository, err := SharedRepository()
	if err != nil {
		return err
	}
	if err := ensureDatabaseBackupTable(repository); err != nil {
		return err
	}
	finishedAt := ""
	if !item.FinishedAt.IsZero() {
		finishedAt = item.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = repository.Exec(`INSERT INTO database_backups(
		id,database_id,database_name,database_type,container_name,artifact_path,
		artifact_size,sha256,operation,status,error,started_at,finished_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(id) DO UPDATE SET
		artifact_size=excluded.artifact_size,
		sha256=excluded.sha256,
		status=excluded.status,
		error=excluded.error,
		finished_at=excluded.finished_at`,
		item.ID,
		item.DatabaseID,
		item.DatabaseName,
		item.DatabaseType,
		item.ContainerName,
		item.ArtifactPath,
		item.ArtifactSize,
		item.SHA256,
		item.Operation,
		item.Status,
		item.Error,
		item.StartedAt.UTC().Format(time.RFC3339Nano),
		finishedAt,
	)
	return err
}

func databaseBackupSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func acquireDatabaseBackupOperationLock(container string) (func(), bool) {
	databaseBackupOperationLocksMu.Lock()
	lock, ok := databaseBackupOperationLocks[container]
	if !ok {
		lock = &sync.Mutex{}
		databaseBackupOperationLocks[container] = lock
	}
	databaseBackupOperationLocksMu.Unlock()
	if !lock.TryLock() {
		return nil, false
	}
	return lock.Unlock, true
}

func databaseBackupOperationError(operation service.DatabaseBackupOperation) string {
	if operation == service.DatabaseBackupOperationRestore {
		return "数据库恢复执行失败"
	}
	return "数据库备份执行失败"
}

func sanitizeDatabaseBackupExecution(execution service.DatabaseBackupExecution, password string) service.DatabaseBackupExecution {
	_ = password
	for index := range execution.Results {
		execution.Results[index].Stdout = ""
		execution.Results[index].Stderr = ""
	}
	for index := range execution.CompensationResults {
		execution.CompensationResults[index].Stdout = ""
		execution.CompensationResults[index].Stderr = ""
	}
	execution.Plan.ArtifactPath = ""
	execution.Plan.RollbackArtifactPath = ""
	for index := range execution.Plan.Commands {
		execution.Plan.Commands[index] = sanitizeDatabaseBackupCommand(execution.Plan.Commands[index])
	}
	for index := range execution.Plan.Compensations {
		for commandIndex := range execution.Plan.Compensations[index].Commands {
			execution.Plan.Compensations[index].Commands[commandIndex] = sanitizeDatabaseBackupCommand(execution.Plan.Compensations[index].Commands[commandIndex])
		}
	}
	return execution
}

func sanitizeDatabaseBackupCommand(command service.DatabaseBackupCommand) service.DatabaseBackupCommand {
	command.Program = ""
	command.Args = nil
	command.Container = ""
	command.Environment = nil
	command.StdinPath = ""
	command.StdoutPath = ""
	command.Description = ""
	return command
}
