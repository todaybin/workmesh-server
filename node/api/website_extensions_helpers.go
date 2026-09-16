// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// decodeExtension 限制请求体大小并解码网站扩展接口的动态 JSON 对象。
func decodeExtension(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return nil, errors.New("请求体不能为空")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	var body map[string]any
	if err := decoder.Decode(&body); err != nil {
		return nil, fmt.Errorf("解析网站扩展请求失败: %w", err)
	}
	if body == nil {
		body = map[string]any{}
	}
	return body, nil
}

// bodyString 按兼容字段顺序读取第一个非空字符串参数，并去除两侧空白。
func bodyString(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := body[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// bodyID 将历史接口中的 id、websiteID、templateID 和 outputID 统一转换为字符串。
func bodyID(body map[string]any) string {
	for _, key := range []string{"id", "websiteId", "websiteID", "templateId", "outputId"} {
		if value, ok := body[key]; ok {
			switch typed := value.(type) {
			case string:
				return strings.TrimSpace(typed)
			case float64:
				return strconv.FormatUint(uint64(typed), 10)
			}
		}
	}
	return ""
}

// bodyNumber 读取数字或数字字符串参数；缺失或无法解析时返回零值。
func bodyNumber(body map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := body[key].(float64); ok {
			return value
		}
		if value, ok := body[key].(string); ok {
			n, _ := strconv.ParseFloat(value, 64)
			return n
		}
	}
	return 0
}

// bodyIDs 将 JSON 数组中的数字或字符串 ID 转换为正整数切片。
func bodyIDs(value any) []uint {
	items, _ := value.([]any)
	result := make([]uint, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case float64:
			if v > 0 {
				result = append(result, uint(v))
			}
		case string:
			n, _ := strconv.ParseUint(v, 10, 64)
			if n > 0 {
				result = append(result, uint(n))
			}
		}
	}
	return result
}

// cloneRecord 复制一条扩展记录，避免响应修改存储中的共享 map。
func cloneRecord(item map[string]any) map[string]any {
	out := make(map[string]any, len(item))
	for key, value := range item {
		out[key] = value
	}
	return out
}

// pageRecords 按请求页码返回最多 500 条扩展记录，保持旧前端分页字段不变。
func pageRecords(items []map[string]any, body map[string]any) map[string]any {
	page, size := 1, 100
	if n, ok := body["page"].(float64); ok && int(n) > 0 {
		page = int(n)
	}
	if n, ok := body["pageSize"].(float64); ok && int(n) > 0 {
		size = int(n)
	}
	if size > 500 {
		size = 500
	}
	start := (page - 1) * size
	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	result := make([]map[string]any, end-start)
	copy(result, items[start:end])
	return map[string]any{"total": len(items), "items": result, "page": page, "pageSize": size}
}
