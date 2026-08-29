// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/rand"
	"encoding/hex"
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
	mux.HandleFunc("GET /api/v2/core/auth/passkey/list", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, []any{}) })
	mux.HandleFunc("POST /api/v2/core/auth/api/generate", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, coreToken()) })
	mux.HandleFunc("POST /api/v2/core/auth/api/update", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, nil) })
	mux.HandleFunc("POST /api/v2/core/auth/current/update", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, nil) })
	mux.HandleFunc("POST /api/v2/core/auth/expired/reset", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, nil) })
	mux.HandleFunc("POST /api/v2/core/auth/mfa", func(w http.ResponseWriter, _ *http.Request) {
		coreJSON(w, map[string]string{"secret": "", "qrImage": ""})
	})
	mux.HandleFunc("POST /api/v2/core/auth/mfa/bind", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, nil) })
	mux.HandleFunc("POST /api/v2/core/auth/mfa/close", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, nil) })
	mux.HandleFunc("POST /api/v2/core/auth/mfalogin", handleCoreLogin)
	mux.HandleFunc("POST /api/v2/core/auth/passkey/begin", func(w http.ResponseWriter, _ *http.Request) {
		coreJSON(w, map[string]any{"sessionId": coreToken(), "publicKey": map[string]any{}})
	})
	mux.HandleFunc("POST /api/v2/core/auth/passkey/del", func(w http.ResponseWriter, _ *http.Request) { coreJSON(w, nil) })
	mux.HandleFunc("POST /api/v2/core/auth/passkey/finish", handleCoreLogin)
	mux.HandleFunc("POST /api/v2/core/auth/passkey/register/begin", func(w http.ResponseWriter, _ *http.Request) {
		coreJSON(w, map[string]any{"sessionId": coreToken(), "publicKey": map[string]any{}})
	})
	mux.HandleFunc("POST /api/v2/core/auth/passkey/register/finish", handleCoreLogin)
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
	coreJSON(w, map[string]any{"user": user, "session": session})
}

func handleCoreLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("workmesh_session"); err == nil {
		localCore.Logout(cookie.Value)
	}
	coreJSON(w, nil)
}
func handleCoreCurrent(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("workmesh_session")
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	user, err := localCore.Current(cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}
	coreJSON(w, user)
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
