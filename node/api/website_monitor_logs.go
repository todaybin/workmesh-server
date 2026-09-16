// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerWebsiteMonitorLogRoutes 注册网站监控日志的真实文件查询接口。
// 统计和列表都读取 OpenResty/Nginx 访问日志，不再使用扩展内存缓存。
func registerWebsiteMonitorLogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/websites/monitor/logs/search", websiteMonitorLogsSearch)
	mux.HandleFunc("POST /api/v2/websites/monitor/logs/detail", websiteMonitorLogsDetail)
	mux.HandleFunc("POST /api/v2/websites/monitor/logs/stat", websiteMonitorLogsStat)
	mux.HandleFunc("POST /api/v2/websites/monitor/logs/clear", websiteMonitorLogsClear)
}

// websiteMonitorLogsSearch 返回与前端 MonitorLog 页面一致的分页字段。
func websiteMonitorLogsSearch(w http.ResponseWriter, r *http.Request) {
	query, err := requestMap(r)
	if err != nil {
		websiteMonitorError(w, http.StatusBadRequest, err)
		return
	}
	events, source, err := loadAnalyticsEvents(query)
	if err != nil {
		websiteMonitorError(w, http.StatusInternalServerError, err)
		return
	}
	events = filterAnalyticsEvents(events, query)
	items := analyticsEventRecords(events, query)
	items = redactAnalyticsRecords(items)
	sort.SliceStable(items, func(i, j int) bool {
		return strings.Compare(asString(items[i]["occurredAt"]), asString(items[j]["occurredAt"])) > 0
	})
	page, pageSize := monitorPage(query)
	start := (page - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	// 日志文件通常按时间递增写入，管理端默认看到最新请求。
	pageItems := append([]map[string]any(nil), items[start:end]...)
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{
		"total": len(items), "items": pageItems, "page": page, "pageSize": pageSize, "source": source,
	}})
}

// websiteMonitorLogsDetail 返回筛选条件命中的最新一条真实访问记录。
func websiteMonitorLogsDetail(w http.ResponseWriter, r *http.Request) {
	query, err := requestMap(r)
	if err != nil {
		websiteMonitorError(w, http.StatusBadRequest, err)
		return
	}
	events, source, err := loadAnalyticsEvents(query)
	if err != nil {
		websiteMonitorError(w, http.StatusInternalServerError, err)
		return
	}
	events = filterAnalyticsEvents(events, query)
	items := analyticsEventRecords(events, query)
	items = redactAnalyticsRecords(items)
	if len(items) == 0 {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"found": false, "source": source}})
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		return strings.Compare(asString(items[i]["occurredAt"]), asString(items[j]["occurredAt"])) > 0
	})
	item := items[0]
	item["source"] = source
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
}

// websiteMonitorLogsStat 返回真实日志记录总量，供监控页面展示来源和更新时间。
func websiteMonitorLogsStat(w http.ResponseWriter, r *http.Request) {
	query, err := requestMap(r)
	if err != nil {
		websiteMonitorError(w, http.StatusBadRequest, err)
		return
	}
	events, source, err := loadAnalyticsEvents(query)
	if err != nil {
		websiteMonitorError(w, http.StatusInternalServerError, err)
		return
	}
	events = filterAnalyticsEvents(events, query)
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{
		"total": len(events), "source": source,
	}})
}

// websiteMonitorLogsClear 清空当前站点真实 access.log；未指定站点时拒绝隐式清理全机日志。
func websiteMonitorLogsClear(w http.ResponseWriter, r *http.Request) {
	query, err := requestMap(r)
	if err != nil {
		websiteMonitorError(w, http.StatusBadRequest, err)
		return
	}
	websiteID := analyticsWebsiteID(query)
	if websiteID == 0 {
		websiteMonitorError(w, http.StatusBadRequest, errors.New("清理网站监控日志必须提供 websiteID"))
		return
	}
	if err := service.NewWebsiteService("").ClearWebsiteLog(websiteID, "access.log"); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			websiteMonitorError(w, http.StatusNotFound, err)
			return
		}
		websiteMonitorError(w, http.StatusInternalServerError, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"cleared": true, "websiteID": websiteID}})
}

// filterAnalyticsEvents 应用监控页面的 IP、URI、方法、状态码和时间条件。
func filterAnalyticsEvents(events []analyticsEvent, query map[string]any) []analyticsEvent {
	ip := valueString(query, "ip", "clientIP")
	uri := valueString(query, "uri", "path")
	method := strings.ToUpper(valueString(query, "method"))
	status := int(bodyNumber(query, "status", "statusCode"))
	filtered := make([]analyticsEvent, 0, len(events))
	for _, event := range events {
		if ip != "" && !strings.Contains(event.IP, ip) {
			continue
		}
		if uri != "" && !strings.Contains(event.URI, uri) {
			continue
		}
		if method != "" && event.Method != method {
			continue
		}
		if status > 0 && event.Status != status {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

// analyticsEventRecords 将内部日志记录转换成前端 MonitorLog 所需字段。
func analyticsEventRecords(events []analyticsEvent, query map[string]any) []map[string]any {
	websiteID := analyticsWebsiteID(query)
	items := make([]map[string]any, 0, len(events))
	for _, event := range events {
		item := map[string]any{
			"ip": event.IP, "method": event.Method, "uri": event.URI, "status": event.Status,
			"bytes": event.Bytes, "durationMs": int64(0), "referer": event.Referer,
			"userAgent": event.UserAgent, "occurredAt": event.Occurred.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if websiteID > 0 {
			item["websiteID"] = websiteID
		}
		items = append(items, item)
	}
	return items
}

// redactAnalyticsRecords 脱敏网站访问日志响应中的请求头、来源和查询参数。
func redactAnalyticsRecords(items []map[string]any) []map[string]any {
	redacted := make([]map[string]any, 0, len(items))
	for _, item := range items {
		redacted = append(redacted, redactAnalyticsRecord(item))
	}
	return redacted
}

// monitorPage 统一处理页面分页参数并限制单次响应大小。
func monitorPage(query map[string]any) (int, int) {
	page, pageSize := int(bodyNumber(query, "page")), int(bodyNumber(query, "pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 20
	}
	return page, pageSize
}

func asString(value any) string {
	return strings.TrimSpace(fmt.Sprint(value))
}

func websiteMonitorError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}
