// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// WebsiteTemplateMigration 创建模板与模板产物的关系表。
// 业务记录不再依赖 website_extension_state 的 JSON BLOB；旧 BLOB 的一次性
// 导入仍由仓储初始化执行，以便迁移失败时阻止服务就绪并可再次重试。
func WebsiteTemplateMigration() storage.Migration {
	return storage.SQLMigration("0015-website-template-relational", `
CREATE TABLE IF NOT EXISTS website_templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'single',
    content TEXT NOT NULL DEFAULT '',
    file_path TEXT NOT NULL DEFAULT '',
    variables TEXT NOT NULL DEFAULT '',
    remark TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_website_templates_name ON website_templates(name);
CREATE INDEX IF NOT EXISTS idx_website_templates_created ON website_templates(created_at DESC, id DESC);
CREATE TABLE IF NOT EXISTS website_template_outputs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    template_id INTEGER NOT NULL REFERENCES website_templates(id) ON DELETE CASCADE,
    template_type TEXT NOT NULL DEFAULT '',
    variable_values TEXT NOT NULL DEFAULT '{}',
    output_path TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_website_template_outputs_template ON website_template_outputs(template_id, created_at DESC, id DESC);
CREATE TABLE IF NOT EXISTS website_template_migrations (
    migration_key TEXT PRIMARY KEY,
    migrated_at TEXT NOT NULL
)`)
}

// migrateLegacyWebsiteTemplates 把旧扩展状态中的模板和产物一次性导入关系表。
// 迁移使用事务和完成标记，重复启动不会重复插入，失败时下次启动可重试。
func migrateLegacyWebsiteTemplates(db *sql.DB) error {
	var migrated string
	err := db.QueryRow(`SELECT migration_key FROM website_template_migrations WHERE migration_key=?`, "legacy-json-v1").Scan(&migrated)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("读取网站模板迁移状态失败: %w", err)
	}

	var payload []byte
	err = db.QueryRow(`SELECT payload FROM website_extension_state WHERE id=1`).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return markWebsiteTemplateMigration(db)
	}
	// 独立仓储测试或旧数据库可能尚未创建扩展兼容表；这表示没有可迁移
	// 的旧模板，而不是模板业务成功或失败。关系表仍需写入完成标记，避免
	// 每次请求重复探测不存在的历史表。
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return markWebsiteTemplateMigration(db)
	}
	if err != nil {
		return fmt.Errorf("读取旧网站模板状态失败: %w", err)
	}
	var legacy struct {
		Templates []map[string]any `json:"templates"`
		Outputs   []map[string]any `json:"outputs"`
	}
	if err := json.Unmarshal(payload, &legacy); err != nil {
		return fmt.Errorf("解析旧网站模板状态失败: %w", err)
	}
	if len(legacy.Templates) == 0 && len(legacy.Outputs) == 0 {
		return markWebsiteTemplateMigration(db)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始网站模板迁移失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, item := range legacy.Templates {
		if err := migrateLegacyTemplate(tx, item); err != nil {
			return err
		}
	}
	for _, item := range legacy.Outputs {
		if err := migrateLegacyTemplateOutput(tx, item); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO website_template_migrations(migration_key,migrated_at) VALUES(?,?)`, "legacy-json-v1", formatTemplateTime(time.Now().UTC())); err != nil {
		return fmt.Errorf("记录网站模板迁移状态失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交网站模板迁移失败: %w", err)
	}
	return nil
}

// markWebsiteTemplateMigration 在没有旧数据时也写入完成标记，避免每次启动重复扫描 BLOB。
func markWebsiteTemplateMigration(db *sql.DB) error {
	_, err := db.Exec(`INSERT INTO website_template_migrations(migration_key,migrated_at) VALUES(?,?) ON CONFLICT(migration_key) DO NOTHING`, "legacy-json-v1", formatTemplateTime(time.Now().UTC()))
	return err
}

// migrateLegacyTemplate 导入单条旧模板记录，保留原 ID 以维持产物关联。
func migrateLegacyTemplate(tx *sql.Tx, item map[string]any) error {
	id := legacyUint(item, "id")
	name := strings.TrimSpace(legacyString(item, "name"))
	if name == "" {
		return nil
	}
	typ := legacyString(item, "type")
	if typ == "" {
		typ = "single"
	}
	created := legacyTime(item, "createdAt", "created_at")
	updated := legacyTime(item, "updatedAt", "updated_at")
	args := []any{name, typ, legacyString(item, "content"), legacyString(item, "filePath", "file_path"), legacyJSONString(item, "variables", ""), legacyString(item, "remark"), created, updated}
	if id > 0 {
		_, err := tx.Exec(`INSERT OR IGNORE INTO website_templates(id,name,type,content,file_path,variables,remark,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, append([]any{id}, args...)...)
		return err
	}
	_, err := tx.Exec(`INSERT INTO website_templates(name,type,content,file_path,variables,remark,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, args...)
	return err
}

// migrateLegacyTemplateOutput 导入单条旧产物，仅关联已存在的模板。
func migrateLegacyTemplateOutput(tx *sql.Tx, item map[string]any) error {
	templateID := legacyUint(item, "templateID", "templateId")
	if templateID == 0 {
		return nil
	}
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM website_templates WHERE id=?`, templateID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	id := legacyUint(item, "id")
	args := []any{legacyString(item, "name"), templateID, legacyString(item, "templateType", "type"), legacyJSONString(item, "variableValues", "{}"), legacyString(item, "outputPath", ""), legacyTime(item, "createdAt", "created_at"), legacyTime(item, "updatedAt", "updated_at")}
	if id > 0 {
		_, err := tx.Exec(`INSERT OR IGNORE INTO website_template_outputs(id,name,template_id,template_type,variable_values,output_path,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, append([]any{id}, args...)...)
		return err
	}
	_, err := tx.Exec(`INSERT INTO website_template_outputs(name,template_id,template_type,variable_values,output_path,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, args...)
	return err
}

// legacyString 读取旧 JSON 记录中的字符串字段。
func legacyString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			if text, ok := value.(string); ok {
				return text
			}
			if value != nil {
				return fmt.Sprint(value)
			}
		}
	}
	return ""
}

// legacyUint 兼容旧 JSON 数字和字符串 ID。
func legacyUint(item map[string]any, keys ...string) uint {
	for _, key := range keys {
		value, ok := item[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			if typed > 0 {
				return uint(typed)
			}
		case json.Number:
			if parsed, err := strconv.ParseUint(string(typed), 10, 32); err == nil {
				return uint(parsed)
			}
		case string:
			if parsed, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 32); err == nil {
				return uint(parsed)
			}
		}
	}
	return 0
}

// legacyJSONString 保留旧记录中对象或数组字段的 JSON 表示。
func legacyJSONString(item map[string]any, key, fallback string) string {
	value, ok := item[key]
	if !ok || value == nil {
		return fallback
	}
	if text, ok := value.(string); ok {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(encoded)
}

// legacyTime 读取旧记录时间，缺失时使用当前 UTC 时间保证关系表约束完整。
func legacyTime(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := strings.TrimSpace(legacyString(item, key)); text != "" {
			return text
		}
	}
	return formatTemplateTime(time.Now().UTC())
}
