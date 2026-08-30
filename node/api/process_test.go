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
