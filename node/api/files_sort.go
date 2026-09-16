// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"sort"
	"time"
)

// sortFileItems 按前端请求排序，并保证目录始终排在普通文件之前。
func sortFileItems(items []map[string]any, sortBy, sortOrder string) {
	if len(items) < 2 {
		return
	}
	less := fileItemLess(sortBy, sortOrder)
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

// fileItemLess 创建文件列表排序比较器，支持名称、大小和修改时间。
func fileItemLess(sortBy, sortOrder string) func(map[string]any, map[string]any) bool {
	if sortBy == "" {
		sortBy = "name"
	}
	ascending := sortOrder != "descending"
	return func(a, b map[string]any) bool {
		result := compareFileItems(a, b, sortBy)
		if result == 0 {
			result = compareFileItemText(a, b, "name")
		}
		if !ascending {
			result = -result
		}
		return result < 0
	}
}

// compareFileItems 比较文件列表的主要排序字段。
func compareFileItems(a, b map[string]any, sortBy string) int {
	switch sortBy {
	case "size":
		return compareInt64(fileItemInt64(a, "size"), fileItemInt64(b, "size"))
	case "modTime", "updateTime":
		return compareTime(fileItemTime(a, "modTime"), fileItemTime(b, "modTime"))
	default:
		return compareFileItemText(a, b, "name")
	}
}

// compareFileItemText 比较文件对象中的文本字段。
func compareFileItemText(a, b map[string]any, key string) int {
	av, bv := fileItemString(a, key), fileItemString(b, key)
	if av < bv {
		return -1
	}
	if av > bv {
		return 1
	}
	return 0
}

// compareInt64 比较两个整数并返回三态结果。
func compareInt64(a, b int64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// compareTime 比较两个时间并返回三态结果。
func compareTime(a, b time.Time) int {
	if a.Before(b) {
		return -1
	}
	if a.After(b) {
		return 1
	}
	return 0
}

// fileItemString 读取文件排序字段中的字符串值。
func fileItemString(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}

// fileItemInt64 读取文件排序字段中的整数值。
func fileItemInt64(item map[string]any, key string) int64 {
	switch value := item[key].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}

// fileItemTime 读取文件排序字段中的时间值。
func fileItemTime(item map[string]any, key string) time.Time {
	value, _ := item[key].(time.Time)
	return value
}
