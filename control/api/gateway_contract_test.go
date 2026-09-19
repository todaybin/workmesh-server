// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	linkpkg "github.com/todaybin/workmesh-server/runtime/link"
	"github.com/todaybin/workmesh-server/runtime/machineid"
)

func TestGatewayV2RegistrationIdempotenceAuthorizationAndCSRF(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	t.Setenv("WORKMESH_GATEWAY_ALLOW_HTTP", "1")
	oldGatewayDB := gatewayDB
	gatewayDB = nil
	t.Cleanup(func() { gatewayDB = oldGatewayDB })

	var registerCalls atomic.Int32
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workmesh/node/register" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		registerCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"bindingId": "binding-local", "token": "must-not-leak"}})
	}))
	defer cloud.Close()

	authorize := func(r *http.Request) bool {
		cookie, err := r.Cookie(sessionCookieName)
		return err == nil && cookie.Value == "session-local"
	}
	mux := http.NewServeMux()
	RegisterGatewayRoutes(mux, "node-local", "primary", authorize)
	handler := NewSecurityMiddleware(mux, SecurityMiddlewareOptions{DataDir: dataDir, Authorize: authorize})
	body := `{"gatewayUrl":"` + cloud.URL + `","nodeId":"node-local","registrationToken":"registration-local"}`

	unauthorized := performGatewayRequest(handler, body, false, false)
	assertGatewayError(t, unauthorized, http.StatusUnauthorized, "LOCAL_AUTH_REQUIRED")
	withoutCSRF := performGatewayRequest(handler, body, true, false)
	assertGatewayError(t, withoutCSRF, http.StatusForbidden, "CSRF_INVALID")

	first := performGatewayRequest(handler, body, true, true)
	fingerprint, err := machineid.Current()
	if err != nil {
		t.Fatal(err)
	}
	deviceID := gatewayDeviceID(fingerprint.Code)
	assertGatewayRegistered(t, first, deviceID, "binding-local")
	if strings.Contains(first.Body.String(), "must-not-leak") || strings.Contains(first.Body.String(), "registration-local") {
		t.Fatalf("注册响应泄露 Gateway 凭据: %s", first.Body.String())
	}
	second := performGatewayRequest(handler, body, true, true)
	assertGatewayRegistered(t, second, deviceID, "binding-local")
	if registerCalls.Load() != 1 {
		t.Fatalf("重复注册调用云端次数=%d, want 1", registerCalls.Load())
	}

	conflictBody := `{"gatewayUrl":"` + cloud.URL + `","nodeId":"other-node","registrationToken":"registration-local"}`
	ignoredAlias := performGatewayRequest(handler, conflictBody, true, true)
	assertGatewayRegistered(t, ignoredAlias, deviceID, "binding-local")
	if registerCalls.Load() != 1 {
		t.Fatalf("客户端 nodeId 不应创建第二个 Gateway 设备，调用次数=%d", registerCalls.Load())
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/v2/workmesh/gateway/status", nil)
	statusRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-local"})
	status := httptest.NewRecorder()
	handler.ServeHTTP(status, statusRequest)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"code":200`) || strings.Contains(status.Body.String(), "must-not-leak") || strings.Contains(status.Body.String(), "registration-local") {
		t.Fatalf("Gateway 状态响应异常或泄露凭据: status=%d body=%s", status.Code, status.Body.String())
	}
}

// TestGatewayV2LinkRejectsStaleEpochAndMismatchedSignedNode 验证节点链路的 fencing、签名身份和错误 envelope。
func TestGatewayV2LinkRejectsStaleEpochAndMismatchedSignedNode(t *testing.T) {
	const secret = "gateway-contract-signing-secret"
	t.Setenv("WORKMESH_LINK_SECRET", secret)
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	RegisterLinkRoutes(mux, "node-local", "primary", NewRoleManager("node-local", "primary"))

	stale := `{"operationId":"stale-epoch","nodeId":"node-local","to":"secondary","expectedEpoch":0}`
	response := performSignedLinkRequest(t, mux, "/api/v2/link/fencing/check", []byte(stale), secret, "peer-node", "nonce-stale")
	assertLinkError(t, response, http.StatusConflict, secret)

	// 请求签名有效，但头部节点与正文节点不同，必须拒绝并且不回显密钥或签名材料。
	mismatch := []byte(`{"nodeId":"forged-node","roleEpoch":1,"version":"v2"}`)
	response = performSignedLinkRequest(t, mux, "/api/v2/link/heartbeat", mismatch, secret, "peer-node", "nonce-mismatch")
	assertLinkError(t, response, http.StatusUnauthorized, secret)
}

func performGatewayRequest(handler http.Handler, body string, authenticated, withCSRF bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v2/workmesh/gateway/register", strings.NewReader(body))
	if authenticated {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-local"})
	}
	if withCSRF {
		request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf-local"})
		request.Header.Set(csrfHeaderName, "csrf-local")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertGatewayError(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	var envelope struct {
		Code    string `json:"code"`
		Details struct {
			ErrCode string `json:"errCode"`
		} `json:"details"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if response.Code != wantStatus || envelope.Code != "ERR" || envelope.Details.ErrCode != wantCode {
		t.Fatalf("错误响应 status=%d envelope=%+v", response.Code, envelope)
	}
}

func assertGatewayRegistered(t *testing.T, response *httptest.ResponseRecorder, nodeID, bindingID string) {
	t.Helper()
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Registered bool   `json:"registered"`
			NodeID     string `json:"nodeId"`
			BindingID  string `json:"bindingId"`
			Status     string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || envelope.Code != 200 || !envelope.Data.Registered || envelope.Data.NodeID != nodeID || envelope.Data.BindingID != bindingID || envelope.Data.Status != "running" {
		t.Fatalf("注册响应不稳定: status=%d envelope=%+v body=%s", response.Code, envelope, response.Body.String())
	}
}

func performSignedLinkRequest(t *testing.T, handler http.Handler, path string, body []byte, secret, nodeID, nonce string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	request.Header.Set(linkpkg.HeaderNodeID, nodeID)
	request.Header.Set(linkpkg.HeaderTimestamp, timestamp)
	request.Header.Set(linkpkg.HeaderNonce, nonce)
	request.Header.Set(linkpkg.HeaderSignature, linkpkg.Sign([]byte(secret), request.Method, request.URL.RequestURI(), timestamp, nonce, body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertLinkError(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, secret string) {
	t.Helper()
	var envelope struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if response.Code != wantStatus || envelope.Code != "ERR" {
		t.Fatalf("链路错误 envelope 异常: status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), secret) || strings.Contains(response.Body.String(), "X-WorkMesh-Signature") {
		t.Fatalf("链路错误响应泄露签名材料: %s", response.Body.String())
	}
}
