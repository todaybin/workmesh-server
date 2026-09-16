// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// websiteTemplateRecord 是模板关系表的 API DTO，字段名称保持旧前端契约。
type websiteTemplateRecord struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	FilePath  string `json:"filePath"`
	Variables string `json:"variables"`
	Remark    string `json:"remark"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// websiteTemplateOutputRecord 是模板产物关系表的 API DTO。
type websiteTemplateOutputRecord struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	TemplateID     uint   `json:"templateID"`
	TemplateType   string `json:"templateType"`
	VariableValues string `json:"variableValues"`
	OutputPath     string `json:"outputPath"`
	TemplateName   string `json:"templateName"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// websiteTemplateRepository 负责模板和产物的关系化持久化，不再把业务记录放入 JSON 状态 BLOB。
type websiteTemplateRepository struct {
	db         *sql.DB
	repository storage.Transactional
	initErr    error
}

// newWebsiteTemplateRepository 创建仓储并执行幂等建表及旧状态迁移。
func newWebsiteTemplateRepository(db *sql.DB) *websiteTemplateRepository {
	repo := &websiteTemplateRepository{db: db}
	if db == nil {
		repo.initErr = errors.New("网站模板公共数据库未初始化")
		return repo
	}
	if err := ensureWebsiteTemplateTables(db); err != nil {
		repo.initErr = err
		return repo
	}
	repo.repository, repo.initErr = storage.NewSQLiteRepository(db)
	return repo
}

// executor returns the initialized repository used by template business
// operations. Table creation and legacy import remain startup migration work.
func (r *websiteTemplateRepository) executor() (storage.Transactional, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	if r.repository != nil {
		return r.repository, nil
	}
	if r.db == nil {
		return nil, errors.New("网站模板公共数据库未初始化")
	}
	repository, err := storage.NewSQLiteRepository(r.db)
	if err != nil {
		return nil, err
	}
	r.repository = repository
	return repository, nil
}

// ensureWebsiteTemplateTables 创建模板领域关系表，并幂等导入旧 JSON 状态。
func ensureWebsiteTemplateTables(db *sql.DB) error {
	if db == nil {
		return errors.New("网站模板公共数据库未初始化")
	}
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS website_extension_state (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS website_templates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'single',
			content TEXT NOT NULL DEFAULT '',
			file_path TEXT NOT NULL DEFAULT '',
			variables TEXT NOT NULL DEFAULT '',
			remark TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_website_templates_name ON website_templates(name)`,
		`CREATE INDEX IF NOT EXISTS idx_website_templates_created ON website_templates(created_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS website_template_outputs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			template_id INTEGER NOT NULL REFERENCES website_templates(id) ON DELETE CASCADE,
			template_type TEXT NOT NULL DEFAULT '',
			variable_values TEXT NOT NULL DEFAULT '{}',
			output_path TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_website_template_outputs_template ON website_template_outputs(template_id, created_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS website_template_migrations (
			migration_key TEXT PRIMARY KEY,
			migrated_at TEXT NOT NULL
		)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("创建网站模板关系表失败: %w", err)
		}
	}
	return migrateLegacyWebsiteTemplates(db)
}
