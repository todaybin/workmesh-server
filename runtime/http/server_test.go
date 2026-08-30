// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONSanitizesInternalStateNames(t *testing.T) {
	recorder := httptest.NewRecorder()
	JSON(recorder, http.StatusNotImplemented, map[string]any{
		"code": "ERR",
		"details": map[string]string{"errCode": "MIGRATION_PENDING"},
		"message": "该接口正在迁移",
		"items": []any{"MIGRATION_PENDING"},
	})
	if strings.Contains(recorder.Body.String(), "MIGRATION_PENDING") || strings.Contains(recorder.Body.String(), "正在迁移") {
		t.Fatalf("响应暴露了内部状态名称: %s", recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是有效 JSON: %v", err)
	}
	if body["message"] != "该功能暂不可用" {
		t.Fatalf("消息未规范化: %#v", body["message"])
	}
	details, ok := body["details"].(map[string]any)
	if !ok || details["errCode"] != "FEATURE_UNAVAILABLE" {
		t.Fatalf("错误码未规范化: %#v", body["details"])
	}
}

func TestJSONDoesNotMutateInput(t *testing.T) {
	input := map[string]any{"details": map[string]string{"errCode": "MIGRATION_PENDING"}}
	recorder := httptest.NewRecorder()
	JSON(recorder, http.StatusBadRequest, input)
	if input["details"].(map[string]string)["errCode"] != "MIGRATION_PENDING" {
		t.Fatal("JSON 不应修改调用方数据")
	}
}
