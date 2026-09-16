package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/logsource"
)

// QueryWAFLogs reads WorkMesh-owned JSONL audit files. It deliberately does
// not consult SQLite or any external WAF installation.
func (s *WebsiteService) QueryWAFLogs(kind string, filters map[string]string, page, pageSize int) (map[string]any, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 20
	}
	items, err := s.readWAFLogItems(kind, filters)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		return fmt.Sprint(items[i]["timestamp"], items[i]["time"], items[i]["createdAt"]) > fmt.Sprint(items[j]["timestamp"], items[j]["time"], items[j]["createdAt"])
	})
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return map[string]any{"page": page, "pageSize": pageSize, "total": total, "items": items[start:end], "source": "workmesh-waf-files"}, nil
}

func (s *WebsiteService) wafLogRoot() string {
	if root := strings.TrimSpace(os.Getenv("WORKMESH_WAF_LOG_DIR")); root != "" {
		return filepath.Clean(root)
	}
	root := s.wafRoot()
	if filepath.Base(root) == "data" {
		return filepath.Join(filepath.Dir(root), "logs")
	}
	return filepath.Join(root, "logs")
}

// WAFOverview returns the aggregate data used by the dashboard. It reads all
// matching WorkMesh JSONL records instead of deriving counts from one page of
// the log table.
func (s *WebsiteService) WAFOverview() (map[string]any, error) {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	requests, err := s.readWAFLogItems("log", nil)
	if err != nil {
		return nil, err
	}
	intercepts, err := s.readWAFLogItems("intercept", nil)
	if err != nil {
		return nil, err
	}

	requestTrend := make([]map[string]any, 0, 7)
	interceptTrend := make([]map[string]any, 0, 7)
	requestDays := map[string]int{}
	interceptDays := map[string]int{}
	for offset := 6; offset >= 0; offset-- {
		day := now.AddDate(0, 0, -offset).Format("2006-01-02")
		requestDays[day], interceptDays[day] = 0, 0
	}
	sources := map[string]int{}
	todayRequests, todayIntercepts, today4xx, today5xx := 0, 0, 0, 0
	for _, item := range requests {
		day, ok := wafLogDay(item)
		if !ok {
			continue
		}
		if _, exists := requestDays[day]; exists {
			requestDays[day]++
		}
		if day == today {
			todayRequests++
			status := wafLogStatus(item)
			if status >= 400 && status < 500 {
				today4xx++
			}
			if status >= 500 {
				today5xx++
			}
		}
	}
	for _, item := range intercepts {
		if !wafLogIsIntercept(item) {
			continue
		}
		day, ok := wafLogDay(item)
		if !ok {
			continue
		}
		if _, exists := interceptDays[day]; exists {
			interceptDays[day]++
		}
		if day == today {
			todayIntercepts++
		}
		if time.Since(wafLogTime(item)) <= 30*24*time.Hour {
			source := firstLogValue(item, "ipRegion", "region", "country", "source")
			if source == "" {
				source = "未知来源"
			}
			sources[source]++
		}
	}
	for offset := 6; offset >= 0; offset-- {
		day := now.AddDate(0, 0, -offset).Format("2006-01-02")
		requestTrend = append(requestTrend, map[string]any{"day": day, "count": requestDays[day]})
		interceptTrend = append(interceptTrend, map[string]any{"day": day, "count": interceptDays[day]})
	}
	sourceRows := make([]map[string]any, 0, len(sources))
	for source, count := range sources {
		sourceRows = append(sourceRows, map[string]any{"source": source, "count": count})
	}
	sort.Slice(sourceRows, func(i, j int) bool {
		return sourceRows[i]["count"].(int) > sourceRows[j]["count"].(int)
	})
	if len(sourceRows) > 20 {
		sourceRows = sourceRows[:20]
	}
	return map[string]any{
		"today": map[string]any{
			"requests": todayRequests, "intercepts": todayIntercepts,
			"count4xx": today4xx, "count5xx": today5xx,
		},
		"requestTrend":   requestTrend,
		"interceptTrend": interceptTrend,
		"sources":        sourceRows,
		"source":         "workmesh-waf-files",
	}, nil
}

func (s *WebsiteService) readWAFLogItems(kind string, filters map[string]string) ([]map[string]any, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	fileNames := map[string][]string{
		"access":    {"access.jsonl", "workmesh-custom-audit.jsonl", "access.log", "modsecurity-audit.json"},
		"log":       {"access.jsonl", "audit.jsonl", "workmesh-custom-audit.jsonl", "access.log", "modsecurity-audit.json"},
		"attack":    {"attack.jsonl", "audit.jsonl", "workmesh-custom-audit.jsonl", "modsecurity-audit.json"},
		"intercept": {"intercept.jsonl", "audit.jsonl", "workmesh-custom-audit.jsonl", "modsecurity-audit.json"},
		"block":     {"block.jsonl", "audit.jsonl", "workmesh-custom-audit.jsonl", "modsecurity-audit.json"},
	}
	names, ok := fileNames[kind]
	if !ok {
		return nil, fmt.Errorf("不支持的 WAF 日志类型: %s", kind)
	}
	type sourceFile struct {
		path      string
		host      string
		websiteID uint
	}
	paths := make([]sourceFile, 0, len(names))
	for _, name := range names {
		paths = append(paths, sourceFile{path: filepath.Join(s.wafLogRoot(), name)})
	}
	s.mu.RLock()
	sites := append([]modelWebsiteSnapshot(nil), snapshotWebsites(s)...)
	s.mu.RUnlock()
	for _, site := range sites {
		for _, name := range names {
			paths = append(paths, sourceFile{path: filepath.Join(site.wafDir, "logs", name), host: site.host, websiteID: site.websiteID})
		}
	}
	pathsForRead := make([]string, 0, len(paths))
	for _, source := range paths {
		pathsForRead = append(pathsForRead, source.path)
	}
	readResult, err := (logsource.FileSource{}).Read(context.Background(), logsource.Request{
		Paths: pathsForRead, MaxBytesPerFile: 64 << 20, MaxLines: 100000,
	})
	if err != nil {
		return nil, err
	}
	sourceByPath := make(map[string]sourceFile, len(paths))
	for _, source := range paths {
		sourceByPath[filepath.Clean(source.path)] = source
	}
	linesByPath := make(map[string][]string, len(readResult.UsedPaths))
	pathOrder := make([]string, 0, len(readResult.UsedPaths))
	for _, line := range readResult.Lines {
		path := filepath.Clean(line.Path)
		if _, exists := linesByPath[path]; !exists {
			pathOrder = append(pathOrder, path)
		}
		linesByPath[path] = append(linesByPath[path], line.Text)
	}

	items := make([]map[string]any, 0, len(readResult.Lines))
	seen := map[string]bool{}
	for _, path := range pathOrder {
		source := sourceByPath[path]
		records := parseWAFLogFile(filepath.Base(path), strings.Join(linesByPath[path], "\n"))
		for _, item := range records {
			if source.host != "" && !hasLogValue(item["host"]) {
				item["host"] = source.host
			}
			if source.websiteID != 0 && !hasLogValue(item["websiteID"]) {
				item["websiteID"] = source.websiteID
			}
			key, _ := json.Marshal(item)
			if seen[string(key)] {
				continue
			}
			seen[string(key)] = true
			if (kind == "intercept" || kind == "block") && !wafLogIsIntercept(item) {
				continue
			}
			if wafLogMatches(item, filters) {
				items = append(items, item)
			}
		}
	}
	return items, nil
}

var nginxCombinedLogPattern = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "([^"]*)" ([0-9]{3}) (\S+) "([^"]*)" "([^"]*)"$`)

// parseWAFLogFile only accepts the explicitly selected access.log as Nginx
// combined text. All other selected files must contain JSON records.
func parseWAFLogFile(name, content string) []map[string]any {
	if filepath.Base(name) == "access.log" {
		return parseNginxCombinedLogs(content)
	}
	return parseWAFJSONRecords(content)
}

func parseWAFJSONRecords(content string) []map[string]any {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	var records []map[string]any
	decoder := json.NewDecoder(strings.NewReader(content))
	decoded := false
	for {
		var value any
		err := decoder.Decode(&value)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			decoded = false
			break
		}
		decoded = true
		appendWAFJSONRecords(&records, value)
	}
	if decoded {
		return records
	}

	// A malformed line must not discard valid JSONL records around it.
	records = nil
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		var value any
		if err := json.Unmarshal([]byte(strings.TrimSpace(scanner.Text())), &value); err != nil {
			continue
		}
		appendWAFJSONRecords(&records, value)
	}
	return records
}

func appendWAFJSONRecords(records *[]map[string]any, value any) {
	switch raw := value.(type) {
	case map[string]any:
		*records = append(*records, normalizeWAFLogRecord(raw))
	case []any:
		for _, entry := range raw {
			appendWAFJSONRecords(records, entry)
		}
	}
}

func normalizeWAFLogRecord(item map[string]any) map[string]any {
	transaction := logMap(item["transaction"])
	request := logMap(transaction["request"])
	response := logMap(transaction["response"])
	if len(request) == 0 {
		request = logMap(item["request"])
	}
	if len(response) == 0 {
		response = logMap(item["response"])
	}

	if !hasLogValue(item["client_ip"]) {
		if value := firstMapValue(transaction, "client_ip", "clientIP", "ip"); hasLogValue(value) {
			item["client_ip"] = value
		}
	}
	if !hasLogValue(item["client_ip"]) && hasLogValue(item["clientIP"]) {
		item["client_ip"] = item["clientIP"]
	}

	when := firstMapValue(item, "timestamp", "time", "createdAt", "created_at")
	if !hasLogValue(when) {
		when = firstMapValue(transaction, "time_stamp", "timestamp", "time", "createdAt", "created_at")
	}
	if hasLogValue(when) {
		if !hasLogValue(item["timestamp"]) {
			item["timestamp"] = when
		}
		if !hasLogValue(item["time"]) {
			item["time"] = when
		}
	}

	if !hasLogValue(item["status"]) && !hasLogValue(item["statusCode"]) && !hasLogValue(item["status_code"]) {
		if value := firstMapValue(response, "http_code", "httpCode", "status", "statusCode"); hasLogValue(value) {
			item["status"] = value
		}
	}
	if !hasLogValue(item["uri"]) {
		if value := firstMapValue(request, "uri", "url", "request_uri"); hasLogValue(value) {
			item["uri"] = value
		}
	}
	if !hasLogValue(item["method"]) {
		if value := firstMapValue(request, "method"); hasLogValue(value) {
			item["method"] = value
		}
	}
	if !hasLogValue(item["host"]) {
		headers := logMap(request["headers"])
		if value := firstMapValue(headers, "host", "Host"); hasLogValue(value) {
			item["host"] = value
		} else if value := firstMapValue(request, "http_host", "host"); hasLogValue(value) {
			item["host"] = value
		}
	}
	if item["messages"] != nil && item["message"] == nil {
		item["message"] = item["messages"]
	}
	return item
}

func parseNginxCombinedLogs(content string) []map[string]any {
	result := make([]map[string]any, 0)
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		matches := nginxCombinedLogPattern.FindStringSubmatch(line)
		if len(matches) != 8 || net.ParseIP(matches[1]) == nil {
			continue
		}
		when, err := time.Parse("02/Jan/2006:15:04:05 -0700", matches[2])
		if err != nil {
			continue
		}
		status, err := strconv.Atoi(matches[4])
		if err != nil {
			continue
		}
		if matches[5] != "-" {
			if _, err := strconv.ParseInt(matches[5], 10, 64); err != nil {
				continue
			}
		}
		request := strings.Fields(matches[3])
		if len(request) != 3 || !strings.HasPrefix(request[2], "HTTP/") {
			continue
		}
		timestamp := when.UTC().Format(time.RFC3339)
		result = append(result, map[string]any{
			"client_ip":  matches[1],
			"timestamp":  timestamp,
			"time":       timestamp,
			"method":     request[0],
			"uri":        request[1],
			"status":     status,
			"bytes":      matches[5],
			"referer":    matches[6],
			"user_agent": matches[7],
			"source":     "access-log",
		})
	}
	return result
}

func logMap(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func firstMapValue(item map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, exists := item[key]; exists && hasLogValue(value) {
			return value
		}
	}
	return nil
}

func hasLogValue(value any) bool {
	if value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) != ""
	}
	return true
}

// ClearWAFLogs truncates only the fixed WorkMesh/OpenResty log files selected
// by kind. It never accepts a caller-supplied path.
func (s *WebsiteService) ClearWAFLogs(kind string) (int, error) {
	names := map[string][]string{
		"access":    {"access.jsonl", "workmesh-custom-audit.jsonl", "access.log", "modsecurity-audit.json"},
		"log":       {"access.jsonl", "audit.jsonl", "workmesh-custom-audit.jsonl", "access.log", "modsecurity-audit.json"},
		"attack":    {"attack.jsonl", "audit.jsonl", "workmesh-custom-audit.jsonl", "modsecurity-audit.json"},
		"intercept": {"intercept.jsonl", "audit.jsonl", "workmesh-custom-audit.jsonl", "modsecurity-audit.json"},
		"block":     {"block.jsonl", "audit.jsonl", "workmesh-custom-audit.jsonl", "modsecurity-audit.json"},
	}
	files, ok := names[strings.ToLower(strings.TrimSpace(kind))]
	if !ok {
		return 0, fmt.Errorf("不支持的 WAF 日志类型: %s", kind)
	}
	paths := make([]string, 0)
	for _, name := range files {
		paths = append(paths, filepath.Join(s.wafLogRoot(), name))
	}
	s.mu.RLock()
	for _, site := range snapshotWebsites(s) {
		for _, name := range files {
			paths = append(paths, filepath.Join(site.wafDir, "logs", name))
		}
	}
	s.mu.RUnlock()
	result, err := (logsource.FileMaintenance{}).Cleanup(context.Background(), logsource.CleanupRequest{
		Paths: paths, Mode: logsource.CleanupTruncate,
	})
	return len(result.ClearedPaths), err
}

type modelWebsiteSnapshot struct {
	wafDir    string
	host      string
	websiteID uint
}

func snapshotWebsites(s *WebsiteService) []modelWebsiteSnapshot {
	out := make([]modelWebsiteSnapshot, 0, len(s.websites))
	for _, site := range s.websites {
		out = append(out, modelWebsiteSnapshot{wafDir: s.SitePath(site, "waf"), host: site.PrimaryDomain, websiteID: site.ID})
	}
	return out
}

func wafLogMatches(item map[string]any, filters map[string]string) bool {
	for key, value := range filters {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch key {
		case "keyword":
			data, _ := json.Marshal(item)
			if !strings.Contains(strings.ToLower(string(data)), strings.ToLower(value)) {
				return false
			}
		case "host", "site":
			if !logFieldContains(item, value, "host", "site") {
				return false
			}
		case "ip", "clientIP":
			if !logFieldContains(item, value, "client_ip", "clientIP", "ip") {
				return false
			}
		case "ipRegion", "ip_region":
			if !logFieldContains(item, value, "ipRegion", "ip_region", "region", "country") {
				return false
			}
		case "rule", "ruleID":
			if !logFieldContains(item, value, "rule", "ruleID", "rule_id") {
				return false
			}
		case "websiteID", "websiteId":
			got := firstLogValue(item, "websiteID", "websiteId")
			if got != value {
				return false
			}
		case "status":
			if strconv.Itoa(wafLogStatus(item)) != value {
				return false
			}
		case "startTime", "endTime":
			when := wafLogTime(item)
			if when.IsZero() {
				return false
			}
			bound, err := parseWAFTime(value)
			if err != nil {
				continue
			}
			if key == "startTime" && when.Before(bound) {
				return false
			}
			if key == "endTime" && when.After(bound) {
				return false
			}
		default:
			if !strings.Contains(strings.ToLower(fmt.Sprint(item[key])), strings.ToLower(value)) {
				return false
			}
		}
	}
	return true
}

func logFieldContains(item map[string]any, value string, keys ...string) bool {
	for _, key := range keys {
		if strings.Contains(strings.ToLower(fmt.Sprint(item[key])), strings.ToLower(value)) {
			return true
		}
	}
	return false
}

func firstLogValue(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fmt.Sprint(item[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func wafLogStatus(item map[string]any) int {
	for _, key := range []string{"status", "statusCode", "status_code"} {
		if value, ok := item[key]; ok {
			switch raw := value.(type) {
			case float64:
				return int(raw)
			case int:
				return raw
			default:
				result, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(raw)))
				return result
			}
		}
	}
	return 0
}

func wafLogIsIntercept(item map[string]any) bool {
	action := strings.ToLower(firstLogValue(item, "action", "disruptiveAction"))
	if action == "block" || action == "deny" || action == "drop" {
		return true
	}
	if fmt.Sprint(item["disruptive"]) == "true" {
		return true
	}
	status := wafLogStatus(item)
	return status == 403 || status == 429
}

func wafLogTime(item map[string]any) time.Time {
	for _, key := range []string{"timestamp", "time", "createdAt", "created_at"} {
		value := strings.TrimSpace(fmt.Sprint(item[key]))
		if value == "" || value == "<nil>" {
			continue
		}
		if parsed, err := parseWAFTime(value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func wafLogDay(item map[string]any) (string, bool) {
	when := wafLogTime(item)
	if when.IsZero() {
		return "", false
	}
	return when.UTC().Format("2006-01-02"), true
}

func parseWAFTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if numeric, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(numeric, 0).UTC(), nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid WAF time %q", value)
}
