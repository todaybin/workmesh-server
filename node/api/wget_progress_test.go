// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWgetProgressWriterUpdatesIncrementally(t *testing.T) {
	initFileWgetState()
	const key = "wget-progress-test"
	fileWgetState.Lock()
	fileWgetState.items[key] = &fileWgetProcess{Key: key, Path: "/var/lib/workmesh/file.bin", Status: "running", Total: 6}
	fileWgetState.cancel[key] = func() {}
	fileWgetState.Unlock()
	t.Cleanup(func() {
		fileWgetState.Lock()
		delete(fileWgetState.items, key)
		delete(fileWgetState.cancel, key)
		fileWgetState.Unlock()
	})

	var destination bytes.Buffer
	counter := &wgetProgressWriter{dst: &destination, key: key}
	if _, err := counter.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	fileWgetState.RLock()
	first := fileWgetState.items[key].Downloaded
	fileWgetState.RUnlock()
	if first != 3 {
		t.Fatalf("第一次写入后进度应为 3，实际 %d", first)
	}
	if _, err := counter.Write([]byte("def")); err != nil {
		t.Fatal(err)
	}
	fileWgetState.RLock()
	second := fileWgetState.items[key].Downloaded
	fileWgetState.RUnlock()
	if second != 6 {
		t.Fatalf("第二次写入后进度应为 6，实际 %d", second)
	}
	if destination.String() != "abcdef" {
		t.Fatalf("目标文件内容错误: %q", destination.String())
	}
}

func TestWgetStopAcceptsFrontendKeyAndMarksCancelled(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	initFileWgetState()
	const key = "wget-stop-key-test"
	fileWgetState.Lock()
	fileWgetState.items[key] = &fileWgetProcess{Key: key, Path: "/var/lib/workmesh/file.bin", Status: "running", StartedAt: time.Now().UTC()}
	fileWgetState.cancel[key] = func() {}
	fileWgetState.Unlock()
	t.Cleanup(func() {
		fileWgetState.Lock()
		delete(fileWgetState.items, key)
		delete(fileWgetState.cancel, key)
		fileWgetState.Unlock()
	})

	body, _ := json.Marshal(map[string]string{"key": key})
	res := httptest.NewRecorder()
	fileAdvancedHandler(res, httptest.NewRequest(http.MethodPost, "/api/v2/files/wget/stop", bytes.NewReader(body)))
	if res.Code != http.StatusOK {
		t.Fatalf("前端 key 停止下载应成功，状态=%d 响应=%s", res.Code, res.Body.String())
	}
	fileWgetState.RLock()
	status := fileWgetState.items[key].Status
	fileWgetState.RUnlock()
	if status != "cancelled" {
		t.Fatalf("停止下载后状态应为 cancelled，实际 %q", status)
	}
}
