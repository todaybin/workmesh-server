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
	"sort"
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

func decodeFileRequest(r *http.Request) (fileRequest, error) {
	var req fileRequest
	if r.Body == nil {
		return req, errors.New("请求体不能为空")
	}
	err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req)
	return req, err
}
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

// sortFileItems keeps directories before regular files, matching the original
// panel behavior, while honoring the table's requested field and direction.
func sortFileItems(items []map[string]any, sortBy, sortOrder string) {
	if len(items) < 2 {
		return
	}
	if sortBy == "" {
		sortBy = "name"
	}
	ascending := sortOrder != "descending"
	valueString := func(item map[string]any, key string) string {
		if value, ok := item[key].(string); ok {
			return value
		}
		return ""
	}
	valueInt64 := func(item map[string]any, key string) int64 {
		switch value := item[key].(type) {
		case int64:
			return value
		case int:
			return int64(value)
		case float64:
			return int64(value)
		}
		return 0
	}
	valueTime := func(item map[string]any, key string) time.Time {
		if value, ok := item[key].(time.Time); ok {
			return value
		}
		return time.Time{}
	}
	less := func(a, b map[string]any) bool {
		var result int
		switch sortBy {
		case "size":
			av, bv := valueInt64(a, "size"), valueInt64(b, "size")
			if av < bv {
				result = -1
			} else if av > bv {
				result = 1
			}
		case "modTime", "updateTime":
			av, bv := valueTime(a, "modTime"), valueTime(b, "modTime")
			if av.Before(bv) {
				result = -1
			} else if av.After(bv) {
				result = 1
			}
		default:
			av, bv := valueString(a, "name"), valueString(b, "name")
			if av < bv {
				result = -1
			} else if av > bv {
				result = 1
			}
		}
		if result == 0 {
			// Ensure stable output when the primary field ties.
			av, bv := valueString(a, "name"), valueString(b, "name")
			if av < bv {
				result = -1
			} else if av > bv {
				result = 1
			}
		}
		if !ascending {
			result = -result
		}
		return result < 0
	}
	var dirs, files []map[string]any
	for _, item := range items {
		if isDir, _ := item["isDir"].(bool); isDir {
			dirs = append(dirs, item)
		} else {
			files = append(files, item)
		}
	}
	sort.SliceStable(dirs, func(i, j int) bool { return less(dirs[i], dirs[j]) })
	sort.SliceStable(files, func(i, j int) bool { return less(files[i], files[j]) })
	copy(items, append(dirs, files...))
}

func handleFilesSearch(w http.ResponseWriter, r *http.Request) {
	req, err := decodeFileRequest(r)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	root, err := cleanFilePath(req.Path)
	if err != nil {
		fileError(w, http.StatusBadRequest, err)
		return
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	if !rootInfo.IsDir() {
		root = filepath.Dir(root)
		rootInfo, err = os.Stat(root)
		if err != nil {
			fileError(w, http.StatusNotFound, err)
			return
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		fileError(w, http.StatusNotFound, err)
		return
	}
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if !req.ShowHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if strings.TrimSpace(req.Search) != "" && !strings.Contains(strings.ToLower(entry.Name()), strings.ToLower(strings.TrimSpace(req.Search))) {
			continue
		}
		info, e := entry.Info()
		if e == nil {
			items = append(items, fileInfo(filepath.Join(root, entry.Name()), info))
		}
	}
	sortFileItems(items, req.SortBy, req.SortOrder)
	// 原系统返回完整的 FileInfo 根对象，前端依赖 data.path 判断当前目录是否有效。
	result := fileInfo(root, rootInfo)
	result["items"] = items
	result["itemTotal"] = len(items)
	result["total"] = len(items)
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
}

func handleFilesContent(w http.ResponseWriter, r *http.Request) {
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
	data, err := os.ReadFile(path)
	if err != nil {
		fileError(w, 404, err)
		return
	}
	info, _ := os.Stat(path)
	extension := filepath.Ext(path)
	result := map[string]any{
		"path": path, "name": filepath.Base(path), "content": string(data), "size": len(data),
		"extension": extension, "mimeType": mime.TypeByExtension(extension),
	}
	if info != nil {
		result["isDir"] = info.IsDir()
		result["mode"] = info.Mode().Perm()
		result["isSymlink"] = info.Mode()&os.ModeSymlink != 0
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": result})
}
func handleFilesSave(w http.ResponseWriter, r *http.Request) {
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
	// 临时文件与目标位于同一目录，确保 rename 在同一文件系统内原子替换，
	// 避免服务中断或并发读取时看到半写入内容。
	if err = os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	// 保存前保留有限历史版本，供历史查询与恢复接口使用。
	if old, readErr := os.ReadFile(path); readErr == nil && len(old) <= 2<<20 {
		fileAux.Lock()
		loadFileAuxLocked()
		now := time.Now().UTC()
		fileAux.data.History = append(fileAux.data.History, fileHistoryItem{
			ID: fileAuxID("history"), FileID: path, Path: path, CurrentPath: path,
			FileName: filepath.Base(path), Extension: filepath.Ext(path), FileMode: "",
			Operation: "save", ContentSize: int64(len(old)), Content: string(old), CreatedAt: now, UpdatedAt: now,
		})
		if len(fileAux.data.History) > 200 {
			fileAux.data.History = fileAux.data.History[len(fileAux.data.History)-200:]
		}
		_ = saveFileAuxLocked()
		fileAux.Unlock()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".workmesh-save-*")
	if err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.WriteString(req.Content)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpName, path)
	}
	if err != nil {
		fileError(w, 500, err)
		return
	}
	applyWebsiteOwnership(path)
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}
func handleFilesCreate(w http.ResponseWriter, r *http.Request) {
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
	if req.IsDir {
		err = os.MkdirAll(path, 0755)
	} else {
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL, 0644)
		if e == nil {
			e = f.Close()
		}
		err = e
	}
	if err != nil {
		fileError(w, 409, err)
		return
	}
	applyWebsiteOwnership(path)
	info, _ := os.Stat(path)
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": fileInfo(path, info)})
}
func handleFilesDelete(w http.ResponseWriter, r *http.Request) {
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
	if req.ForceDelete {
		err = os.RemoveAll(path)
	} else {
		// 非强制删除进入本服务自己的回收目录，保留原路径以支持恢复。
		info, statErr := os.Stat(path)
		if statErr != nil {
			fileError(w, http.StatusNotFound, statErr)
			return
		}
		trashRoot := filepath.Join(filepath.Dir(fileAuxPath()), "recycle")
		if err = os.MkdirAll(trashRoot, 0o750); err == nil {
			trashPath := filepath.Join(trashRoot, fileAuxID("item"))
			err = os.Rename(path, trashPath)
			// 数据目录可能位于另一挂载点（测试临时目录也常见），此时
			// rename 会返回 EXDEV；回收到源目录旁可保持原子移动语义。
			if errors.Is(err, syscall.EXDEV) {
				localRoot := filepath.Join(filepath.Dir(path), ".workmesh-recycle")
				if mkdirErr := os.MkdirAll(localRoot, 0o750); mkdirErr != nil {
					err = mkdirErr
				} else {
					trashPath = filepath.Join(localRoot, fileAuxID("item"))
					err = os.Rename(path, trashPath)
				}
			}
			if err == nil {
				fileAux.Lock()
				loadFileAuxLocked()
				fileAux.data.Recycle = append(fileAux.data.Recycle, fileRecycleItem{ID: fileAuxID("recycle"), OriginalPath: path, TrashPath: trashPath, Name: info.Name(), Size: info.Size(), IsDir: info.IsDir(), DeletedAt: time.Now().UTC()})
				err = saveFileAuxLocked()
				fileAux.Unlock()
			}
		}
	}
	if err != nil {
		fileError(w, 404, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200})
}
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		fileError(w, 500, err)
		return
	}
	for _, raw := range paths {
		src, e := cleanFilePath(raw)
		if e != nil {
			fileError(w, 400, e)
			return
		}
		name := filepath.Base(src)
		if len(paths) == 1 && req.Name != "" {
			name = filepath.Base(req.Name)
		}
		target := filepath.Join(dir, name)
		if filepath.Clean(target) == filepath.Clean(src) {
			continue
		}
		if req.Cover {
			_ = os.RemoveAll(target)
		} else if _, e := os.Stat(target); e == nil {
			fileError(w, http.StatusConflict, fmt.Errorf("目标已存在: %s", target))
			return
		}
		if req.Type == "copy" {
			e = copyPath(src, target)
		} else {
			e = os.Rename(src, target)
			if errors.Is(e, syscall.EXDEV) {
				e = copyPath(src, target)
				if e == nil {
					e = os.RemoveAll(src)
				}
			}
		}
		if e != nil {
			fileError(w, 500, e)
			return
		}
		applyWebsiteOwnership(target)
	}
	if req.TaskID != "" {
		ensureAppTaskLog(req.TaskID, "", "file-move", "completed", "文件操作完成")
		appendAppTaskLog(req.TaskID, "[TASK-END]")
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"taskID": req.TaskID, "path": dir}})
}

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
func handleFilesDownload(w http.ResponseWriter, r *http.Request) {
	path, err := cleanFilePath(r.URL.Query().Get("path"))
	if err != nil {
		fileError(w, 400, err)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(path)+"\"")
	http.ServeFile(w, r, path)
}
func fileError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}
