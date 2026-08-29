// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import "context"

// Service 是控制面业务入口，迁移 Core 功能时按领域扩展实现。
type Service interface {
	Health(context.Context) error
}

// BasicService 提供骨架阶段的健康实现。
type BasicService struct{}

// Health 报告控制面基础服务可用。
func (BasicService) Health(context.Context) error { return nil }
