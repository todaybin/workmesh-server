// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

type fileAsyncTask struct {
	cancel context.CancelFunc
	kind   string
}

var fileAsyncTasks struct {
	sync.Mutex
	items map[string]fileAsyncTask
}

func startFileAsyncTask(taskID, kind, message string) (context.Context, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errors.New("任务 ID 不能为空")
	}
	ctx, cancel := context.WithCancel(context.Background())
	fileAsyncTasks.Lock()
	if fileAsyncTasks.items == nil {
		fileAsyncTasks.items = make(map[string]fileAsyncTask)
	}
	if _, exists := fileAsyncTasks.items[taskID]; exists {
		fileAsyncTasks.Unlock()
		cancel()
		return nil, errors.New("任务已存在")
	}
	fileAsyncTasks.items[taskID] = fileAsyncTask{cancel: cancel, kind: kind}
	fileAsyncTasks.Unlock()
	if err := ensureAppTaskLogChecked(taskID, "", "file-"+kind, "installing", message); err != nil {
		fileAsyncTasks.Lock()
		delete(fileAsyncTasks.items, taskID)
		fileAsyncTasks.Unlock()
		cancel()
		return nil, err
	}
	return ctx, nil
}

func finishFileAsyncTask(taskID, status, message string) {
	fileAsyncTasks.Lock()
	delete(fileAsyncTasks.items, taskID)
	fileAsyncTasks.Unlock()
	ensureAppTaskLog(taskID, "", "file-task", status, message)
	appendAppTaskLog(taskID, "[TASK-END]")
}

func stopFileAsyncTask(taskID, kind string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return errors.New("taskID 不能为空")
	}
	fileAsyncTasks.Lock()
	task, ok := fileAsyncTasks.items[taskID]
	if ok && kind != "" && task.kind != kind {
		ok = false
	}
	if ok {
		task.cancel()
	}
	fileAsyncTasks.Unlock()
	if !ok {
		return errors.New("异步文件任务不存在或已完成")
	}
	return nil
}

func checkFileTask(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return context.Canceled
	default:
		return nil
	}
}

func copyFileWithContext(ctx context.Context, dst io.Writer, src io.Reader) error {
	buffer := make([]byte, 32<<10)
	for {
		if err := checkFileTask(ctx); err != nil {
			return err
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			if _, err := dst.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func zipPathsContext(ctx context.Context, sources []string, destination string) error {
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
	zw := zip.NewWriter(tmp)
	for _, source := range sources {
		if err := checkFileTask(ctx); err != nil {
			_ = tmp.Close()
			return err
		}
		source, err = cleanFilePath(source)
		if err != nil {
			break
		}
		baseDir := filepath.Dir(source)
		err = filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := checkFileTask(ctx); err != nil {
				return err
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
			copyErr := copyFileWithContext(ctx, entry, in)
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
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, destination)
}

func tarGzipPathsContext(ctx context.Context, sources []string, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
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
		if err := checkFileTask(ctx); err != nil {
			_ = tmp.Close()
			return err
		}
		source, err = cleanFilePath(source)
		if err != nil {
			break
		}
		baseDir := filepath.Dir(source)
		err = filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := checkFileTask(ctx); err != nil {
				return err
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
			copyErr := copyFileWithContext(ctx, tw, in)
			_ = in.Close()
			return copyErr
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

func unzipPathContext(ctx context.Context, source, destination string) error {
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
		if err := checkFileTask(ctx); err != nil {
			return err
		}
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
		tmp, err := os.CreateTemp(filepath.Dir(target), ".workmesh-unzip-*")
		if err == nil {
			err = copyFileWithContext(ctx, tmp, in)
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

func untarGzipPathContext(ctx context.Context, source, destination string) error {
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
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		if err := checkFileTask(ctx); err != nil {
			return err
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
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
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		err = copyFileWithContext(ctx, out, io.LimitReader(tr, 512<<20))
		closeErr := out.Close()
		if err != nil {
			_ = os.Remove(target)
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
}

func moveFilesContext(ctx context.Context, sources []string, destination string, copyMode, replace bool, name string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for _, raw := range sources {
		if err := checkFileTask(ctx); err != nil {
			return err
		}
		src, err := cleanFilePath(raw)
		if err != nil {
			return err
		}
		targetName := filepath.Base(src)
		if len(sources) == 1 && name != "" {
			targetName = filepath.Base(name)
		}
		target := filepath.Join(destination, targetName)
		if filepath.Clean(target) == filepath.Clean(src) {
			continue
		}
		if replace {
			if err := os.RemoveAll(target); err != nil {
				return err
			}
		} else if _, err := os.Stat(target); err == nil {
			return errors.New("目标已存在: " + target)
		}
		if copyMode {
			err = copyPathContext(ctx, src, target)
		} else {
			err = os.Rename(src, target)
			if errors.Is(err, syscall.EXDEV) {
				err = copyPathContext(ctx, src, target)
				if err == nil {
					err = os.RemoveAll(src)
				}
			}
		}
		if err != nil {
			return err
		}
		applyWebsiteOwnership(target)
	}
	return nil
}

func copyPathContext(ctx context.Context, source, destination string) error {
	if err := checkFileTask(ctx); err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
			return err
		}
		return filepath.Walk(source, func(path string, item os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := checkFileTask(ctx); err != nil {
				return err
			}
			rel, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			target := filepath.Join(destination, rel)
			if item.IsDir() {
				return os.MkdirAll(target, item.Mode().Perm())
			}
			return copyPathContext(ctx, path, target)
		})
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	copyErr := copyFileWithContext(ctx, out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	return closeErr
}

func fileTaskError(taskID string, err error) {
	if errors.Is(err, context.Canceled) {
		finishFileAsyncTask(taskID, "cancelled", "文件任务已取消")
		return
	}
	finishFileAsyncTask(taskID, "failed", err.Error())
}

func fileTaskSuccess(taskID, message string) {
	finishFileAsyncTask(taskID, "running", message)
}
