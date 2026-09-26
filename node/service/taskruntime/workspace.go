// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var workspaceComponentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// WorkspaceLayout 描述一个项目任务的受控目录，不包含宿主网站和其他项目路径。
type WorkspaceLayout struct {
	Root      string
	Project   string
	Worktree  string
	Temp      string
	Cache     string
	Artifacts string
}

// Ensure 创建当前项目任务所需的受控目录，并拒绝任何符号链接路径组件。
// 目录创建只发生在 workspace root 内，不负责删除旧源码或缓存。
func (w WorkspaceLayout) Ensure(ctx context.Context) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := validateWorkspaceRoot(w.Root); err != nil {
		return err
	}
	for _, path := range []string{w.Project, filepath.Dir(w.Worktree), filepath.Dir(w.Temp), w.Cache, filepath.Dir(w.Artifacts), w.Worktree, w.Temp, w.Artifacts} {
		if err := ensureWorkspaceDirectory(w.Root, path); err != nil {
			return err
		}
	}
	return nil
}

// WorkspaceCleanupError 表示 Sandbox 已销毁但任务临时目录未能安全回收。
type WorkspaceCleanupError struct {
	Err error
}

// Error 返回清理错误上下文。
func (e *WorkspaceCleanupError) Error() string {
	if e == nil || e.Err == nil {
		return "任务临时目录清理失败"
	}
	return e.Err.Error()
}

// Unwrap 保留底层清理错误。
func (e *WorkspaceCleanupError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NewWorkspaceLayout 根据受控 root、项目和任务标识创建目录契约。
func NewWorkspaceLayout(root, projectID, taskID string) (WorkspaceLayout, error) {
	root = strings.TrimSpace(root)
	projectID = strings.TrimSpace(projectID)
	taskID = strings.TrimSpace(taskID)
	if root == "" || !isAbsolutePath(root) {
		return WorkspaceLayout{}, errors.New("workspace root 必须是绝对路径")
	}
	if !workspaceComponentPattern.MatchString(projectID) || projectID == "." || projectID == ".." {
		return WorkspaceLayout{}, errors.New("projectId 不是安全标识")
	}
	if !workspaceComponentPattern.MatchString(taskID) || taskID == "." || taskID == ".." {
		return WorkspaceLayout{}, errors.New("taskId 不是安全标识")
	}
	root = filepath.Clean(root)
	if root == filepath.VolumeName(root)+string(filepath.Separator) || root == "." {
		return WorkspaceLayout{}, errors.New("workspace root 不能是文件系统根目录")
	}
	info, err := os.Lstat(root)
	if err != nil {
		return WorkspaceLayout{}, fmt.Errorf("workspace root 不可用: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return WorkspaceLayout{}, errors.New("workspace root 必须是非符号链接目录")
	}
	projectRoot := filepath.Join(root, projectID)
	if err := rejectSymlinkPath(filepath.ToSlash(root), filepath.ToSlash(projectRoot)); err != nil {
		return WorkspaceLayout{}, fmt.Errorf("项目 workspace 路径不安全: %w", err)
	}
	return WorkspaceLayout{
		Root:      root,
		Project:   projectRoot,
		Worktree:  filepath.Join(projectRoot, "worktrees", taskID),
		Temp:      filepath.Join(projectRoot, "tmp", taskID),
		Cache:     filepath.Join(projectRoot, "cache"),
		Artifacts: filepath.Join(projectRoot, "artifacts", taskID),
	}, nil
}

// CleanupTransient 只删除任务 tmp 目录，保留源码、共享缓存和审计 Artifact。
func (w WorkspaceLayout) CleanupTransient(ctx context.Context) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := validateWorkspaceLayout(w); err != nil {
		return err
	}
	info, err := os.Lstat(w.Temp)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取任务临时目录失败: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("任务临时目录必须是非符号链接目录")
	}
	if err := os.RemoveAll(w.Temp); err != nil {
		return fmt.Errorf("删除任务临时目录失败: %w", err)
	}
	if _, err := os.Lstat(w.Temp); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return errors.New("任务临时目录删除后仍然存在")
		}
		return fmt.Errorf("确认任务临时目录已删除失败: %w", err)
	}
	return nil
}

func validateWorkspaceLayout(w WorkspaceLayout) error {
	root := filepath.Clean(strings.TrimSpace(w.Root))
	project := filepath.Clean(strings.TrimSpace(w.Project))
	temp := filepath.Clean(strings.TrimSpace(w.Temp))
	if root == "." || !isAbsolutePath(root) || root == filepath.VolumeName(root)+string(filepath.Separator) {
		return errors.New("workspace root 不能是文件系统根目录")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("workspace root 不可用: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("workspace root 必须是非符号链接目录")
	}
	projectRel, err := filepath.Rel(root, project)
	if err != nil || !workspaceComponentPattern.MatchString(filepath.ToSlash(projectRel)) {
		return errors.New("项目 workspace 路径结构无效")
	}
	tempRel, err := filepath.Rel(project, temp)
	if err != nil {
		return errors.New("任务临时目录路径结构无效")
	}
	parts := strings.Split(filepath.ToSlash(tempRel), "/")
	if len(parts) != 2 || parts[0] != "tmp" || !workspaceComponentPattern.MatchString(parts[1]) {
		return errors.New("任务临时目录必须是项目 tmp 下的任务子目录")
	}
	if err := rejectSymlinkPath(filepath.ToSlash(root), filepath.ToSlash(project)); err != nil {
		return err
	}
	if err := rejectSymlinkPath(filepath.ToSlash(root), filepath.ToSlash(temp)); err != nil {
		return err
	}
	return nil
}

func validateWorkspaceRoot(root string) error {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || !isAbsolutePath(root) || root == filepath.VolumeName(root)+string(filepath.Separator) {
		return errors.New("workspace root 不能是文件系统根目录")
	}
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("workspace root 不可用: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("workspace root 必须是非符号链接目录")
	}
	return nil
}

func ensureWorkspaceDirectory(root, path string) error {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("workspace 目录越出受控 root")
	}
	current := root
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		if component == "" || component == "." || component == ".." {
			return errors.New("workspace 目录组件无效")
		}
		next := filepath.Join(current, component)
		info, statErr := os.Lstat(next)
		if errors.Is(statErr, os.ErrNotExist) {
			if mkdirErr := os.Mkdir(next, 0o750); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				return fmt.Errorf("创建 workspace 目录失败: %w", mkdirErr)
			}
			info, statErr = os.Lstat(next)
		}
		if statErr != nil {
			return fmt.Errorf("检查 workspace 目录失败: %w", statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("workspace 目录必须是非符号链接目录")
		}
		current = next
	}
	return nil
}
