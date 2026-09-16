// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDecodeSingleJSONRejectsTrailingValue 验证公共解析器拒绝尾随 JSON 值。
func TestDecodeSingleJSONRejectsTrailingValue(t *testing.T) {
	var value map[string]any
	if err := decodeSingleJSON(strings.NewReader(`{"ok":true} {"extra":true}`), &value, 1024); err == nil {
		t.Fatal("尾随 JSON 未被拒绝")
	}
}

// TestDecodeJSONRejectsTrailingValue 验证主机/容器共用解析入口的边界行为。
func TestDecodeJSONRejectsTrailingValue(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v2/hosts/tool/status", strings.NewReader(`{"type":"supervisor"}{}`))
	var value map[string]any
	if err := decodeJSON(r, &value); err == nil {
		t.Fatal("decodeJSON 接受了尾随 JSON")
	}
}

// TestRequestMapJSONBoundaries 验证设置/功能域公共解析器的空体兼容、错误 JSON 和尾随 JSON 行为。
func TestRequestMapJSONBoundaries(t *testing.T) {
	empty := httptest.NewRequest(http.MethodPost, "/api/v2/settings/update", nil)
	value, err := requestMap(empty)
	if err != nil || value == nil {
		t.Fatalf("空体应兼容为空对象: value=%#v err=%v", value, err)
	}
	if _, err := requestMap(httptest.NewRequest(http.MethodPost, "/api/v2/settings/update", strings.NewReader(`{"key":`))); err == nil {
		t.Fatal("requestMap 接受了 malformed JSON")
	}
	if _, err := requestMap(httptest.NewRequest(http.MethodPost, "/api/v2/settings/update", strings.NewReader(`{"key":"a"}{"key":"b"}`))); err == nil {
		t.Fatal("requestMap 接受了尾随 JSON")
	}
	unknown, err := requestMap(httptest.NewRequest(http.MethodPost, "/api/v2/settings/update", strings.NewReader(`{"futureField":true}`)))
	if err != nil || unknown["futureField"] != true {
		t.Fatalf("未知字段应保持兼容: value=%#v err=%v", unknown, err)
	}
}

// TestSettingsMalformedJSONUsesV2ErrorEnvelope 验证公共设置路由把解析错误转换为标准 v2 错误 envelope。
func TestSettingsMalformedJSONUsesV2ErrorEnvelope(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)
	for name, body := range map[string]string{
		"malformed": `{"key":`,
		"trailing":  `{"key":"a"}{"key":"b"}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v2/settings/update", strings.NewReader(body)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("解析错误应返回 400: status=%d body=%s", response.Code, response.Body.String())
			}
			var envelope struct {
				Code    string            `json:"code"`
				Details map[string]string `json:"details"`
			}
			if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Code != "ERR" || envelope.Details["errCode"] != "INVALID_JSON" {
				t.Fatalf("错误 envelope 不符合 v2 契约: %#v", envelope)
			}
		})
	}
}
