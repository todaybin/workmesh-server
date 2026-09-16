// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/logsource"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var analyticsAccessLogPattern = regexp.MustCompile(`^([^ ]+) [^ ]+ [^ ]+ \[([^]]+)\] "([A-Z]+) ([^ ]+) [^"]*" ([0-9]{3}) ([0-9-]+) "([^"]*)" "([^"]*)"`)

const (
	analyticsLogReadLimit = 8 << 20
	analyticsEventLimit   = 50000
)

type analyticsEvent struct {
	IP        string
	Method    string
	URI       string
	Status    int
	Bytes     int64
	Referer   string
	UserAgent string
	Occurred  time.Time
}

// registerAnalyticsRoutes 注册 Agent 站点监控统计兼容接口。
func registerAnalyticsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/status", analyticsHandler)
	for _, path := range []string{"/api/v2/attack/stat", "/api/v2/block/search", "/api/v2/config/site", "/api/v2/config/site/update", "/api/v2/global", "/api/v2/qps", "/api/v2/rank", "/api/v2/relation/stat", "/api/v2/stat", "/api/v2/test", "/api/v2/trend", "/api/v2/visitors", "/api/v2/visitors/loc"} {
		mux.HandleFunc("POST "+path, analyticsHandler)
	}
}

// isAnalyticsRoute 判断路径是否属于站点监控统计兼容接口。
func isAnalyticsRoute(pattern string) bool {
	for _, path := range []string{"/api/v2/status", "/api/v2/attack/stat", "/api/v2/block/search", "/api/v2/config/site", "/api/v2/config/site/update", "/api/v2/global", "/api/v2/qps", "/api/v2/rank", "/api/v2/relation/stat", "/api/v2/stat", "/api/v2/test", "/api/v2/trend", "/api/v2/visitors", "/api/v2/visitors/loc"} {
		if pattern == "GET "+path || pattern == "POST "+path {
			return true
		}
	}
	return false
}

// analyticsHandler 返回旧 DTO 兼容结构，并持久化全局及站点监控配置。
func analyticsHandler(w http.ResponseWriter, r *http.Request) {
	query, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	store := getDomainStore()
	path := r.URL.Path
	if path == "/api/v2/config/site/update" || path == "/api/v2/global" {
		store.mu.Lock()
		if store.state.Settings == nil {
			store.state.Settings = map[string]any{}
		}
		key := "monitorGlobal"
		if path == "/api/v2/config/site/update" {
			key = "monitorSite:" + strconv.FormatUint(uint64(analyticsWebsiteID(query)), 10)
		}
		store.state.Settings[key] = query
		err = store.saveLocked()
		store.mu.Unlock()
		if err != nil {
			domainError(w, http.StatusInternalServerError, "STATE_SAVE", err.Error())
			return
		}
		successAnalytics(w, query)
		return
	}
	if path == "/api/v2/config/site" {
		key := "monitorSite:" + strconv.FormatUint(uint64(analyticsWebsiteID(query)), 10)
		store.mu.RLock()
		value := store.state.Settings[key]
		store.mu.RUnlock()
		if value == nil {
			value = analyticsDefaultConfig(analyticsWebsiteID(query))
		}
		successAnalytics(w, value)
		return
	}
	if path == "/api/v2/status" {
		store.mu.RLock()
		value := store.state.Settings["monitorGlobal"]
		store.mu.RUnlock()
		enabled := true
		if config, ok := value.(map[string]any); ok {
			if raw, exists := config["enabled"].(bool); exists {
				enabled = raw
			}
		}
		successAnalytics(w, map[string]any{"enabled": enabled})
		return
	}
	data, err := analyticsData(path, query)
	if err != nil {
		domainError(w, http.StatusInternalServerError, "ANALYTICS_READ", err.Error())
		return
	}
	successAnalytics(w, data)
}

func successAnalytics(w http.ResponseWriter, data any) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

// analyticsWebsiteID 从多种前端字段形式解析网站 ID。
func analyticsWebsiteID(query map[string]any) uint {
	for _, key := range []string{"websiteID", "websiteId", "id"} {
		switch value := query[key].(type) {
		case float64:
			if value > 0 {
				return uint(value)
			}
		case string:
			id, _ := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
			if id > 0 {
				return uint(id)
			}
		}
	}
	return 0
}

// analyticsDefaultConfig 返回指定网站的默认监控配置。
func analyticsDefaultConfig(websiteID uint) map[string]any {
	return map[string]any{"websiteID": websiteID, "enabled": true, "storeDays": 30, "storeSize": int64(1073741824), "excludeStatus": "", "excludeExt": "", "excludeURI": "", "excludeIP": "", "excludeUA": "", "cdnType": "", "realIPHeader": ""}
}

// analyticsData 根据统计路径聚合真实访问日志事件。
func analyticsData(path string, query map[string]any) (any, error) {
	events, source, err := loadAnalyticsEvents(query)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	switch path {
	case "/api/v2/qps":
		var flow int64
		for _, event := range events {
			flow += event.Bytes
		}
		return map[string]any{"flow": flow, "qps": int64(len(events)), "updatedAt": now, "source": source}, nil
	case "/api/v2/rank":
		return analyticsRank(events, valueString(query, "type", "rankType")), nil
	case "/api/v2/visitors":
		return analyticsVisitors(events), nil
	case "/api/v2/visitors/loc":
		return analyticsVisitorLocations(events), nil
	case "/api/v2/attack/stat", "/api/v2/block/search", "/api/v2/relation/stat":
		return analyticsStatusSummary(events, path, now), nil
	case "/api/v2/test":
		return map[string]any{"passed": true, "query": query, "eventCount": len(events), "source": source, "updatedAt": now}, nil
	default:
		return analyticsDaily(events), nil
	}
}

// loadAnalyticsEvents 读取受限大小的 Nginx/OpenResty 访问日志，避免统计接口返回固定零值。
// 未找到日志时返回真实空结果；不会执行 shell，也不会读取用户任意指定目录之外的隐式文件。
func loadAnalyticsEvents(query map[string]any) ([]analyticsEvent, string, error) {
	paths := make([]string, 0, 3)
	if configured := strings.TrimSpace(os.Getenv("WORKMESH_ANALYTICS_LOG")); configured != "" {
		paths = append(paths, configured)
	} else {
		dataDir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
		if dataDir == "" {
			dataDir = "./data"
		}
		// 传入 websiteID 时，站点自己的 access.log 是唯一正确的数据源。
		// 只有未指定站点时才读取节点级日志，避免一个站点的统计混入其他站点流量。
		if websiteID := analyticsWebsiteID(query); websiteID > 0 {
			svc := service.NewWebsiteService("")
			if site, err := svc.Get(websiteID); err == nil {
				paths = append(paths, filepath.Join(svc.SitePath(site, "logs"), "access.log"))
			}
			// 旧版本站点可能仍使用 data/websites/<id>/logs，保留只读兼容路径。
			paths = append(paths, filepath.Join(dataDir, "websites", strconv.FormatUint(uint64(websiteID), 10), "logs", "access.log"))
		} else {
			paths = append(paths, filepath.Join(dataDir, "logs", "access.log"), "/var/log/nginx/access.log", "/var/log/openresty/access.log")
		}
	}
	result, err := (logsource.FileSource{}).Read(context.Background(), logsource.Request{
		Paths: paths, MaxBytesPerFile: analyticsLogReadLimit, MaxLines: analyticsEventLimit, FirstAvailable: true,
	})
	if err != nil {
		source := ""
		if len(result.UsedPaths) > 0 {
			source = result.UsedPaths[0]
		}
		return nil, source, fmt.Errorf("读取访问日志失败: %w", err)
	}
	source := ""
	if len(result.UsedPaths) > 0 {
		source = result.UsedPaths[0]
	}
	if len(result.Lines) == 0 && source == "" {
		return []analyticsEvent{}, "", nil
	}
	start, end := analyticsTimeRange(query)
	events := make([]analyticsEvent, 0)
	for _, line := range result.Lines {
		event, ok := parseAnalyticsEvent(line.Text)
		if !ok || event.Occurred.Before(start) || event.Occurred.After(end) {
			continue
		}
		events = append(events, event)
		if len(events) >= analyticsEventLimit {
			break
		}
	}
	return events, source, nil
}

// analyticsTimeRange 解析统计查询的起止时间并校正反向范围。
func analyticsTimeRange(query map[string]any) (time.Time, time.Time) {
	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	for _, key := range []string{"endTime", "end", "to"} {
		if value := parseAnalyticsTime(query[key]); !value.IsZero() {
			end = value
			break
		}
	}
	for _, key := range []string{"startTime", "start", "from"} {
		if value := parseAnalyticsTime(query[key]); !value.IsZero() {
			start = value
			break
		}
	}
	if start.After(end) {
		start, end = end, start
	}
	return start, end
}

// parseAnalyticsTime 解析时间戳、RFC3339 和常见日期字符串。
func parseAnalyticsTime(value any) time.Time {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.UTC()
		}
	}
	if seconds, err := strconv.ParseInt(text, 10, 64); err == nil && seconds > 0 {
		return time.Unix(seconds, 0).UTC()
	}
	return time.Time{}
}

// parseAnalyticsEvent 将 Nginx combined 格式日志行转换为统计事件。
func parseAnalyticsEvent(line string) (analyticsEvent, bool) {
	match := analyticsAccessLogPattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) != 9 {
		return analyticsEvent{}, false
	}
	when, err := time.Parse("02/Jan/2006:15:04:05 -0700", match[2])
	if err != nil {
		return analyticsEvent{}, false
	}
	status, err := strconv.Atoi(match[5])
	if err != nil {
		return analyticsEvent{}, false
	}
	bytes := int64(0)
	if match[6] != "-" {
		bytes, _ = strconv.ParseInt(match[6], 10, 64)
	}
	return analyticsEvent{IP: match[1], Method: match[3], URI: match[4], Status: status, Bytes: bytes, Referer: match[7], UserAgent: match[8], Occurred: when.UTC()}, true
}

// analyticsDaily 按日期汇总访问次数、流量和独立访客。
func analyticsDaily(events []analyticsEvent) []map[string]any {
	type daily struct {
		item map[string]any
		ips  map[string]struct{}
	}
	grouped := map[string]*daily{}
	for _, event := range events {
		day := event.Occurred.Format("2006-01-02")
		item := grouped[day]
		if item == nil {
			item = &daily{item: map[string]any{"day": day, "pv": int64(0), "uv": int64(0), "ip": int64(0), "flow": int64(0), "spider": int64(0), "req": int64(0), "count4xx": int64(0), "count5xx": int64(0)}, ips: map[string]struct{}{}}
			grouped[day] = item
		}
		item.item["pv"] = item.item["pv"].(int64) + 1
		item.item["req"] = item.item["req"].(int64) + 1
		item.item["flow"] = item.item["flow"].(int64) + event.Bytes
		if event.Status >= 400 && event.Status < 500 {
			item.item["count4xx"] = item.item["count4xx"].(int64) + 1
		}
		if event.Status >= 500 {
			item.item["count5xx"] = item.item["count5xx"].(int64) + 1
		}
		if event.IP != "" {
			item.ips[event.IP] = struct{}{}
		}
		if analyticsSpider(event.UserAgent) {
			item.item["spider"] = item.item["spider"].(int64) + 1
		}
	}
	result := make([]map[string]any, 0, len(grouped))
	for _, item := range grouped {
		item.item["uv"] = int64(len(item.ips))
		item.item["ip"] = int64(len(item.ips))
		result = append(result, item.item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i]["day"].(string) < result[j]["day"].(string) })
	return result
}

// analyticsVisitors 返回按日期统计的访客趋势。
func analyticsVisitors(events []analyticsEvent) []map[string]any { return analyticsDaily(events) }

// analyticsVisitorLocations 汇总访问来源 IP 的地域占位信息。
func analyticsVisitorLocations(events []analyticsEvent) []map[string]any {
	counts := map[string]int64{}
	for _, event := range events {
		location := event.IP
		if location == "" {
			location = "未知"
		}
		counts[location]++
	}
	result := make([]map[string]any, 0, len(counts))
	for name, value := range counts {
		result = append(result, map[string]any{"name": name, "value": value})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i]["value"].(int64) == result[j]["value"].(int64) {
			return result[i]["name"].(string) < result[j]["name"].(string)
		}
		return result[i]["value"].(int64) > result[j]["value"].(int64)
	})
	return result
}

// analyticsRank 按 URI、IP 或状态码生成访问排行。
func analyticsRank(events []analyticsEvent, rankType string) []map[string]any {
	counts := map[string]int64{}
	for _, event := range events {
		key := event.URI
		switch strings.ToLower(rankType) {
		case "ip":
			key = event.IP
		case "referer":
			key = event.Referer
		case "status_code", "status":
			key = strconv.Itoa(event.Status)
		case "browser":
			key = analyticsBrowser(event.UserAgent)
		case "os":
			key = analyticsOS(event.UserAgent)
		case "device":
			key = analyticsDevice(event.UserAgent)
		}
		if strings.TrimSpace(key) == "" {
			key = "-"
		}
		counts[key]++
	}
	result := make([]map[string]any, 0, len(counts))
	for name, value := range counts {
		result = append(result, map[string]any{"name": name, "value": value})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i]["value"].(int64) == result[j]["value"].(int64) {
			return result[i]["name"].(string) < result[j]["name"].(string)
		}
		return result[i]["value"].(int64) > result[j]["value"].(int64)
	})
	if len(result) > 100 {
		result = result[:100]
	}
	return result
}

// analyticsStatusSummary 生成攻击、拦截和关联统计的真实摘要。
func analyticsStatusSummary(events []analyticsEvent, path string, now time.Time) map[string]any {
	counts := map[string]int64{}
	for _, event := range events {
		include := event.Status >= 400
		if path == "/api/v2/block/search" {
			include = event.Status == http.StatusForbidden || event.Status == http.StatusTooManyRequests
		}
		if include {
			counts[strconv.Itoa(event.Status)]++
		}
	}
	items := make([]map[string]any, 0, len(counts))
	for name, value := range counts {
		items = append(items, map[string]any{"name": name, "value": value})
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["name"].(string) < items[j]["name"].(string) })
	return map[string]any{"total": int64(len(items)), "items": items, "updatedAt": now}
}

// analyticsSpider 判断 User-Agent 是否来自常见搜索或抓取机器人。
func analyticsSpider(ua string) bool {
	ua = strings.ToLower(ua)
	for _, token := range []string{"bot", "spider", "crawler", "slurp", "bingpreview"} {
		if strings.Contains(ua, token) {
			return true
		}
	}
	return false
}

// analyticsBrowser 从 User-Agent 识别浏览器类型。
func analyticsBrowser(ua string) string {
	ua = strings.ToLower(ua)
	for _, browser := range []string{"edge", "chrome", "firefox", "safari", "opera"} {
		if strings.Contains(ua, browser) {
			return browser
		}
	}
	return "Other"
}

// analyticsOS 从 User-Agent 识别客户端操作系统。
func analyticsOS(ua string) string {
	ua = strings.ToLower(ua)
	for _, osName := range []string{"windows", "android", "iphone", "mac os", "linux"} {
		if strings.Contains(ua, osName) {
			return osName
		}
	}
	return "Other"
}

// analyticsDevice 根据 User-Agent 区分移动端和桌面端。
func analyticsDevice(ua string) string {
	ua = strings.ToLower(ua)
	if strings.Contains(ua, "mobile") || strings.Contains(ua, "iphone") || strings.Contains(ua, "android") {
		return "Mobile"
	}
	return "Desktop"
}
