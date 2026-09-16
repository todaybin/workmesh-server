// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

// writeTerminalError 将终端错误转换为前端约定的 base64 命令输出帧。
func writeTerminalError(ws *streamWebSocket, err error) error {
	if err == nil {
		err = errors.New("终端操作失败")
	}
	message, _ := json.Marshal(terminalServerMessage{Type: "cmd", Data: base64.StdEncoding.EncodeToString([]byte(err.Error() + "\r\n"))})
	return ws.writeText(message)
}
