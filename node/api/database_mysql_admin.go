// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
)

// decodeDatabaseSecret 接受 1Panel 的 base64 密码字段，同时兼容未编码的内部调用。
func decodeDatabaseSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
		return string(decoded)
	}
	return value
}

func databaseAdminHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "%"
	}
	return value
}

func mysqlTarget(ctx context.Context, server, typ string) (service.MySQLTarget, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return service.MySQLTarget{}, errors.New("MySQL 实例不能为空")
	}
	target, ok := mysqlTargetForRequest(ctx, server, typ)
	if !ok {
		return service.MySQLTarget{}, errors.New("目标 MySQL/MariaDB 实例未登记")
	}
	return target, nil
}

func mysqlTypeForTarget(target service.MySQLTarget) string {
	typ := strings.ToLower(strings.TrimSpace(target.Type))
	if typ != "mariadb" {
		return "mysql"
	}
	return typ
}

func databaseUserByIDOrIdentity(ctx context.Context, id int64, server, username, host string) (service.DatabaseUser, bool) {
	items := databaseAdmin.ListUsers(ctx, server, username)
	host = databaseAdminHost(host)
	for _, item := range items {
		if id > 0 && item.ID == id {
			return item, true
		}
		if id == 0 && strings.EqualFold(item.Database, server) && strings.EqualFold(item.Username, username) && item.Host == host {
			return item, true
		}
	}
	return service.DatabaseUser{}, false
}

func grantDatabaseNames(ctx context.Context, server, username, host string) []string {
	grants := databaseAdmin.ListGrants(ctx, server, username)
	items := make([]string, 0, len(grants))
	for _, grant := range grants {
		if strings.EqualFold(grant.Username, username) && grant.Host == databaseAdminHost(host) {
			items = append(items, grant.Database)
		}
	}
	return items
}

func rollbackUserMetadata(ctx context.Context, user service.DatabaseUser) {
	_ = databaseAdmin.DeleteUserByIdentity(ctx, user.Database, user.Username, user.Host)
	for _, grant := range databaseAdmin.ListGrants(ctx, user.Database, user.Username) {
		_ = databaseAdmin.DeleteGrantByIdentity(ctx, user.Database, grant.Database, grant.Username, grant.Host)
	}
}

func formatMySQLAdminError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("MySQL 管理操作失败: %w", err)
}
