// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterHostContainerCronRoutesDoesNotConflict(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)
	request, err := http.NewRequest(http.MethodGet, "/api/v2/hosts/components/go", nil)
	if err != nil {
		t.Fatal(err)
	}
	response := newRecorder()
	mux.ServeHTTP(response, request)
	if response.status != http.StatusOK {
		t.Fatalf("组件路由状态码错误: %d", response.status)
	}
}

func TestCoreLoginAndSession(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)
	request, _ := http.NewRequest(http.MethodPost, "/api/v2/core/auth/login", bytes.NewBufferString(`{"Name":"admin","Password":"admin"}`))
	request.Header.Set("Content-Type", "application/json")
	response := newRecorder()
	mux.ServeHTTP(response, request)
	if response.status != http.StatusOK {
		t.Fatalf("登录状态码错误: %d", response.status)
	}
}

func TestRegisterIncludesExplicitDashboardRoutes(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)
	for _, path := range []string{
		"/api/v2/dashboard/base/os",
		"/api/v2/dashboard/current/node",
		"/api/v2/dashboard/current/top/cpu",
		"/api/v2/dashboard/current/top/mem",
		"/api/v2/dashboard/quick/option",
		"/api/v2/dashboard/app/launcher",
	} {
		response := newRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.status != http.StatusOK {
			t.Fatalf("仪表盘路由 %s 未返回成功状态: %d", path, response.status)
		}
	}
}

type recorder struct {
	header http.Header
	status int
}

func newRecorder() *recorder               { return &recorder{header: make(http.Header)} }
func (r *recorder) Header() http.Header    { return r.header }
func (r *recorder) WriteHeader(status int) { r.status = status }
func (r *recorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return len(body), nil
}
