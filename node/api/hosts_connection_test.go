// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestHostConnectionTestByInfoProbesTCPPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("当前沙箱禁止 loopback 监听: %v", err)
	}
	defer listener.Close()
	port, _ := strconv.Atoi(strings.TrimPrefix(listener.Addr().String(), "127.0.0.1:"))
	mux := http.NewServeMux()
	registerHostRoutes(mux)
	request := httptest.NewRequest(http.MethodPost, "/api/v2/hosts/test/byinfo", strings.NewReader(`{"addr":"127.0.0.1","port":`+strconv.Itoa(port)+`}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Connected bool `json:"connected"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Data.Connected {
		t.Fatalf("expected connected probe: %s", response.Body.String())
	}
}

func TestHostConnectionTestRejectsInvalidPort(t *testing.T) {
	mux := http.NewServeMux()
	registerHostRoutes(mux)
	request := httptest.NewRequest(http.MethodPost, "/api/v2/hosts/test/byinfo", strings.NewReader(`{"addr":"127.0.0.1","port":0}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_HOST_TEST") {
		t.Fatalf("expected invalid host test error, status=%d body=%s", response.Code, response.Body.String())
	}
}
