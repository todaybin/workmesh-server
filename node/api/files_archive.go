// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func zipPath(source, destination string) error {
	return zipPaths([]string{source}, destination)
}

func zipPaths(sources []string, destination string) error {
	if len(sources) == 0 || strings.TrimSpace(destination) == "" {
		return errors.New("压缩源和目标不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".workmesh-zip-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	out := tmp
	zw := zip.NewWriter(out)
	for _, source := range sources {
		source, err = cleanFilePath(source)
		if err != nil {
			break
		}
		baseDir := filepath.Dir(source)
		err = filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			rel, e := filepath.Rel(baseDir, path)
			if e != nil {
				return e
			}
			entry, e := zw.Create(filepath.ToSlash(rel))
			if e != nil {
				return e
			}
			in, e := os.Open(path)
			if e != nil {
				return e
			}
			_, copyErr := io.Copy(entry, in)
			_ = in.Close()
			return copyErr
		})
		if err != nil {
			break
		}
	}
	if closeErr := zw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, destination)
}

func unzipPath(source, destination string) error {
	items, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer items.Close()
	root := filepath.Clean(destination)
	if err := os.MkdirAll(root, 0o750); err != nil {
		return err
	}
	for _, item := range items.File {
		if item.FileInfo().Mode()&os.ModeSymlink != 0 {
			return errors.New("压缩包不允许包含符号链接")
		}
		target := filepath.Join(root, filepath.FromSlash(item.Name))
		if !strings.HasPrefix(target, root+string(os.PathSeparator)) && target != root {
			return errors.New("压缩包包含非法路径")
		}
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		in, err := item.Open()
		if err != nil {
			return err
		}
		// 每个条目先写同目录临时文件，完成后原子替换目标，避免中断留下半文件。
		tmp, err := os.CreateTemp(filepath.Dir(target), ".workmesh-unzip-*")
		if err == nil {
			_, err = io.Copy(tmp, in)
			if closeErr := tmp.Close(); err == nil {
				err = closeErr
			}
			if err == nil {
				err = os.Rename(tmp.Name(), target)
			} else {
				_ = os.Remove(tmp.Name())
			}
		}
		_ = in.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untarGzipPath(source, destination string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		header, e := tr.Next()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return e
		}
		if header.Name == "" || strings.Contains(header.Name, "..") {
			return errors.New("压缩包包含非法路径")
		}
		target := filepath.Join(destination, filepath.FromSlash(header.Name))
		root := filepath.Clean(destination)
		if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return errors.New("压缩包包含非法路径")
		}
		if header.FileInfo().IsDir() {
			if e = os.MkdirAll(target, 0755); e != nil {
				return e
			}
			continue
		}
		if e = os.MkdirAll(filepath.Dir(target), 0755); e != nil {
			return e
		}
		out, e := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if e != nil {
			return e
		}
		_, e = io.Copy(out, io.LimitReader(tr, 512<<20))
		_ = out.Close()
		if e != nil {
			return e
		}
	}
	return nil
}
