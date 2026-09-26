// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

// cronDatabaseTarget 是计划任务要备份的一个库及其所属实例连接信息。
type cronDatabaseTarget struct {
	ID        int64
	Name      string
	Type      string
	Instance  string
	Container string
	Username  string
	Password  string
}

func (s *CronjobService) executeDatabaseCron(ctx context.Context, job model.Cronjob) (model.CommandResult, error) {
	targets, err := cronDatabaseTargets(ctx, job)
	if err != nil {
		return model.CommandResult{}, err
	}
	if len(targets) == 0 {
		return model.CommandResult{}, errors.New("没有可备份的数据库")
	}
	lines := make([]string, 0, len(targets))
	for _, target := range targets {
		artifact := filepath.Join(cronBackupRoot(), "databases", target.Type, target.Instance, target.Name, time.Now().UTC().Format("20060102T150405")+".dump")
		backup := s.databaseBackup
		if backup == nil {
			backup = defaultCronDatabaseBackup
		}
		if err := backup(ctx, target, artifact); err != nil {
			return model.CommandResult{ExitCode: 1, Stderr: err.Error()}, err
		}
		if err := pruneCronCopies(filepath.Dir(artifact), job.RetainCopies); err != nil {
			return model.CommandResult{}, err
		}
		lines = append(lines, target.Instance+"/"+target.Name+" "+artifact)
	}
	return model.CommandResult{ExitCode: 0, Stdout: strings.Join(lines, "\n")}, nil
}

func cronDatabaseTargets(ctx context.Context, job model.Cronjob) ([]cronDatabaseTarget, error) {
	svc := NewDatabaseService(nil)
	rows := make([]Database, 0)
	for _, typ := range cronDatabaseTypes(job.DBType) {
		rows = append(rows, svc.Search(ctx, typ, "")...)
	}
	selected := map[string]struct{}{}
	raw := strings.TrimSpace(job.DBName)
	all := raw == "" || raw == "all"
	if !all {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				selected[part] = struct{}{}
			}
		}
	}
	byName := map[string]Database{}
	for _, row := range rows {
		byName[strings.ToLower(row.Name)] = row
	}
	targets := make([]cronDatabaseTarget, 0)
	for _, row := range rows {
		if cronDatabaseRowIsInstance(row, rows) {
			continue
		}
		if !all {
			if _, ok := selected[strconv.FormatInt(row.ID, 10)]; !ok {
				if _, ok := selected[row.Name]; !ok {
					continue
				}
			}
		}
		parent := row
		if found, ok := byName[strings.ToLower(row.InitialDB)]; ok {
			parent = found
		}
		if full, ok := svc.Find(ctx, parent.ID); ok {
			parent = full
		}
		username := parent.Username
		if username == "" {
			username = row.Username
		}
		password := parent.Password
		if password == "" {
			password = row.Password
		}
		container := parent.ContainerName
		if container == "" {
			container = row.ContainerName
		}
		targets = append(targets, cronDatabaseTarget{
			ID: row.ID, Name: row.Name, Type: row.Type, Instance: parent.Name,
			Container: container, Username: username, Password: password,
		})
	}
	return targets, nil
}

func cronDatabaseTypes(dbType string) []string {
	switch strings.ToLower(strings.TrimSpace(dbType)) {
	case "mysql", "mariadb", "mysql-cluster":
		return []string{"mysql", "mariadb", "mysql-cluster"}
	case "mongodb", "mongo":
		return []string{"mongodb", "mongo"}
	default:
		return []string{"postgresql", "postgres", "postgresql-cluster"}
	}
}

func cronDatabaseRowIsInstance(row Database, rows []Database) bool {
	parent := strings.ToLower(strings.TrimSpace(row.InitialDB))
	if parent == "" || parent == strings.ToLower(row.Name) {
		return true
	}
	for _, candidate := range rows {
		if candidate.ID != row.ID && strings.EqualFold(candidate.Name, parent) {
			return false
		}
	}
	return true
}

func defaultCronDatabaseBackup(ctx context.Context, target cronDatabaseTarget, artifact string) error {
	if strings.TrimSpace(target.Container) == "" {
		return errors.New("数据库未登记容器，不能备份")
	}
	kind := DatabaseBackupPostgres
	switch {
	case strings.Contains(target.Type, "mysql") || target.Type == "mariadb":
		kind = DatabaseBackupMariaDB
	case strings.Contains(target.Type, "mongo"):
		return errors.New("MongoDB 备份尚未接入容器导出")
	}
	if target.Username == "" {
		target.Username = "postgres"
	}
	if err := os.MkdirAll(filepath.Dir(artifact), 0o750); err != nil {
		return err
	}
	_, err := NewDatabaseBackupService(NewDockerDatabaseBackupExecutor(), 0).Backup(ctx, DatabaseBackupRequest{
		Type: kind, ContainerName: target.Container, DatabaseName: target.Name,
		Username: target.Username, Password: target.Password, ArtifactPath: artifact,
	})
	return err
}

func (s *CronjobService) executeArchiveJob(ctx context.Context, job model.Cronjob) (model.CommandResult, error) {
	source := strings.TrimSpace(job.SourceDir)
	if source == "" {
		return model.CommandResult{}, errors.New("备份路径不能为空")
	}
	artifact := filepath.Join(cronBackupRoot(), job.Type, cronSafeName(job.Name), time.Now().UTC().Format("20060102T150405")+".tar.gz")
	if err := archiveCronPath(ctx, source, artifact, job.ExclusionRules); err != nil {
		return model.CommandResult{}, err
	}
	if err := pruneCronCopies(filepath.Dir(artifact), job.RetainCopies); err != nil {
		return model.CommandResult{}, err
	}
	return model.CommandResult{ExitCode: 0, Stdout: artifact}, nil
}

func (s *CronjobService) executeResourceArchive(ctx context.Context, job model.Cronjob) (model.CommandResult, error) {
	paths, err := cronResourcePaths(job)
	if err != nil {
		return model.CommandResult{}, err
	}
	if len(paths) == 0 {
		return model.CommandResult{}, errors.New("没有可备份的资源")
	}
	lines := make([]string, 0, len(paths))
	for _, source := range paths {
		artifact := filepath.Join(cronBackupRoot(), job.Type, cronSafeName(filepath.Base(source)), time.Now().UTC().Format("20060102T150405")+".tar.gz")
		if err := archiveCronPath(ctx, source, artifact, job.ExclusionRules); err != nil {
			return model.CommandResult{}, err
		}
		if job.Type == "snapshot" && snapshotWantsImage(job.SnapshotRule) {
			if err := writeSnapshotImageList(ctx, s, filepath.Dir(artifact)); err != nil {
				return model.CommandResult{}, err
			}
		}
		if err := pruneCronCopies(filepath.Dir(artifact), job.RetainCopies); err != nil {
			return model.CommandResult{}, err
		}
		lines = append(lines, artifact)
	}
	return model.CommandResult{ExitCode: 0, Stdout: strings.Join(lines, "\n")}, nil
}

func (s *CronjobService) executeCutWebsiteLog(ctx context.Context, job model.Cronjob) (model.CommandResult, error) {
	sites, err := cronWebsiteDirs(job.Website)
	if err != nil {
		return model.CommandResult{}, err
	}
	lines := make([]string, 0, len(sites))
	for _, site := range sites {
		logDir := filepath.Join(site, "log")
		if _, err := os.Stat(filepath.Join(logDir, "access.log")); err != nil {
			continue
		}
		artifact := filepath.Join(cronBackupRoot(), "cut-website-log", cronSafeName(filepath.Base(site)), time.Now().UTC().Format("20060102T150405")+".tar.gz")
		if err := archiveCronPath(ctx, logDir, artifact, ""); err != nil {
			return model.CommandResult{}, err
		}
		for _, name := range []string{"access.log", "error.log"} {
			_ = os.WriteFile(filepath.Join(logDir, name), nil, 0o640)
		}
		lines = append(lines, artifact)
	}
	if len(lines) == 0 {
		return model.CommandResult{}, errors.New("没有可轮转的网站日志")
	}
	return model.CommandResult{ExitCode: 0, Stdout: strings.Join(lines, "\n")}, nil
}

func (s *CronjobService) executeNTP(ctx context.Context) (model.CommandResult, error) {
	result, err := s.cmd.Execute(ctx, model.CommandRequest{Program: "timedatectl", Args: []string{"set-ntp", "true"}, Timeout: 15 * time.Second})
	if err != nil {
		return result, fmt.Errorf("同步 NTP 失败: %w", err)
	}
	if result.ExitCode != 0 {
		return result, fmt.Errorf("同步 NTP 失败: %s", strings.TrimSpace(result.Stderr))
	}
	return result, nil
}

func (s *CronjobService) executeSyncIPGroup(ctx context.Context, job model.Cronjob) (model.CommandResult, error) {
	dir := strings.TrimSpace(job.SourceDir)
	if dir == "" {
		dir = filepath.Join(cronDataRoot(), "apps", "openresty", "openresty", "1pwaf", "data", "rules", "ip_group")
	}
	urlDir := filepath.Join(dir, "ip_group_url")
	entries, err := os.ReadDir(urlDir)
	if err != nil {
		return model.CommandResult{}, errors.New("WAF IP 组目录不存在")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	synced := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_url") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(urlDir, entry.Name()))
		if err != nil {
			return model.CommandResult{}, err
		}
		remote := strings.TrimSpace(string(raw))
		if remote == "" {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, remote, nil)
		if err != nil {
			return model.CommandResult{}, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return model.CommandResult{}, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if readErr != nil {
			return model.CommandResult{}, readErr
		}
		if resp.StatusCode >= 400 {
			return model.CommandResult{}, fmt.Errorf("同步 IP 组失败: HTTP %d", resp.StatusCode)
		}
		name := strings.TrimSuffix(entry.Name(), "_url")
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o640); err != nil {
			return model.CommandResult{}, err
		}
		synced = append(synced, name)
	}
	if len(synced) == 0 {
		return model.CommandResult{}, errors.New("没有可同步的 IP 组")
	}
	return model.CommandResult{ExitCode: 0, Stdout: strings.Join(synced, "\n")}, nil
}
