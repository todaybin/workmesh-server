// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
)

func tarGzipPaths(sources []string, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".workmesh-tar-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	gw := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gw)
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
			rel, e := filepath.Rel(baseDir, path)
			if e != nil {
				return e
			}
			header, e := tar.FileInfoHeader(info, "")
			if e != nil {
				return e
			}
			header.Name = filepath.ToSlash(rel)
			if e = tw.WriteHeader(header); e != nil {
				return e
			}
			if info.IsDir() {
				return nil
			}
			in, e := os.Open(path)
			if e != nil {
				return e
			}
			_, e = io.Copy(tw, in)
			_ = in.Close()
			return e
		})
		if err != nil {
			break
		}
	}
	if closeErr := tw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := gw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, destination)
}

// handleWebsiteFileRead 以网站 ID 解析根目录，并拒绝读取站点目录以外的任何路径。
