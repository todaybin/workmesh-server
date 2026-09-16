// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	osuser "os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type fileRequest struct {
	Path        string   `json:"path"`
	Content     string   `json:"content"`
	Name        string   `json:"name"`
	IsDir       bool     `json:"isDir"`
	ForceDelete bool     `json:"forceDelete"`
	Paths       []string `json:"paths"`
	Dst         string   `json:"dst"`
	OldName     string   `json:"oldName"`
	NewName     string   `json:"newName"`
	OldPaths    []string `json:"oldPaths"`
	NewPath     string   `json:"newPath"`
	Type        string   `json:"type"`
	Cover       bool     `json:"cover"`
	CoverPaths  []string `json:"coverPaths"`
	TaskID      string   `json:"taskID"`
	Files       []string `json:"files"`
	Replace     bool     `json:"replace"`
	Secret      string   `json:"secret"`
	ShowHidden  bool     `json:"showHidden"`
	SortBy      string   `json:"sortBy"`
	SortOrder   string   `json:"sortOrder"`
	Search      string   `json:"search"`
	Page        int      `json:"page"`
	PageSize    int      `json:"pageSize"`
}

// decodeFileRequest 读取并限制文件接口的 JSON 请求体大小。
func decodeFileRequest(r *http.Request) (fileRequest, error) {
	var req fileRequest
	if r.Body == nil {
		return req, errors.New("请求体不能为空")
	}
	err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req)
	return req, err
}

// cleanFilePath 规范化文件路径并拒绝路径穿越输入。
func cleanFilePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("文件路径不能为空")
	}
	for _, segment := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == ".." {
			return "", errors.New("path traversal is not allowed")
		}
	}
	clean := filepath.Clean(path)
	if clean == "." || strings.ContainsRune(clean, 0) {
		return "", errors.New("文件路径无效")
	}
	return clean, nil
}

// fileInfo 将操作系统文件信息转换为前端需要的对象结构。
func fileInfo(path string, info os.FileInfo) map[string]any {
	extension := filepath.Ext(info.Name())
	uid, gid, user, group := fileOwner(info)
	return map[string]any{
		"path": path, "name": info.Name(), "size": info.Size(), "isDir": info.IsDir(),
		"isSymlink": info.Mode()&os.ModeSymlink != 0, "isHidden": strings.HasPrefix(info.Name(), "."),
		"mode": fmt.Sprintf("%04o", info.Mode().Perm()), "modTime": info.ModTime(), "updateTime": info.ModTime(),
		"extension": extension, "mimeType": mime.TypeByExtension(extension),
		"uid": uid, "gid": gid, "user": user, "group": group,
		"type": map[bool]string{true: "dir", false: "file"}[info.IsDir()],
	}
}

// fileOwner 解析文件的数字 UID/GID 及可读用户名和组名。
func fileOwner(info os.FileInfo) (uid, gid, user, group string) {
	uid, gid = "-", "-"
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		uid = strconv.FormatUint(uint64(stat.Uid), 10)
		gid = strconv.FormatUint(uint64(stat.Gid), 10)
		user, group = uid, gid
		if item, err := osuser.LookupId(uid); err == nil {
			user = item.Username
		}
		if item, err := osuser.LookupGroupId(gid); err == nil {
			group = item.Name
		}
	}
	return
}

// applyWebsiteOwnership 为网站目录中的新文件应用面板约定的属主。
func applyWebsiteOwnership(path string) {
	if !isWebsiteContentPath(path) {
		return
	}
	user, userErr := osuser.Lookup("www")
	group, groupErr := osuser.LookupGroup("www")
	if userErr != nil || groupErr != nil {
		return
	}
	uid, uidErr := strconv.Atoi(user.Uid)
	gid, gidErr := strconv.Atoi(group.Gid)
	if uidErr == nil && gidErr == nil {
		_ = os.Chown(path, uid, gid)
	}
}

// handleFilesRename 重命名文件或目录并返回新的路径。
func handleFilesRename(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	oldPath := req.Path
	if strings.TrimSpace(req.OldName) != "" {
		oldPath = req.OldName
	}
	old, err := cleanFilePath(oldPath)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	newName := req.Name
	if strings.TrimSpace(req.NewName) != "" {
		newName = req.NewName
	}
	if newName == "" {
		fileError(w, 400, errors.New("新名称不能为空"))
		return
	}
	if filepath.Base(newName) != newName || newName == "." || newName == ".." {
		fileError(w, 400, errors.New("新名称包含非法路径"))
		return
	}
	dst := filepath.Join(filepath.Dir(old), newName)
	if _, statErr := os.Stat(dst); statErr == nil {
		fileError(w, http.StatusConflict, errors.New("目标名称已存在"))
		return
	}
	if err = os.Rename(old, dst); err != nil {
		fileError(w, 500, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": dst}})
}

// handleFilesMove 移动或复制一个或多个文件，并记录异步任务完成状态。
func handleFilesMove(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	paths := req.OldPaths
	if len(paths) == 0 && req.Path != "" {
		paths = []string{req.Path}
	}
	dir := req.NewPath
	if dir == "" {
		dir = req.Dst
	}
	dir, err = cleanFilePath(dir)
	if err != nil || len(paths) == 0 {
		if err == nil {
			err = errors.New("源文件不能为空")
		}
		fileError(w, 400, err)
		return
	}
	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		taskID = idToken()
	}
	ctx, err := startFileAsyncTask(taskID, "move", "开始文件移动")
	if err != nil {
		fileError(w, http.StatusConflict, err)
		return
	}
	copyMode := req.Type == "copy"
	go func() {
		if err := moveFilesContext(ctx, paths, dir, copyMode, req.Cover, req.Name); err != nil {
			fileTaskError(taskID, err)
			return
		}
		fileTaskSuccess(taskID, "文件操作完成")
	}()
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"taskID": taskID, "path": dir, "status": "queued"}})
}

// copyPath 递归复制文件或目录，处理跨设备移动的回退场景。
func copyPath(source, destination string) error {
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
			rel, _ := filepath.Rel(source, path)
			if rel == "." {
				return nil
			}
			target := filepath.Join(destination, rel)
			if item.IsDir() {
				return os.MkdirAll(target, item.Mode().Perm())
			}
			return copyPath(path, target)
		})
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// handleFilesSize 计算文件或目录下的实际文件总大小。
func handleFilesSize(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	path, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	var total int64
	filepath.Walk(path, func(_ string, info os.FileInfo, e error) error {
		if e == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": path, "size": total}})
}

// handleFilesTree 构建最多三层的文件树，供文件选择器使用。
func handleFilesTree(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	root, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, 400, err)
		return
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	var walk func(string, int) []map[string]any
	walk = func(path string, depth int) []map[string]any {
		if depth > 2 {
			return nil
		}
		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return nil
		}
		result := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			if !req.ShowHidden && strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			full := filepath.Join(path, entry.Name())
			info, e := entry.Info()
			if e != nil {
				continue
			}
			item := map[string]any{
				"id": fileAuxID("tree"), "name": entry.Name(), "path": full,
				"isDir": info.IsDir(), "extension": filepath.Ext(entry.Name()),
			}
			if info.IsDir() {
				item["children"] = walk(full, depth+1)
			}
			result = append(result, item)
		}
		return result
	}
	rootNode := map[string]any{
		"id": fileAuxID("tree"), "name": filepath.Base(root), "path": root,
		"isDir": rootInfo.IsDir(), "extension": filepath.Ext(rootInfo.Name()),
	}
	if rootInfo.IsDir() {
		rootNode["children"] = walk(root, 0)
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": []map[string]any{rootNode}})
}

// handleFilesUpload 接收 multipart 文件并以临时文件原子写入目标目录。
func handleFilesUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		fileError(w, 400, err)
		return
	}
	dir, err := cleanFilePath(r.FormValue("path"))
	if err != nil {
		fileError(w, 400, err)
		return
	}
	for _, headers := range r.MultipartForm.File {
		for _, header := range headers {
			in, e := header.Open()
			if e != nil {
				fileError(w, 400, e)
				return
			}
			dst := filepath.Join(dir, filepath.Base(header.Filename))
			out, e := os.CreateTemp(dir, ".workmesh-upload-*")
			tmpName := ""
			if e == nil {
				tmpName = out.Name()
				_ = out.Chmod(0o600)
				_, e = io.Copy(out, io.LimitReader(in, 64<<20))
				if closeErr := out.Close(); e == nil {
					e = closeErr
				}
				if e == nil {
					e = os.Rename(tmpName, dst)
				} else {
					_ = os.Remove(tmpName)
				}
			}
			_ = in.Close()
			if e != nil {
				fileError(w, 500, e)
				return
			}
			applyWebsiteOwnership(dst)
			if info, statErr := os.Stat(dst); statErr == nil {
				fileAux.Lock()
				loadFileAuxLocked()
				fileAux.data.Uploads = append(fileAux.data.Uploads, fileUploadItem{ID: fileAuxID("upload"), Path: dst, Name: info.Name(), Size: info.Size(), CreatedAt: time.Now().UTC()})
				_ = saveFileAuxLocked()
				fileAux.Unlock()
			}
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}
