// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import "database/sql"

var controlDB *sql.DB

// SetControlDatabase 注入与节点执行面共享的 SQLite 连接。
func SetControlDatabase(db *sql.DB) { controlDB = db }
