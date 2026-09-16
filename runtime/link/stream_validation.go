// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import "errors"

// validateStream 校验同步流名称，避免非法名称进入任何持久化实现。
func validateStream(stream string) error {
	if stream == "" || len(stream) > 128 {
		return errors.New("同步流名称无效")
	}
	for _, char := range stream {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '.' && char != '_' && char != '-' {
			return errors.New("同步流名称包含非法字符")
		}
	}
	return nil
}
