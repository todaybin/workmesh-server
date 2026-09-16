// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"io"
)

// decodeSingleJSON 将请求体解析为一个 JSON 值，并拒绝第二个值或非空尾随内容。
// 该边界保留未知字段兼容性；字段白名单由具体业务 Handler 负责校验。
func decodeSingleJSON(reader io.Reader, target any, limit int64) error {
	if reader == nil {
		return errors.New("请求体不能为空")
	}
	decoder := json.NewDecoder(io.LimitReader(reader, limit))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("请求 JSON 只能包含一个值")
		}
		return err
	}
	return nil
}
