// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runtimePackageVersion(item runtimeRecord) (appVersionRecord, bool) {
	store := getAppStore()
	store.mu.RLock()
	defer store.mu.RUnlock()
	for _, app := range store.state.Catalog {
		if !appMatchesType(app, normalizeRuntimeTypeFilter(item.Type)) {
			continue
		}
		for _, version := range app.Versions {
			if strings.EqualFold(strings.TrimSpace(version.ID), strings.TrimSpace(item.AppDetailID)) {
				return version, true
			}
		}
	}
	return appVersionRecord{}, false
}

// refreshRuntimePackageIfNeeded 仅在应用目录版本变更时同步包内运行脚本。
// 返回的回滚函数用于在后续容器重建失败时恢复原脚本。
func refreshRuntimePackageIfNeeded(current runtimeRecord, updated *runtimeRecord) (func(), error) {
	noop := func() {}
	if updated == nil || !strings.EqualFold(updated.Resource, "appstore") || normalizeRuntimeTypeFilter(updated.Type) == "php" {
		return noop, nil
	}
	version, ok := runtimePackageVersion(*updated)
	if !ok || version.LastModified <= 0 || version.LastModified == current.PackageModified {
		return noop, nil
	}
	if strings.TrimSpace(version.DownloadURL) == "" {
		return noop, errors.New("运行时应用包已更新，但归档地址为空")
	}
	if strings.TrimSpace(updated.InstallPath) == "" {
		return noop, errors.New("运行时尚未完成安装，无法更新应用包")
	}
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	downloadDir := filepath.Join(root, "runtimes", ".downloads")
	if err := os.MkdirAll(downloadDir, 0o750); err != nil {
		return noop, fmt.Errorf("创建运行时下载目录失败: %w", err)
	}
	archivePath := filepath.Join(downloadDir, updated.ID+"-update-"+idToken()+".tar.gz")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	err := downloadAppArchive(ctx, version.DownloadURL, archivePath)
	cancel()
	if err != nil {
		return noop, fmt.Errorf("下载运行时更新包失败: %w", err)
	}
	defer os.Remove(archivePath)
	rollback, err := installRuntimeRunScriptFromArchive(archivePath, updated.InstallPath, updated.Type, updated.Version)
	if err != nil {
		return noop, fmt.Errorf("更新运行时脚本失败: %w", err)
	}
	updated.DownloadURL = version.DownloadURL
	updated.PackageModified = version.LastModified
	return rollback, nil
}

func installRuntimeRunScriptFromArchive(archivePath, installDir, runtimeType, version string) (func(), error) {
	stage, err := os.MkdirTemp(filepath.Dir(installDir), ".runtime-script-stage-")
	if err != nil {
		return func() {}, err
	}
	defer os.RemoveAll(stage)
	if err := extractTarGz(archivePath, stage); err != nil {
		return func() {}, err
	}
	source := filepath.Join(stage, strings.ToLower(runtimeType), version, "run.sh")
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return func() {}, errors.New("运行时更新包缺少普通文件 run.sh")
	}
	if info.Size() > 2<<20 {
		return func() {}, errors.New("运行时 run.sh 超过 2 MiB 限制")
	}
	input, err := os.Open(source)
	if err != nil {
		return func() {}, err
	}
	content, err := io.ReadAll(io.LimitReader(input, (2<<20)+1))
	closeErr := input.Close()
	if err != nil {
		return func() {}, err
	}
	if closeErr != nil {
		return func() {}, closeErr
	}
	target := filepath.Join(installDir, "run.sh")
	oldInfo, oldErr := os.Lstat(target)
	hadOld := oldErr == nil
	if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
		return func() {}, oldErr
	}
	if hadOld && (!oldInfo.Mode().IsRegular() || oldInfo.Mode()&os.ModeSymlink != 0) {
		return func() {}, errors.New("现有运行时 run.sh 不是普通文件")
	}
	backup := target + ".bak"
	if _, err := os.Stat(backup); err == nil {
		backup += "." + time.Now().UTC().Format("20060102T150405.000000000Z")
	} else if !errors.Is(err, os.ErrNotExist) {
		return func() {}, err
	}
	if hadOld {
		if err := os.Rename(target, backup); err != nil {
			return func() {}, err
		}
	}
	temporary := target + ".tmp-" + idToken()
	if err := os.WriteFile(temporary, content, 0o750); err != nil {
		if hadOld {
			_ = os.Rename(backup, target)
		}
		return func() {}, err
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		if hadOld {
			_ = os.Rename(backup, target)
		}
		return func() {}, err
	}
	rollback := func() {
		if hadOld {
			_ = os.Remove(target)
			_ = os.Rename(backup, target)
		} else {
			_ = os.Remove(target)
		}
	}
	return rollback, nil
}
