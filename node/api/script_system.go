// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"embed"
	"strconv"
	"strings"
)

//go:embed script_system/*.sh
var systemScriptFiles embed.FS

const systemScriptCreatedAt = "2026-09-05T02:39:00.459798921+08:00"

type systemScriptMeta struct {
	ID          string
	Name        string
	Description string
	Interactive bool
	File        string
}

// 与 1Panel 脚本库当前返回的 9 条系统脚本对齐，搜索时直接给出中文名称。
var systemScriptCatalog = []systemScriptMeta{
	{ID: "10", Name: "安装 Docker", Description: "安装 Docker", Interactive: true, File: "script_system/10.sh"},
	{ID: "11", Name: "安装 ClamAV", Description: "安装 ClamAV", Interactive: false, File: "script_system/11.sh"},
	{ID: "12", Name: "安装 Fail2ban", Description: "安装 Fail2ban", Interactive: false, File: "script_system/12.sh"},
	{ID: "13", Name: "安装防火墙", Description: "安装 iptables / nftables / UFW / firewalld", Interactive: true, File: "script_system/13.sh"},
	{ID: "14", Name: "安装 Pure-FTPd", Description: "安装 Pure-FTPd", Interactive: false, File: "script_system/14.sh"},
	{ID: "15", Name: "安装 Supervisor", Description: "安装 Supervisor", Interactive: false, File: "script_system/15.sh"},
	{ID: "16", Name: "安装 Rsync", Description: "安装 Rsync", Interactive: false, File: "script_system/16.sh"},
	{ID: "17", Name: "安装 FFmpeg", Description: "安装 FFmpeg", Interactive: false, File: "script_system/17.sh"},
	{ID: "18", Name: "安装 KVM", Description: "安装 KVM", Interactive: false, File: "script_system/18.sh"},
}

func systemScriptByName(name string) bool {
	for _, item := range systemScriptCatalog {
		if item.Name == strings.TrimSpace(name) {
			return true
		}
	}
	return false
}

func (s *scriptLibraryStore) ensureSystemScripts() error {
	if s == nil || s.repository == nil {
		return nil
	}
	existing := map[string]struct{}{}
	for _, item := range s.items {
		existing[item.Name] = struct{}{}
	}
	added := false
	for _, meta := range systemScriptCatalog {
		if _, ok := existing[meta.Name]; ok {
			continue
		}
		body, err := systemScriptFiles.ReadFile(meta.File)
		if err != nil {
			return err
		}
		s.items = append(s.items, scriptLibraryItem{
			ID: meta.ID, Name: meta.Name, Script: string(body), Description: meta.Description,
			Approved: true, IsInteractive: meta.Interactive, IsSystem: true, CreatedAt: systemScriptCreatedAt, UpdatedAt: systemScriptCreatedAt,
		})
		added = true
	}
	if !added {
		return nil
	}
	return s.saveLocked()
}

func nextScriptNumericID(items []scriptLibraryItem) uint {
	var max uint = 18
	for _, item := range items {
		if id := scriptNumericID(item.ID); id > max {
			max = id
		}
	}
	return max + 1
}

func scriptNumericID(id string) uint {
	value, err := strconv.ParseUint(strings.TrimSpace(id), 10, 64)
	if err != nil || value == 0 {
		return 0
	}
	return uint(value)
}

func scriptIDString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed > 0 && typed == float64(uint64(typed)) {
			return strconv.FormatUint(uint64(typed), 10)
		}
	case int:
		if typed > 0 {
			return strconv.Itoa(typed)
		}
	}
	return ""
}

func scriptSearchView(it scriptLibraryItem) map[string]any {
	var groupList any = []uint(nil)
	var groupBelong any = []string(nil)
	if len(it.GroupList) > 0 {
		groupList = it.GroupList
		if it.GroupBelong == nil {
			groupBelong = []string{}
		} else {
			groupBelong = it.GroupBelong
		}
	}
	id := any(scriptNumericID(it.ID))
	if scriptNumericID(it.ID) == 0 {
		id = it.ID
	}
	return map[string]any{
		"id": id, "name": it.Name, "isInteractive": it.IsInteractive, "lable": "",
		"script": it.Script, "groupList": groupList, "groupBelong": groupBelong,
		"isSystem": it.IsSystem, "description": it.Description, "createdAt": it.CreatedAt,
	}
}
