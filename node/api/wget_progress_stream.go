// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
)

type wgetProgressRequest struct {
	Type string   `json:"type"`
	Keys []string `json:"keys"`
}

type wgetProgressMessage struct {
	Total   int64   `json:"total"`
	Written int64   `json:"written"`
	Percent float64 `json:"percent"`
	Name    string  `json:"name"`
	Key     string  `json:"key"`
	Status  string  `json:"status"`
	Error   string  `json:"error,omitempty"`
}

func handleWgetProgressStream(w http.ResponseWriter, r *http.Request) {
	if !requireStreamAuth(w, r, "WORKMESH_FILE_TOKEN", "WORKMESH_STREAM_TOKEN") {
		return
	}
	ws, err := upgradeStreamWebSocket(w, r)
	if err != nil {
		return
	}
	defer ws.close()
	for {
		opcode, payload, err := ws.readFrame()
		if err != nil {
			return
		}
		if opcode == 0x8 {
			return
		}
		if opcode == 0x9 {
			ws.writeMu.Lock()
			_ = writeStreamFrame(ws.conn, 0xA, payload)
			ws.writeMu.Unlock()
			continue
		}
		if opcode != 0x1 {
			continue
		}
		var request wgetProgressRequest
		if err := json.Unmarshal(payload, &request); err != nil || request.Type != "wget" || len(request.Keys) > 200 {
			_ = writeWgetProgressError(ws, errors.New("下载进度请求无效或 keys 超过 200 个"))
			continue
		}
		messages := wgetProgressSnapshot(request.Keys)
		encoded, _ := json.Marshal(messages)
		if err := ws.writeText(encoded); err != nil {
			return
		}
	}
}

func wgetProgressSnapshot(keys []string) []wgetProgressMessage {
	initFileWgetState()
	fileWgetState.RLock()
	defer fileWgetState.RUnlock()
	result := make([]wgetProgressMessage, 0, len(keys))
	for _, key := range keys {
		if len(key) > 128 || !strings.HasPrefix(key, "wget-") {
			continue
		}
		item := fileWgetState.items[key]
		if item == nil {
			continue
		}
		percent := float64(0)
		if item.Total > 0 {
			percent = float64(item.Downloaded) * 100 / float64(item.Total)
		}
		if item.Status == "completed" {
			percent = 100
		}
		result = append(result, wgetProgressMessage{Total: item.Total, Written: item.Downloaded, Percent: percent, Name: filepath.Base(item.Path), Key: key, Status: item.Status, Error: item.Error})
	}
	return result
}

func writeWgetProgressError(ws *streamWebSocket, err error) error {
	payload, _ := json.Marshal(map[string]any{"type": "error", "message": err.Error()})
	return ws.writeText(payload)
}
