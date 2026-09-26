// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

func cronDataRoot() string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return filepath.Clean(root)
	}
	return absolute
}

func cronBackupRoot() string {
	return filepath.Join(cronDataRoot(), "backups")
}

func cronWebsiteRoot() string {
	root := strings.TrimSpace(os.Getenv("PANEL_WEBSITE_DIR"))
	if root == "" {
		root = "/www/wwwroot"
	}
	return root
}

func cronSafeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "job"
	}
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			builder.WriteRune(char)
		}
	}
	if builder.Len() == 0 {
		return "job"
	}
	return builder.String()
}

func cronExcluded(path, rules string) bool {
	base := filepath.Base(path)
	for _, rule := range strings.FieldsFunc(rules, func(char rune) bool { return char == ',' || char == '\n' }) {
		rule = strings.TrimSpace(rule)
		if rule != "" && (base == rule || strings.Contains(path, rule)) {
			return true
		}
	}
	return false
}

func archiveCronPath(ctx context.Context, source, artifact, rules string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(artifact), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(artifact, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()
	base := filepath.Dir(source)
	if info.IsDir() {
		base = source
	}
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if strings.HasPrefix(path, cronBackupRoot()+string(os.PathSeparator)) || path == cronBackupRoot() {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path != source && cronExcluded(path, rules) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		reader, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, reader)
		reader.Close()
		return copyErr
	})
}

func pruneCronCopies(dir string, retain uint64) error {
	if retain == 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if uint64(len(names)) <= retain {
		return nil
	}
	for _, name := range names[:len(names)-int(retain)] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func cronResourcePaths(job model.Cronjob) ([]string, error) {
	switch job.Type {
	case "website":
		return cronWebsiteDirs(job.Website)
	case "app":
		return cronAppDirs(job)
	case "snapshot":
		source := strings.TrimSpace(job.SourceDir)
		if source == "" {
			source = cronDataRoot()
		}
		return []string{source}, nil
	default:
		return nil, errors.New("不支持的备份资源")
	}
}

func cronWebsiteDirs(spec string) ([]string, error) {
	root := cronWebsiteRoot()
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "all" {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		paths := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				paths = append(paths, filepath.Join(root, entry.Name()))
			}
		}
		return paths, nil
	}
	paths := make([]string, 0)
	for _, name := range strings.Split(spec, ",") {
		name = strings.TrimSpace(name)
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		paths = append(paths, filepath.Join(root, name))
	}
	return paths, nil
}

func cronAppDirs(job model.Cronjob) ([]string, error) {
	root := filepath.Join(cronDataRoot(), "apps")
	ignored := snapshotIgnoredApps(job.SnapshotRule)
	spec := strings.TrimSpace(job.AppID)
	entries := make([]string, 0)
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || !info.IsDir() || path == root {
			return nil
		}
		if strings.Count(strings.TrimPrefix(path, root), string(os.PathSeparator)) != 2 {
			return nil
		}
		base := filepath.Base(path)
		if _, skip := ignored[base]; skip {
			return nil
		}
		if spec != "" && spec != "all" && !strings.Contains(","+spec+",", ","+base+",") && !strings.Contains(spec, base) {
			return nil
		}
		entries = append(entries, path)
		return nil
	})
	return entries, nil
}

func snapshotWantsImage(raw string) bool {
	var rule struct {
		WithImage bool `json:"withImage"`
	}
	_ = json.Unmarshal([]byte(raw), &rule)
	return rule.WithImage
}

func snapshotIgnoredApps(raw string) map[string]struct{} {
	var rule struct {
		IgnoreAppIDs []string `json:"ignoreAppIDs"`
	}
	_ = json.Unmarshal([]byte(raw), &rule)
	ignored := map[string]struct{}{}
	for _, id := range rule.IgnoreAppIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			ignored[id] = struct{}{}
		}
	}
	return ignored
}

func writeSnapshotImageList(ctx context.Context, service *CronjobService, dir string) error {
	result, err := service.cmd.Execute(ctx, model.CommandRequest{Program: "docker", Args: []string{"images", "--format", "{{.Repository}}:{{.Tag}}"}, Timeout: 30 * time.Second})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "images.txt"), []byte(result.Stdout), 0o640)
}
