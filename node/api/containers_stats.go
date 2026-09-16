// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// handleContainerItemStats 查询单个容器或 Docker 全局资源的磁盘占用统计。
func handleContainerItemStats(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	name := valueString(req, "name", "id")
	if name == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_NAME"}, "message": "容器名称不能为空"})
		return
	}
	if name != "system" {
		rows, runErr := dockerJSONLines(r, "inspect", "--size", "--format", "{{json .}}", name)
		if runErr != nil || len(rows) == 0 {
			if runErr == nil {
				runErr = errors.New("容器不存在")
			}
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": runErr.Error()})
			return
		}
		item := rows[0]
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"sizeRw": numberValue(item, "SizeRw"), "sizeRootFs": numberValue(item, "SizeRootFs")}})
		return
	}
	rows, runErr := dockerJSONLines(r, "system", "df", "--format", "{{json .}}")
	if runErr != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": runErr.Error()})
		return
	}
	data := map[string]any{"containerUsage": int64(0), "containerReclaimable": int64(0), "imageUsage": int64(0), "imageReclaimable": int64(0), "volumeUsage": int64(0), "volumeReclaimable": int64(0), "buildCacheUsage": int64(0), "buildCacheReclaimable": int64(0)}
	for _, row := range rows {
		typ := strings.ToLower(valueString(row, "Type"))
		size := int64(parseDockerBytes(valueString(row, "Size")))
		reclaim := int64(parseDockerBytes(strings.TrimSpace(strings.Split(valueString(row, "Reclaimable"), "(")[0])))
		switch typ {
		case "images":
			data["imageUsage"] = data["imageUsage"].(int64) + size
			data["imageReclaimable"] = data["imageReclaimable"].(int64) + reclaim
		case "containers":
			data["containerUsage"] = data["containerUsage"].(int64) + size
			data["containerReclaimable"] = data["containerReclaimable"].(int64) + reclaim
		case "local volumes", "volumes":
			data["volumeUsage"] = data["volumeUsage"].(int64) + size
			data["volumeReclaimable"] = data["volumeReclaimable"].(int64) + reclaim
		case "build cache":
			data["buildCacheUsage"] = data["buildCacheUsage"].(int64) + size
			data["buildCacheReclaimable"] = data["buildCacheReclaimable"].(int64) + reclaim
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": data})
}

// numberValue 将 Docker JSON 中可能出现的数值类型统一转换为 int64。
func numberValue(item map[string]any, key string) int64 {
	switch value := item[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case json.Number:
		v, _ := value.Int64()
		return v
	}
	return 0
}

// dockerBoolValue 读取 Docker JSON 对象中的布尔字段，缺失或类型不符时返回 false。
func dockerBoolValue(item map[string]any, key string) bool {
	value, _ := item[key].(bool)
	return value
}

// handleContainerInspect 根据对象类型返回容器、镜像、网络、卷或 Compose 的真实详情。
func handleContainerInspect(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	id, typ, detail := valueString(req, "id", "name"), strings.ToLower(valueString(req, "type")), valueString(req, "detail")
	if id == "" || typ == "" {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "id 和 type 不能为空"})
		return
	}
	if typ == "compose" {
		store := getContainerStore()
		store.mu.RLock()
		records := append([]composeRecord(nil), store.state.Composes...)
		store.mu.RUnlock()
		for _, record := range records {
			if record.Name == id || record.Path == id {
				path := record.Path
				if detail != "" {
					path = detail
				}
				content, readErr := os.ReadFile(path)
				if readErr != nil {
					wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": readErr.Error()})
					return
				}
				wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": string(content)})
				return
			}
		}
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "Compose 不存在"})
		return
	}
	if !validDockerIdentifier(id) {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "对象标识无效"})
		return
	}
	// 使用逐行 JSON 格式，避免 Docker inspect 默认数组响应被错误解析为对象。
	rows, runErr := dockerJSONLines(r, "inspect", "--format", "{{json .}}", id)
	if typ == "image" {
		rows, runErr = dockerJSONLines(r, "image", "inspect", id)
	} else if typ == "network" {
		rows, runErr = dockerJSONLines(r, "network", "inspect", id)
	} else if typ == "volume" {
		rows, runErr = dockerJSONLines(r, "volume", "inspect", id)
	}
	if runErr != nil || len(rows) == 0 {
		if runErr == nil {
			runErr = errors.New("对象不存在")
		}
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": runErr.Error()})
		return
	}
	b, _ := json.Marshal(rows[0])
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": string(b)})
}

// handleContainerListStats 调用 Docker stats 获取所有运行容器的即时资源使用情况。
func handleContainerListStats(w http.ResponseWriter, r *http.Request) {
	result, err := runDocker(r, "stats", "--no-stream", "--no-trunc", "--format", "{{json .}}")
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" && err != nil {
			message = err.Error()
		}
		status := http.StatusInternalServerError
		if isDockerUnavailable(result, err) {
			status = http.StatusServiceUnavailable
		}
		wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": message})
		return
	}
	items := make([]map[string]any, 0)
	for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			wmhttp.JSON(w, http.StatusInternalServerError, map[string]any{"code": "ERR", "message": fmt.Sprintf("解析 Docker 统计失败: %v", err)})
			return
		}
		usage, limit := parseDockerMemoryUsage(valueString(raw, "MemUsage"))
		items = append(items, map[string]any{
			"containerID": valueString(raw, "ID", "Container"), "cpuTotalUsage": 0, "systemUsage": 0,
			"cpuPercent": parseDockerPercent(valueString(raw, "CPUPerc")), "percpuUsage": 0, "memoryCache": 0,
			"memoryUsage": usage, "memoryLimit": limit, "memoryPercent": parseDockerPercent(valueString(raw, "MemPerc")),
		})
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": items})
}

// parseDockerPercent 将 Docker 输出中的百分数字符串转换为浮点数。
func parseDockerPercent(value string) float64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")), 64)
	return parsed
}

// parseDockerMemoryUsage 拆分 Docker 的“已用/限制”内存字段并转换为字节数。
func parseDockerMemoryUsage(value string) (uint64, uint64) {
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return parseDockerBytes(parts[0]), parseDockerBytes(parts[1])
}

// parseDockerBytes 按 Docker 支持的二进制或十进制单位将容量字符串转换为字节数。
func parseDockerBytes(value string) uint64 {
	value = strings.TrimSpace(value)
	units := []struct {
		suffix string
		factor float64
	}{{"TiB", 1 << 40}, {"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"B", 1}}
	for _, unit := range units {
		if strings.HasSuffix(value, unit.suffix) {
			number, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, unit.suffix)), 64)
			if number > 0 {
				return uint64(number * unit.factor)
			}
			return 0
		}
	}
	return 0
}

// countNonEmptyLines 统计命令输出中去除空白后的有效行数。
func countNonEmptyLines(value string) int {
	count := 0
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
