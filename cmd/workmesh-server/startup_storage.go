// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/todaybin/workmesh-server/config"
	"github.com/todaybin/workmesh-server/internal/storage"
	nodeapi "github.com/todaybin/workmesh-server/node/api"
	"github.com/todaybin/workmesh-server/node/service"
	"github.com/todaybin/workmesh-server/runtime/role"
)

// initializeServerState 执行监听 HTTP 前的完整持久化启动阶段。
// 失败时关闭刚打开的连接，防止迁移失败后残留 SQLite 文件描述符。
func initializeServerState(cfg config.Config) (*storage.Store, error) {
	stateStore, err := storage.Open(filepath.Join(cfg.DataDir, "workmesh.db"))
	if err != nil {
		return nil, fmt.Errorf("打开状态存储失败: %w", err)
	}
	if err := initializeUnifiedSchema(stateStore); err != nil {
		_ = stateStore.Close()
		return nil, fmt.Errorf("执行统一存储迁移失败: %w", err)
	}
	if report, err := importLegacyData(context.Background(), stateStore, cfg.DataDir); err != nil {
		_ = stateStore.Close()
		return nil, fmt.Errorf("导入旧数据失败（%+v）: %w", report, err)
	}
	if err := initializeSharedServices(stateStore); err != nil {
		_ = stateStore.Close()
		return nil, err
	}
	if err := nodeapi.RecoverDeploymentState(cfg.DataDir); err != nil {
		_ = stateStore.Close()
		return nil, fmt.Errorf("恢复部署制品状态失败: %w", err)
	}
	if _, err := role.New(cfg.NodeID, cfg.Role); err != nil {
		_ = stateStore.Close()
		return nil, fmt.Errorf("节点角色配置无效: %w", err)
	}
	return stateStore, nil
}

// initializeSharedServices 注入统一 SQLite 到网站、节点和控制面公共服务。
func initializeSharedServices(stateStore *storage.Store) error {
	if err := service.SetWebsiteDB(stateStore.DB()); err != nil {
		return fmt.Errorf("初始化网站公共数据库存储失败: %w", err)
	}
	if err := service.SetWebsiteSecurityDB(stateStore.DB()); err != nil {
		return fmt.Errorf("初始化证书公共数据库存储失败: %w", err)
	}
	if err := nodeapi.SetSharedStore(stateStore); err != nil {
		return fmt.Errorf("初始化节点公共控制面存储失败: %w", err)
	}
	if err := nodeapi.SetCoreDatabase(stateStore.DB()); err != nil {
		return fmt.Errorf("初始化认证 SQLite 存储失败: %w", err)
	}
	return nil
}
