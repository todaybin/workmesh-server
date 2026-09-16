// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// GetWebsiteProxyCache 返回站点当前反向代理缓存配置；配置来源是实际 proxy 文件。
func (s *WebsiteService) GetWebsiteProxyCache(id uint) (map[string]any, error) {
	items, err := s.ListWebsiteProxies(id)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(toString(item["name"])), "root") || len(items) == 1 {
			item["open"] = item["cache"]
			item["cacheLimit"] = item["serverCacheTime"]
			item["cacheLimitUnit"] = item["serverCacheUnit"]
			item["shareCache"] = item["serverCacheTime"]
			item["shareCacheUnit"] = item["serverCacheUnit"]
			item["cacheExpire"] = item["cacheTime"]
			item["cacheExpireUnit"] = item["cacheUnit"]
			return item, nil
		}
	}
	return map[string]any{"id": id, "cache": false, "open": false, "cacheTime": 0, "cacheUnit": "", "serverCacheTime": 0, "serverCacheUnit": "", "cacheLimit": 0, "cacheExpire": 0}, nil
}

// UpdateWebsiteProxyCache 将缓存字段合并到 root 代理并执行真实配置校验。
func (s *WebsiteService) UpdateWebsiteProxyCache(id uint, value map[string]any) (map[string]any, error) {
	if id == 0 {
		return nil, errors.New("网站 ID 无效")
	}
	if value == nil {
		value = map[string]any{}
	}
	current, err := s.GetWebsiteProxyCache(id)
	if err != nil {
		return nil, err
	}
	merged := cloneMap(current)
	for _, key := range []string{"cache", "cacheTime", "cacheUnit", "cacheUint", "serverCacheTime", "serverCacheUnit", "serverCacheUint"} {
		if raw, ok := value[key]; ok {
			merged[key] = raw
		}
	}
	if raw, ok := value["open"]; ok {
		merged["cache"] = raw
	}
	if raw, ok := value["cacheExpire"]; ok {
		merged["cacheTime"] = raw
	}
	if raw, ok := value["cacheExpireUnit"]; ok {
		merged["cacheUnit"] = raw
	}
	if raw, ok := value["shareCache"]; ok {
		merged["serverCacheTime"] = raw
	}
	if raw, ok := value["shareCacheUnit"]; ok {
		merged["serverCacheUnit"] = raw
	}
	if raw, ok := value["cacheLimit"]; ok && value["shareCache"] == nil {
		merged["serverCacheTime"] = raw
	}
	if raw, ok := value["cacheLimitUnit"]; ok && value["shareCacheUnit"] == nil {
		merged["serverCacheUnit"] = raw
	}
	name := toString(current["name"])
	if name == "" {
		name = "root"
	}
	merged["enabled"] = true
	return s.UpdateNamedWebsiteProxy(id, name, "edit", merged)
}

// ClearWebsiteProxyCache 删除站点缓存目录并重新加载 OpenResty（可用时）。
func (s *WebsiteService) ClearWebsiteProxyCache(id uint) error {
	site, err := s.Get(id)
	if err != nil {
		return err
	}
	cacheDir := s.SitePath(site, "cache")
	if err := os.RemoveAll(cacheDir); err != nil {
		return err
	}
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return err
	}
	status := s.ProbeOpenResty(context.Background())
	if status.Available && status.ConfigValid && status.Binary != "" {
		if _, err := s.OperateOpenResty(context.Background(), "reload"); err != nil {
			return err
		}
	}
	return nil
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
