// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"net/http"
	"os"
	osuser "os/user"
	"runtime"
	"strconv"
	"strings"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// handleFileMode 修改单个文件的权限模式并保持原响应结构。
func handleFileMode(w http.ResponseWriter, req fileAdvancedRequest) {
	clean, err := cleanFilePath(req.Path)
	if err != nil || req.Mode < 0 || req.Mode > 0o7777 {
		if err == nil {
			err = errors.New("mode 超出范围")
		}
		fileError(w, 400, err)
		return
	}
	if err := os.Chmod(clean, os.FileMode(req.Mode)); err != nil {
		fileError(w, 500, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"path": clean, "mode": req.Mode}})
}

// handleFileOwner 修改单个文件的属主和属组，保留平台错误语义。
func handleFileOwner(w http.ResponseWriter, req fileAdvancedRequest) {
	clean, err := cleanFilePath(req.Path)
	if err != nil || (req.User == "" && req.Group == "") {
		if err == nil {
			err = errors.New("user 或 group 不能为空")
		}
		fileError(w, http.StatusBadRequest, err)
		return
	}
	if runtime.GOOS == "windows" {
		fileError(w, http.StatusNotImplemented, errors.New("Windows 不支持修改文件 owner"))
		return
	}
	uid, gid := -1, -1
	if req.User != "" {
		u, lookupErr := osuser.Lookup(req.User)
		if lookupErr != nil {
			fileError(w, http.StatusBadRequest, lookupErr)
			return
		}
		uid, _ = strconv.Atoi(u.Uid)
	}
	if req.Group != "" {
		g, lookupErr := osuser.LookupGroup(req.Group)
		if lookupErr != nil {
			fileError(w, http.StatusBadRequest, lookupErr)
			return
		}
		gid, _ = strconv.Atoi(g.Gid)
	}
	if err := os.Chown(clean, uid, gid); err != nil {
		fileError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"path": clean, "user": req.User, "group": req.Group}})
}

// handleFileUserGroup 返回主机真实用户和用户组列表。
func handleFileUserGroup(w http.ResponseWriter) {
	users := make([]map[string]string, 0, 32)
	userGroups := map[string]string{}
	groupNames := make([]string, 0, 32)
	seenGroups := map[string]bool{}
	if raw, err := os.ReadFile("/etc/group"); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			fields := strings.Split(line, ":")
			if len(fields) > 2 {
				gid := fields[2]
				if !seenGroups[fields[0]] {
					groupNames = append(groupNames, fields[0])
					seenGroups[fields[0]] = true
				}
				userGroups[gid] = fields[0]
			}
		}
	}
	if raw, err := os.ReadFile("/etc/passwd"); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			fields := strings.Split(line, ":")
			if len(fields) > 3 {
				primaryGroup := userGroups[fields[3]]
				users = append(users, map[string]string{"username": fields[0], "group": primaryGroup, "uid": fields[2], "gid": fields[3]})
			}
		}
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"users": users, "groups": groupNames}})
}
