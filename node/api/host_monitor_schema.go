// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import "github.com/todaybin/workmesh-server/internal/storage"

const hostMonitorSchemaSQL = `
CREATE TABLE IF NOT EXISTS monitor_bases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TEXT NOT NULL,
    cpu REAL NOT NULL,
    memory REAL NOT NULL,
    load_usage REAL NOT NULL,
    cpu_load1 REAL NOT NULL,
    cpu_load5 REAL NOT NULL,
    cpu_load15 REAL NOT NULL,
    top_cpu TEXT NOT NULL DEFAULT '[]',
    top_mem TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_monitor_bases_created ON monitor_bases(created_at);
CREATE TABLE IF NOT EXISTS monitor_ios (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TEXT NOT NULL,
    name TEXT NOT NULL,
    read_bytes REAL NOT NULL,
    write_bytes REAL NOT NULL,
    count REAL NOT NULL,
    time_ms REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_monitor_ios_name_created ON monitor_ios(name, created_at);
CREATE TABLE IF NOT EXISTS monitor_networks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at TEXT NOT NULL,
    name TEXT NOT NULL,
    up REAL NOT NULL,
    down REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_monitor_networks_name_created ON monitor_networks(name, created_at);`

// HostMonitorMigration 创建主机监控历史表。采样行由保存天数清理，不在设置 JSON 里无限增长。
func HostMonitorMigration() storage.Migration {
	return storage.SQLMigration("0017-host-monitor", hostMonitorSchemaSQL)
}
