// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/control/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var localCore = service.NewCoreService()

func coreToken() string {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err == nil {
		return hex.EncodeToString(raw)
	}
	return time.Now().UTC().Format("20060102150405.000000000")
}

// registerCoreAuthExtras 注册 MFA、Passkey 和 API 密钥的兼容接口。
// 当前默认关闭外部认证因子，但接口保持幂等并返回稳定 envelope，便于后续接入硬件/云端提供商。
func registerCoreAuthExtras(mux *http.ServeMux) {
	// 可选外部身份源默认关闭，但仍返回契约化状态，前端可据此隐藏入口而非触发 404。
	mux.HandleFunc("GET /api/v2/core/auth/ldap/status", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, map[string]any{"enabled": false}) })
	mux.HandleFunc("GET /api/v2/core/auth/oidc/status", func(w http.ResponseWriter, _ *http.Request) {
		coreJSON(w, map[string]any{"enabled": false, "displayName": "", "authorizationCode": false})
	})
	mux.HandleFunc("POST /api/v2/core/auth/oidc/begin", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, map[string]any{"authorizationURL": ""}) })
	mux.HandleFunc("POST /api/v2/core/auth/oidc/finish", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotImplemented, errors.New("OIDC 未启用"))
	})
	mux.HandleFunc("GET /api/v2/core/auth/saml2/status", func(w http.ResponseWriter, _ *http.Request) {
		coreJSON(w, map[string]any{"enabled": false, "displayName": "", "syncLogout": false})
	})
	mux.HandleFunc("POST /api/v2/core/auth/saml2/begin", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, map[string]any{"navigation": nil}) })
	mux.HandleFunc("POST /api/v2/core/auth/saml2/finish", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotImplemented, errors.New("SAML2 未启用"))
	})
	mux.HandleFunc("GET /api/v2/core/auth/passkey/list", handleCorePasskeyList)
	mux.HandleFunc("POST /api/v2/core/auth/api/generate", handleCoreAPIGenerate)
	mux.HandleFunc("POST /api/v2/core/auth/api/update", handleCoreAPIUpdate)
	mux.HandleFunc("POST /api/v2/core/auth/current/update", handleCoreCurrentUpdate)
	mux.HandleFunc("POST /api/v2/core/auth/expired/reset", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, nil) })
	mux.HandleFunc("POST /api/v2/core/auth/mfa", func(w http.ResponseWriter, _ *http.Request) {
		coreJSON(w, map[string]string{"secret": "", "qrImage": ""})
	})
	mux.HandleFunc("POST /api/v2/core/auth/mfa/bind", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, nil) })
	mux.HandleFunc("POST /api/v2/core/auth/mfa/close", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, nil) })
	mux.HandleFunc("POST /api/v2/core/auth/mfalogin", handleCoreLogin)
	mux.HandleFunc("POST /api/v2/core/auth/passkey/begin", handleCorePasskeyBegin)
	mux.HandleFunc("POST /api/v2/core/auth/passkey/del", handleCorePasskeyDelete)
	mux.HandleFunc("POST /api/v2/core/auth/passkey/finish", handleCoreLogin)
	mux.HandleFunc("POST /api/v2/core/auth/passkey/register/begin", handleCorePasskeyBegin)
	mux.HandleFunc("POST /api/v2/core/auth/passkey/register/finish", handleCorePasskeyRegisterFinish)
}

func handleCorePasskeyList(w http.ResponseWriter, r *http.Request) {
	if _, err := localCore.Current(coreSessionID(r)); err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	coreJSON(w, localCore.ListPasskeys())
}

func handleCorePasskeyBegin(w http.ResponseWriter, r *http.Request) {
	authSession := coreSessionID(r)
	challenge, err := localCore.BeginPasskeyRegistration(authSession)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	coreJSON(w, map[string]any{"sessionId": challenge, "publicKey": map[string]any{"challenge": challenge, "timeout": 300000, "rp": map[string]string{"name": "WorkMesh"}}})
}

func handleCorePasskeyDelete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		var body struct {
			ID string `json:"id"`
		}
		if err := decodeJSON(r, &body); err == nil {
			id = strings.TrimSpace(body.ID)
		}
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("Passkey id 不能为空"))
		return
	}
	if err := localCore.DeletePasskey(coreSessionID(r), id); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	coreJSON(w, nil)
}

func handleCorePasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	authSession := coreSessionID(r)
	challengeID := strings.TrimSpace(r.Header.Get("Passkey-Session"))
	var body struct {
		SessionID    string `json:"sessionId"`
		CredentialID string `json:"credentialId"`
		Name         string `json:"name"`
	}
	if r.Body != nil {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if challengeID == "" {
		challengeID = strings.TrimSpace(body.SessionID)
	}
	item, err := localCore.FinishPasskeyRegistration(authSession, challengeID, body.CredentialID, body.Name)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrUnauthenticated) {
			status = http.StatusUnauthorized
		}
		writeError(w, status, err)
		return
	}
	coreJSON(w, item)
}

func coreJSON(w http.ResponseWriter, value any) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": value})
}

func handleCoreLogin(w http.ResponseWriter, r *http.Request) {
	var req struct{ Name, Password string }
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, session, err := localCore.Login(req.Name, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "workmesh_session", Value: session.ID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	// 同时返回旧前端使用的扁平字段和会话对象，确保新旧客户端均可登录。
	coreJSON(w, map[string]any{"name": user.Name, "role": user.Role, "token": session.ID, "mfaStatus": "disabled", "mfaSession": "", "user": user, "session": session})
}

func handleCoreLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("workmesh_session"); err == nil {
		localCore.Logout(cookie.Value)
	}
	coreJSON(w, nil)
}
func handleCoreCurrent(w http.ResponseWriter, r *http.Request) {
	sessionID := coreSessionID(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, service.ErrUnauthenticated)
		return
	}
	user, err := localCore.Current(sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	apiConfig, _ := localCore.APIConfig(sessionID)
	coreJSON(w, map[string]any{"id": user.ID, "name": user.Name, "mfaStatus": "disabled", "mfaInterval": 30, "complexitySetting": "medium", "authSource": "local", "authSourceStatus": "enabled", "apiInterfaceStatus": boolString(apiConfig.Enabled), "apiKey": apiConfig.Key, "ipWhiteList": apiConfig.IPWhiteList, "apiTrustedProxies": apiConfig.TrustedProxy, "apiKeyValidityTime": apiConfig.ValidityHours, "role": user.Role, "permissions": []string{"*"}, "masterOnlyPermissions": []string{}, "nodeRoles": []any{}})
}

// coreSessionID 从 Cookie 或 Bearer/API Key 头中读取会话标识。
func coreSessionID(r *http.Request) string {
	if cookie, err := r.Cookie("workmesh_session"); err == nil && strings.TrimSpace(cookie.Value) != "" {
		return strings.TrimSpace(cookie.Value)
	}
	for _, header := range []string{"Authorization", "X-WorkMesh-Token", "X-API-Key"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if strings.HasPrefix(strings.ToLower(value), "bearer ") {
			value = strings.TrimSpace(value[len("bearer "):])
		}
		if value != "" {
			return value
		}
	}
	return ""
}

func boolString(value bool) string {
	if value {
		return "enable"
	}
	return "disable"
}

func handleCoreCurrentUpdate(w http.ResponseWriter, r *http.Request) {
	sessionID := coreSessionID(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, service.ErrUnauthenticated)
		return
	}
	var request struct {
		Name        string `json:"name"`
		Password    string `json:"password"`
		OldPassword string `json:"oldPassword"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, err := localCore.UpdateCurrentUser(sessionID, request.Name, request.OldPassword, request.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	coreJSON(w, user)
}

func handleCoreAPIGenerate(w http.ResponseWriter, r *http.Request) {
	sessionID := coreSessionID(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, service.ErrUnauthenticated)
		return
	}
	key, err := localCore.GenerateAPIKey(sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	coreJSON(w, key)
}

func handleCoreAPIUpdate(w http.ResponseWriter, r *http.Request) {
	sessionID := coreSessionID(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, service.ErrUnauthenticated)
		return
	}
	var request struct {
		Enabled            any    `json:"apiInterfaceStatus"`
		APIKey             string `json:"apiKey"`
		IPWhiteList        string `json:"ipWhiteList"`
		APITrustedProxies  string `json:"apiTrustedProxies"`
		APIKeyValidityTime int    `json:"apiKeyValidityTime"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	enabled := false
	switch value := request.Enabled.(type) {
	case bool:
		enabled = value
	case string:
		enabled = strings.EqualFold(value, "enable") || strings.EqualFold(value, "enabled") || value == "1" || strings.EqualFold(value, "true")
	}
	err := localCore.UpdateAPIConfig(sessionID, service.APIConfig{Enabled: enabled, Key: request.APIKey, IPWhiteList: request.IPWhiteList, TrustedProxy: request.APITrustedProxies, ValidityHours: request.APIKeyValidityTime})
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	coreJSON(w, nil)
}
func handleCoreCaptcha(w http.ResponseWriter, _ *http.Request) {
	coreJSON(w, map[string]any{"captchaID": "disabled", "required": false})
}
func handleCoreWelcome(w http.ResponseWriter, _ *http.Request) {
	coreJSON(w, map[string]string{"status": "ready"})
}
func handleCoreAuthSetting(w http.ResponseWriter, _ *http.Request) {
	coreJSON(w, map[string]any{"mfa": false, "passkey": false})
}
func handleCoreGroups(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/search") {
		coreJSON(w, localCore.Groups())
		return
	}
	var req struct{ ID, Name, Type string }
	_ = decodeJSON(r, &req)
	if strings.HasSuffix(r.URL.Path, "/del") {
		if err := localCore.DeleteGroup(req.ID); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		coreJSON(w, nil)
		return
	}
	coreJSON(w, localCore.UpsertGroup(req.ID, req.Name, req.Type))
}
func handleCoreSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || strings.HasSuffix(r.URL.Path, "/search") || strings.HasSuffix(r.URL.Path, "/search/base") {
		coreJSON(w, localCore.Settings())
		return
	}
	var values map[string]string
	if err := decodeJSON(r, &values); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	localCore.UpdateSettings(values)
	coreJSON(w, localCore.Settings())
}
