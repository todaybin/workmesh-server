// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// registerWebsiteTemplateRoutes 注册模板及产物的关系化 API，覆盖通配扩展处理器。
func registerWebsiteTemplateRoutes(mux *http.ServeMux, repo *websiteTemplateRepository) {
	mux.HandleFunc("POST /api/v2/websites/templates/search", func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeExtension(r)
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		page, size := normalizeTemplatePage(int(bodyNumber(body, "page")), int(bodyNumber(body, "pageSize")))
		total, items, err := repo.searchTemplates(bodyString(body, "name", "keyword"), bodyString(body, "type"), page, size)
		if err != nil {
			websiteTemplateRouteError(w, websiteTemplateErrorStatus(err), err)
			return
		}
		extensionJSON(w, map[string]any{"total": total, "items": items, "page": page, "pageSize": size})
	})

	mux.HandleFunc("POST /api/v2/websites/templates", func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeExtension(r)
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		item, err := repo.createTemplate(websiteTemplateInputFromBody(body))
		if err != nil {
			websiteTemplateRouteError(w, websiteTemplateErrorStatus(err), err)
			return
		}
		extensionJSON(w, item)
	})

	mux.HandleFunc("POST /api/v2/websites/templates/update", func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeExtension(r)
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		item, err := repo.updateTemplate(websiteTemplateInputFromBody(body))
		if err != nil {
			websiteTemplateRouteError(w, websiteTemplateErrorStatus(err), err)
			return
		}
		extensionJSON(w, item)
	})

	mux.HandleFunc("POST /api/v2/websites/templates/get", func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeExtension(r)
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		id, err := websiteTemplateBodyID(body, "id", "templateID", "templateId")
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		item, err := repo.getTemplate(id)
		if err != nil {
			websiteTemplateRouteError(w, websiteTemplateErrorStatus(err), err)
			return
		}
		extensionJSON(w, item)
	})

	mux.HandleFunc("POST /api/v2/websites/templates/del", func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeExtension(r)
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		id, err := websiteTemplateBodyID(body, "id", "templateID", "templateId")
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		if err := repo.deleteTemplate(id); err != nil {
			websiteTemplateRouteError(w, websiteTemplateErrorStatus(err), err)
			return
		}
		extensionJSON(w, map[string]any{"id": id, "deleted": true})
	})

	mux.HandleFunc("POST /api/v2/websites/templates/preview", func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeExtension(r)
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		id, exists := websiteTemplateBodyIDOptional(body, "templateID", "templateId", "id")
		if !exists {
			latest, latestErr := repo.latestTemplate()
			if latestErr != nil {
				websiteTemplateRouteError(w, websiteTemplateErrorStatus(latestErr), latestErr)
				return
			}
			id = latest.ID
		}
		item, err := repo.previewTemplate(id, websiteTemplateVariableValues(body["variableValues"]))
		if err != nil {
			websiteTemplateRouteError(w, websiteTemplateErrorStatus(err), err)
			return
		}
		extensionJSON(w, item)
	})

	mux.HandleFunc("POST /api/v2/websites/templates/outputs/search", func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeExtension(r)
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		templateID, _ := websiteTemplateBodyIDOptional(body, "templateID", "templateId")
		page, size := normalizeTemplatePage(int(bodyNumber(body, "page")), int(bodyNumber(body, "pageSize")))
		total, items, err := repo.searchOutputs(templateID, page, size)
		if err != nil {
			websiteTemplateRouteError(w, websiteTemplateErrorStatus(err), err)
			return
		}
		extensionJSON(w, map[string]any{"total": total, "items": items, "page": page, "pageSize": size})
	})

	mux.HandleFunc("POST /api/v2/websites/templates/outputs", func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeExtension(r)
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		templateID, err := websiteTemplateBodyID(body, "templateID", "templateId")
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		name := bodyString(body, "name")
		if name == "" {
			name = "output-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		}
		item, err := repo.createOutput(websiteTemplateOutputInput{TemplateID: templateID, Name: name, VariableValues: websiteTemplateVariableValues(body["variableValues"])})
		if err != nil {
			websiteTemplateRouteError(w, websiteTemplateErrorStatus(err), err)
			return
		}
		extensionJSON(w, item)
	})

	mux.HandleFunc("POST /api/v2/websites/templates/outputs/get", func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeExtension(r)
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		id, err := websiteTemplateBodyID(body, "id", "outputID", "outputId")
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		item, err := repo.getOutput(id)
		if err != nil {
			websiteTemplateRouteError(w, websiteTemplateErrorStatus(err), err)
			return
		}
		extensionJSON(w, item)
	})

	mux.HandleFunc("POST /api/v2/websites/templates/outputs/del", func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeExtension(r)
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		id, err := websiteTemplateBodyID(body, "id", "outputID", "outputId")
		if err != nil {
			websiteTemplateRouteError(w, http.StatusBadRequest, err)
			return
		}
		if err := repo.deleteOutput(id); err != nil {
			websiteTemplateRouteError(w, websiteTemplateErrorStatus(err), err)
			return
		}
		extensionJSON(w, map[string]any{"id": id, "deleted": true})
	})
}

// websiteTemplateInputFromBody 将兼容字段转换为模板写入结构。
func websiteTemplateInputFromBody(body map[string]any) websiteTemplateInput {
	id, _ := websiteTemplateBodyIDOptional(body, "id", "templateID", "templateId")
	typ := bodyString(body, "type")
	if typ == "" {
		typ = "single"
	}
	return websiteTemplateInput{
		ID:           id,
		Name:         bodyString(body, "name"),
		Type:         typ,
		Content:      bodyString(body, "content"),
		FilePath:     bodyString(body, "filePath", "file_path"),
		Variables:    bodyString(body, "variables"),
		Remark:       bodyString(body, "remark"),
		HasName:      hasTemplateBodyKey(body, "name"),
		HasType:      hasTemplateBodyKey(body, "type"),
		HasContent:   hasTemplateBodyKey(body, "content"),
		HasFilePath:  hasTemplateBodyKey(body, "filePath", "file_path"),
		HasVariables: hasTemplateBodyKey(body, "variables"),
		HasRemark:    hasTemplateBodyKey(body, "remark"),
	}
}

// hasTemplateBodyKey 判断请求是否明确提交了字段，以支持模板部分更新。
func hasTemplateBodyKey(body map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := body[key]; ok {
			return true
		}
	}
	return false
}

// websiteTemplateVariableValues 将请求中的变量对象转换为模板渲染值。
func websiteTemplateVariableValues(value any) map[string]string {
	result := map[string]string{}
	items, ok := value.(map[string]any)
	if !ok {
		return result
	}
	for key, item := range items {
		result[key] = fmt.Sprint(item)
	}
	return result
}

// websiteTemplateBodyID 从请求中读取必填 ID，兼容数字和字符串。
func websiteTemplateBodyID(body map[string]any, keys ...string) (uint, error) {
	id, ok := websiteTemplateBodyIDOptional(body, keys...)
	if !ok || id == 0 {
		return 0, errors.New("ID 无效")
	}
	return id, nil
}

// websiteTemplateBodyIDOptional 从请求中读取可选 ID。
func websiteTemplateBodyIDOptional(body map[string]any, keys ...string) (uint, bool) {
	for _, key := range keys {
		value, exists := body[key]
		if !exists {
			continue
		}
		switch typed := value.(type) {
		case float64:
			if typed > 0 {
				return uint(typed), true
			}
		case int:
			if typed > 0 {
				return uint(typed), true
			}
		case int64:
			if typed > 0 {
				return uint(typed), true
			}
		case uint:
			if typed > 0 {
				return typed, true
			}
		case uint64:
			if typed > 0 {
				return uint(typed), true
			}
		case string:
			id, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 32)
			if err == nil && id > 0 {
				return uint(id), true
			}
		case json.Number:
			id, err := strconv.ParseUint(string(typed), 10, 32)
			if err == nil && id > 0 {
				return uint(id), true
			}
		}
	}
	return 0, false
}

// websiteTemplateRouteError 使用统一错误外壳，并将不存在记录映射为 404。
func websiteTemplateRouteError(w http.ResponseWriter, status int, err error) {
	extensionError(w, status, err)
}

// websiteTemplateErrorStatus 将数据库查询错误映射为兼容 HTTP 状态码。
func websiteTemplateErrorStatus(err error) int {
	if errors.Is(err, os.ErrNotExist) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
