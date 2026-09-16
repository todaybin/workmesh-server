// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"fmt"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
)

func handleImageSearch(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	rows, err := dockerJSONLines(r, "image", "ls", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	keyword := strings.ToLower(valueString(req, "name", "info"))
	byID := map[string]map[string]any{}
	used := map[string]bool{}
	containerRows, _ := dockerJSONLines(r, "ps", "-a", "--format", "{{json .}}")
	for _, row := range containerRows {
		used[strings.ToLower(valueString(row, "Image"))] = true
	}
	for _, row := range rows {
		id := valueString(row, "ID", "Id")
		if id == "" {
			continue
		}
		repository, tag := valueString(row, "Repository"), valueString(row, "Tag")
		label := repository
		if tag != "" && tag != "<none>" {
			label += ":" + tag
		}
		if keyword != "" && !strings.Contains(strings.ToLower(label), keyword) {
			continue
		}
		item := byID[id]
		if item == nil {
			item = map[string]any{"id": id, "tags": []string{}, "size": int64(parseDockerBytes(valueString(row, "Size"))), "createdAt": parseDockerCreated(valueString(row, "CreatedAt")), "isUsed": used[strings.ToLower(label)]}
			byID[id] = item
		}
		if label != "" && !strings.Contains(label, "<none>") {
			item["tags"] = append(item["tags"].([]string), label)
		}
	}
	store := getContainerStore()
	items := make([]map[string]any, 0, len(byID))
	for id, item := range byID {
		description, pinned := resourceDescription(store, "image", strings.TrimPrefix(id, "sha256:"))
		item["description"], item["isPinned"] = description, pinned
		items = append(items, item)
	}
	orderBy, order := valueString(req, "orderBy"), valueString(req, "order")
	sort.SliceStable(items, func(i, j int) bool {
		less := fmt.Sprint(items[i]["createdAt"]) < fmt.Sprint(items[j]["createdAt"])
		if orderBy == "size" {
			less = items[i]["size"].(int64) < items[j]["size"].(int64)
		} else if orderBy == "tags" {
			less = fmt.Sprint(items[i]["tags"]) < fmt.Sprint(items[j]["tags"])
		} else if orderBy == "isUsed" {
			less = !items[i]["isUsed"].(bool) && items[j]["isUsed"].(bool)
		}
		if strings.EqualFold(order, "descending") {
			return !less
		}
		return less
	})
	pageItems, page, pageSize := paginateMaps(items, intValue(req, "page"), intValue(req, "pageSize"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": pageItems, "total": len(items), "page": page, "pageSize": pageSize}})
}

func handleImageOptions(w http.ResponseWriter, r *http.Request) {
	rows, err := dockerJSONLines(r, "image", "ls", "--format", "{{json .}}")
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	seen := map[string]bool{}
	items := make([]map[string]any, 0)
	for _, row := range rows {
		repo, tag := valueString(row, "Repository"), valueString(row, "Tag")
		if repo == "" || tag == "<none>" {
			continue
		}
		name := repo + ":" + tag
		if !seen[name] {
			seen[name] = true
			items = append(items, map[string]any{"option": name})
		}
	}
	sort.Slice(items, func(i, j int) bool { return fmt.Sprint(items[i]["option"]) < fmt.Sprint(items[j]["option"]) })
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
}

func handleImageAll(w http.ResponseWriter, r *http.Request) {
	req := map[string]any{"page": 1, "pageSize": 10000, "orderBy": "createdAt", "order": "descending"}
	body, _ := json.Marshal(req)
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(strings.NewReader(string(body)))
	result := httptest.NewRecorder()
	handleImageSearch(result, r2)
	var envelope map[string]any
	if json.Unmarshal(result.Body.Bytes(), &envelope) != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": "镜像列表解析失败"})
		return
	}
	data, _ := envelope["data"].(map[string]any)
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": data["items"]})
}

func handleResourceOptions(w http.ResponseWriter, r *http.Request, resource string) {
	rows, err := dockerJSONLines(r, resource, "ls", "--format", "{{json .}}")
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		name := valueString(row, "Name")
		if name != "" {
			items = append(items, map[string]any{"option": name})
		}
	}
	sort.Slice(items, func(i, j int) bool { return fmt.Sprint(items[i]["option"]) < fmt.Sprint(items[j]["option"]) })
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": items})
}

func handleNetworkSearch(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	rows, err := dockerJSONLines(r, "network", "ls", "--no-trunc", "--format", "{{json .}}")
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	keyword := strings.ToLower(valueString(req, "info", "name"))
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		name := valueString(row, "Name")
		if keyword != "" && !strings.Contains(strings.ToLower(name), keyword) {
			continue
		}
		inspect, inspectErr := dockerJSONLines(r, "network", "inspect", name, "--format", "{{json .}}")
		if inspectErr != nil || len(inspect) == 0 {
			continue
		}
		detail := inspect[0]
		ipam, _ := detail["IPAM"].(map[string]any)
		configs, _ := ipam["Config"].([]any)
		subnet, gateway := "", ""
		if len(configs) > 0 {
			if c, ok := configs[0].(map[string]any); ok {
				subnet, gateway = valueString(c, "Subnet"), valueString(c, "Gateway")
			}
		}
		labels := make([]string, 0)
		if values, ok := detail["Labels"].(map[string]any); ok {
			for k, v := range values {
				labels = append(labels, k+"="+fmt.Sprint(v))
			}
			sort.Strings(labels)
		}
		created := parseDockerCreated(valueString(detail, "Created"))
		items = append(items, map[string]any{"id": valueString(detail, "Id", "ID"), "name": name, "labels": labels, "driver": valueString(detail, "Driver"), "ipamDriver": valueString(ipam, "Driver"), "subnet": subnet, "gateway": gateway, "createdAt": created, "attachable": dockerBoolValue(detail, "Attachable")})
	}
	pageItems, page, pageSize := paginateMaps(items, intValue(req, "page"), intValue(req, "pageSize"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": pageItems, "total": len(items), "page": page, "pageSize": pageSize}})
}

func handleVolumeSearch(w http.ResponseWriter, r *http.Request) {
	req, err := requestMap(r)
	if err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	rows, err := dockerJSONLines(r, "volume", "ls", "--format", "{{json .}}")
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	keyword := strings.ToLower(valueString(req, "info", "name"))
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		name := valueString(row, "Name")
		if keyword != "" && !strings.Contains(strings.ToLower(name), keyword) {
			continue
		}
		inspect, inspectErr := dockerJSONLines(r, "volume", "inspect", name, "--format", "{{json .}}")
		if inspectErr != nil || len(inspect) == 0 {
			continue
		}
		detail := inspect[0]
		labels := make([]map[string]any, 0)
		if values, ok := detail["Labels"].(map[string]any); ok {
			for k, v := range values {
				labels = append(labels, map[string]any{"key": k, "value": fmt.Sprint(v)})
			}
		}
		options := make([]map[string]any, 0)
		if values, ok := detail["Options"].(map[string]any); ok {
			for k, v := range values {
				options = append(options, map[string]any{"key": k, "value": fmt.Sprint(v)})
			}
		}
		items = append(items, map[string]any{"name": name, "labels": labels, "driver": valueString(detail, "Driver"), "mountpoint": valueString(detail, "Mountpoint"), "createdAt": parseDockerCreated(valueString(detail, "CreatedAt")), "options": options})
	}
	pageItems, page, pageSize := paginateMaps(items, intValue(req, "page"), intValue(req, "pageSize"))
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": pageItems, "total": len(items), "page": page, "pageSize": pageSize}})
}
