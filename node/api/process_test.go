// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProcessWebSocketNonUpgrade(t *testing.T) {
	mux := http.NewServeMux()
	registerProcessRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/process/ws", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 for capability probe, got %d", res.Code)
	}
	if got := res.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("expected JSON capability response, got %q", got)
	}
}

func TestProcessWebSocketUpgradeRequiresAuth(t *testing.T) {
	t.Setenv("WORKMESH_PROCESS_TOKEN", "process-secret")
	mux := http.NewServeMux()
	registerProcessRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/process/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized websocket status = %d", res.Code)
	}
}

func TestProcessWebSocketRejectsCrossOrigin(t *testing.T) {
	t.Setenv("WORKMESH_PROCESS_TOKEN", "process-secret")
	mux := http.NewServeMux()
	registerProcessRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "http://node.example/api/v2/process/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("X-WorkMesh-Token", "process-secret")
	req.Header.Set("Origin", "https://evil.example")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("cross-origin websocket status = %d, want 403", res.Code)
	}
}

func TestWriteWebSocketTextFrameExtendedLength(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	payload := bytes.Repeat([]byte("x"), 126)
	errCh := make(chan error, 1)
	go func() { errCh <- writeWebSocketTextFrame(server, payload) }()
	header := make([]byte, 4)
	if _, err := client.Read(header); err != nil {
		t.Fatal(err)
	}
	if header[0] != 0x81 || header[1] != 126 || header[2] != 0 || header[3] != 126 {
		t.Fatalf("unexpected extended frame header: %v", header)
	}
	if _, err := io.CopyN(io.Discard, client, int64(len(payload))); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestParseListeningOutput(t *testing.T) {
	output := "Netid State Local Address:Port Peer Address:Port Process\n" +
		"tcp LISTEN 0 128 127.0.0.1:9999 0.0.0.0:* users:((workmesh,pid=10,fd=3))\n" +
		"udp UNCONN 0 0 0.0.0.0:53 0.0.0.0:* users:((dns,pid=11,fd=4))\n"
	items := parseListeningOutput(output)
	if len(items) != 2 {
		t.Fatalf("expected two listening entries, got %#v", items)
	}
	if items[0]["protocol"] != "tcp" || items[0]["localAddress"] != "127.0.0.1:9999" {
		t.Fatalf("unexpected first entry: %#v", items[0])
	}
	if items[0]["process"] == "" {
		t.Fatal("process metadata should be preserved")
	}
}

func TestParseListeningOutputIsBounded(t *testing.T) {
	output := "header\n"
	for i := 0; i < 1100; i++ {
		output += "tcp LISTEN 0 128 127.0.0.1:1 0.0.0.0:*\n"
	}
	if got := len(parseListeningOutput(output)); got != 1024 {
		t.Fatalf("expected 1024-entry bound, got %d", got)
	}
}
