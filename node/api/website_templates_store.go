// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// searchTemplates 从 SQLite 读取模板分页数据，查询不依赖进程内缓存。
func (r *websiteTemplateRepository) searchTemplates(name, typ string, page, pageSize int) (int, []websiteTemplateRecord, error) {
	repository, err := r.executor()
	if err != nil {
		return 0, nil, err
	}
	page, pageSize = normalizeTemplatePage(page, pageSize)
	nameLike := "%" + strings.ToLower(strings.TrimSpace(name)) + "%"
	typeLike := strings.TrimSpace(typ)
	var total int
	if err := repository.QueryRow(`SELECT COUNT(*) FROM website_templates WHERE lower(name) LIKE ? AND (?='' OR type=?)`, nameLike, typeLike, typeLike).Scan(&total); err != nil {
		return 0, nil, err
	}
	rows, err := repository.Query(`SELECT id,name,type,content,file_path,variables,remark,created_at,updated_at FROM website_templates WHERE lower(name) LIKE ? AND (?='' OR type=?) ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, nameLike, typeLike, typeLike, pageSize, (page-1)*pageSize)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	items := make([]websiteTemplateRecord, 0)
	for rows.Next() {
		item, scanErr := scanWebsiteTemplate(rows)
		if scanErr != nil {
			return 0, nil, scanErr
		}
		items = append(items, item)
	}
	return total, items, rows.Err()
}

// getTemplate 从 SQLite 读取单个模板。
func (r *websiteTemplateRepository) getTemplate(id uint) (websiteTemplateRecord, error) {
	repository, err := r.executor()
	if err != nil {
		return websiteTemplateRecord{}, err
	}
	row := repository.QueryRow(`SELECT id,name,type,content,file_path,variables,remark,created_at,updated_at FROM website_templates WHERE id=?`, id)
	item, err := scanWebsiteTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return websiteTemplateRecord{}, os.ErrNotExist
	}
	return item, err
}

// createTemplate 校验并写入模板，multi 模板必须引用已落盘 ZIP。
func (r *websiteTemplateRepository) createTemplate(input websiteTemplateInput) (websiteTemplateRecord, error) {
	repository, err := r.executor()
	if err != nil {
		return websiteTemplateRecord{}, err
	}
	if err := validateWebsiteTemplateInput(input, false); err != nil {
		return websiteTemplateRecord{}, err
	}
	if exists, err := r.templateNameExists(input.Name, 0); err != nil {
		return websiteTemplateRecord{}, err
	} else if exists {
		return websiteTemplateRecord{}, errors.New("网站模板名称已存在")
	}
	now := formatTemplateTime(time.Now().UTC())
	result, err := repository.Exec(`INSERT INTO website_templates(name,type,content,file_path,variables,remark,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, input.Name, input.Type, input.Content, input.FilePath, input.Variables, input.Remark, now, now)
	if err != nil {
		return websiteTemplateRecord{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return websiteTemplateRecord{}, err
	}
	return r.getTemplate(uint(id))
}

// updateTemplate 更新模板关系记录并保留原有文件路径契约。
func (r *websiteTemplateRepository) updateTemplate(input websiteTemplateInput) (websiteTemplateRecord, error) {
	repository, err := r.executor()
	if err != nil {
		return websiteTemplateRecord{}, err
	}
	if input.ID == 0 {
		return websiteTemplateRecord{}, errors.New("网站模板 ID 无效")
	}
	existing, err := r.getTemplate(input.ID)
	if err != nil {
		return websiteTemplateRecord{}, err
	}
	if !input.HasName {
		input.Name = existing.Name
	}
	if !input.HasType {
		input.Type = existing.Type
	}
	if !input.HasContent {
		input.Content = existing.Content
	}
	if !input.HasFilePath {
		input.FilePath = existing.FilePath
	}
	if !input.HasVariables {
		input.Variables = existing.Variables
	}
	if !input.HasRemark {
		input.Remark = existing.Remark
	}
	if err := validateWebsiteTemplateInput(input, true); err != nil {
		return websiteTemplateRecord{}, err
	}
	if exists, err := r.templateNameExists(input.Name, input.ID); err != nil {
		return websiteTemplateRecord{}, err
	} else if exists {
		return websiteTemplateRecord{}, errors.New("网站模板名称已存在")
	}
	_, err = repository.Exec(`UPDATE website_templates SET name=?,type=?,content=?,file_path=?,variables=?,remark=?,updated_at=? WHERE id=?`, input.Name, input.Type, input.Content, input.FilePath, input.Variables, input.Remark, formatTemplateTime(time.Now().UTC()), input.ID)
	if err != nil {
		return websiteTemplateRecord{}, err
	}
	return r.getTemplate(input.ID)
}

// deleteTemplate 删除模板及其产物记录，并清理 WorkMesh 生成的文件目录。
func (r *websiteTemplateRepository) deleteTemplate(id uint) error {
	repository, err := r.executor()
	if err != nil {
		return err
	}
	item, err := r.getTemplate(id)
	if err != nil {
		return err
	}
	outputPaths := make([]string, 0)
	rows, err := repository.Query(`SELECT output_path FROM website_template_outputs WHERE template_id=?`, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var outputPath string
		if err := rows.Scan(&outputPath); err != nil {
			_ = rows.Close()
			return err
		}
		outputPaths = append(outputPaths, outputPath)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	if _, err := repository.Exec(`DELETE FROM website_templates WHERE id=?`, id); err != nil {
		return err
	}
	if isWebsiteTemplateOwnedPath(item.FilePath) {
		_ = os.Remove(item.FilePath)
	}
	for _, outputPath := range outputPaths {
		if isWebsiteTemplateOwnedPath(outputPath) {
			_ = os.RemoveAll(outputPath)
		}
	}
	return nil
}

// searchOutputs 从 SQLite 读取产物分页数据，并关联模板名称。
func (r *websiteTemplateRepository) searchOutputs(templateID uint, page, pageSize int) (int, []websiteTemplateOutputRecord, error) {
	repository, err := r.executor()
	if err != nil {
		return 0, nil, err
	}
	page, pageSize = normalizeTemplatePage(page, pageSize)
	var total int
	if err := repository.QueryRow(`SELECT COUNT(*) FROM website_template_outputs WHERE (?=0 OR template_id=?)`, templateID, templateID).Scan(&total); err != nil {
		return 0, nil, err
	}
	rows, err := repository.Query(`SELECT o.id,o.name,o.template_id,o.template_type,o.variable_values,o.output_path,o.created_at,o.updated_at,COALESCE(t.name,'') FROM website_template_outputs o LEFT JOIN website_templates t ON t.id=o.template_id WHERE (?=0 OR o.template_id=?) ORDER BY o.created_at DESC,o.id DESC LIMIT ? OFFSET ?`, templateID, templateID, pageSize, (page-1)*pageSize)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	items := make([]websiteTemplateOutputRecord, 0)
	for rows.Next() {
		var item websiteTemplateOutputRecord
		if err := rows.Scan(&item.ID, &item.Name, &item.TemplateID, &item.TemplateType, &item.VariableValues, &item.OutputPath, &item.CreatedAt, &item.UpdatedAt, &item.TemplateName); err != nil {
			return 0, nil, err
		}
		items = append(items, item)
	}
	return total, items, rows.Err()
}

// getOutput 从 SQLite 读取单个产物。
func (r *websiteTemplateRepository) getOutput(id uint) (websiteTemplateOutputRecord, error) {
	repository, err := r.executor()
	if err != nil {
		return websiteTemplateOutputRecord{}, err
	}
	var item websiteTemplateOutputRecord
	err = repository.QueryRow(`SELECT o.id,o.name,o.template_id,o.template_type,o.variable_values,o.output_path,o.created_at,o.updated_at,COALESCE(t.name,'') FROM website_template_outputs o LEFT JOIN website_templates t ON t.id=o.template_id WHERE o.id=?`, id).Scan(&item.ID, &item.Name, &item.TemplateID, &item.TemplateType, &item.VariableValues, &item.OutputPath, &item.CreatedAt, &item.UpdatedAt, &item.TemplateName)
	if errors.Is(err, sql.ErrNoRows) {
		return websiteTemplateOutputRecord{}, os.ErrNotExist
	}
	return item, err
}

// createOutput 读取模板、真实渲染文件并写入产物关系记录。
func (r *websiteTemplateRepository) createOutput(input websiteTemplateOutputInput) (websiteTemplateOutputRecord, error) {
	repository, err := r.executor()
	if err != nil {
		return websiteTemplateOutputRecord{}, err
	}
	if input.TemplateID == 0 || strings.TrimSpace(input.Name) == "" {
		return websiteTemplateOutputRecord{}, errors.New("模板产物名称和模板 ID 不能为空")
	}
	template, err := r.getTemplate(input.TemplateID)
	if err != nil {
		return websiteTemplateOutputRecord{}, err
	}
	valuesJSON, err := json.Marshal(input.VariableValues)
	if err != nil {
		return websiteTemplateOutputRecord{}, fmt.Errorf("模板变量序列化失败: %w", err)
	}
	now := formatTemplateTime(time.Now().UTC())
	result, err := repository.Exec(`INSERT INTO website_template_outputs(name,template_id,template_type,variable_values,output_path,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, strings.TrimSpace(input.Name), template.ID, template.Type, string(valuesJSON), "", now, now)
	if err != nil {
		return websiteTemplateOutputRecord{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return websiteTemplateOutputRecord{}, err
	}
	outputDir := filepath.Join(websiteTemplateOutputRoot(), strconv.FormatUint(uint64(id), 10))
	if err := renderWebsiteTemplateToDir(template, input.VariableValues, outputDir); err != nil {
		_ = os.RemoveAll(outputDir)
		_, _ = repository.Exec(`DELETE FROM website_template_outputs WHERE id=?`, id)
		return websiteTemplateOutputRecord{}, err
	}
	if _, err := repository.Exec(`UPDATE website_template_outputs SET output_path=?,updated_at=? WHERE id=?`, outputDir, formatTemplateTime(time.Now().UTC()), id); err != nil {
		_ = os.RemoveAll(outputDir)
		_, _ = repository.Exec(`DELETE FROM website_template_outputs WHERE id=?`, id)
		return websiteTemplateOutputRecord{}, err
	}
	return r.getOutput(uint(id))
}

// deleteOutput 删除产物数据库记录及其真实生成目录。
func (r *websiteTemplateRepository) deleteOutput(id uint) error {
	repository, err := r.executor()
	if err != nil {
		return err
	}
	item, err := r.getOutput(id)
	if err != nil {
		return err
	}
	if _, err := repository.Exec(`DELETE FROM website_template_outputs WHERE id=?`, id); err != nil {
		return err
	}
	if isWebsiteTemplateOwnedPath(item.OutputPath) {
		_ = os.RemoveAll(item.OutputPath)
	}
	return nil
}

// previewTemplate 读取真实模板并返回与 1Panel 一致的 html 字段。
func (r *websiteTemplateRepository) previewTemplate(id uint, values map[string]string) (map[string]any, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	template, err := r.getTemplate(id)
	if err != nil {
		return nil, err
	}
	var html string
	if template.Type == "single" {
		html = renderWebsiteTemplateContent(template.Content, values)
	} else {
		if !isWebsiteTemplateOwnedPath(template.FilePath) {
			return nil, errors.New("ZIP 模板文件必须位于 WorkMesh 模板目录")
		}
		html, err = readWebsiteTemplateIndex(template.FilePath, values)
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"html": html}, nil
}

// latestTemplate 返回最新模板，兼容旧客户端省略 templateID 的预览请求。
func (r *websiteTemplateRepository) latestTemplate() (websiteTemplateRecord, error) {
	repository, err := r.executor()
	if err != nil {
		return websiteTemplateRecord{}, err
	}
	row := repository.QueryRow(`SELECT id,name,type,content,file_path,variables,remark,created_at,updated_at FROM website_templates ORDER BY created_at DESC,id DESC LIMIT 1`)
	item, err := scanWebsiteTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return websiteTemplateRecord{}, os.ErrNotExist
	}
	return item, err
}

// ready 统一报告关系表初始化错误，避免接口在数据库不可用时返回伪成功。
func (r *websiteTemplateRepository) ready() error {
	if r == nil {
		return errors.New("网站模板仓储未初始化")
	}
	if r.initErr != nil {
		return r.initErr
	}
	return nil
}

// templateNameExists 检查模板名称冲突。
func (r *websiteTemplateRepository) templateNameExists(name string, excludeID uint) (bool, error) {
	repository, err := r.executor()
	if err != nil {
		return false, err
	}
	var count int
	err = repository.QueryRow(`SELECT COUNT(*) FROM website_templates WHERE name=? AND id<>?`, strings.TrimSpace(name), excludeID).Scan(&count)
	return count > 0, err
}

// scanWebsiteTemplate 将 SQL 行转换为前端兼容记录。
func scanWebsiteTemplate(scanner interface{ Scan(...any) error }) (websiteTemplateRecord, error) {
	var item websiteTemplateRecord
	err := scanner.Scan(&item.ID, &item.Name, &item.Type, &item.Content, &item.FilePath, &item.Variables, &item.Remark, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

// websiteTemplateInput 是模板写入请求的内部结构。
type websiteTemplateInput struct {
	ID           uint
	Name         string
	Type         string
	Content      string
	FilePath     string
	Variables    string
	Remark       string
	HasName      bool
	HasType      bool
	HasContent   bool
	HasFilePath  bool
	HasVariables bool
	HasRemark    bool
}

// websiteTemplateOutputInput 是产物创建请求的内部结构。
type websiteTemplateOutputInput struct {
	TemplateID     uint
	Name           string
	VariableValues map[string]string
}

// validateWebsiteTemplateInput 校验模板字段和 multi 模板文件是否真实存在。
func validateWebsiteTemplateInput(input websiteTemplateInput, updating bool) error {
	if updating && input.ID == 0 {
		return errors.New("网站模板 ID 无效")
	}
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("网站模板名称不能为空")
	}
	if input.Type != "single" && input.Type != "multi" {
		return errors.New("网站模板类型无效")
	}
	if input.Type == "multi" {
		if strings.TrimSpace(input.FilePath) == "" {
			return errors.New("ZIP 模板文件不能为空")
		}
		if !isWebsiteTemplateOwnedPath(input.FilePath) {
			return errors.New("ZIP 模板文件必须位于 WorkMesh 模板目录")
		}
		info, err := os.Stat(input.FilePath)
		if err != nil || info.IsDir() {
			return errors.New("ZIP 模板文件不存在")
		}
	}
	return nil
}

// normalizeTemplatePage 将分页限制在合理范围，避免 SQLite 查询消耗无界资源。
func normalizeTemplatePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 100
	}
	return page, pageSize
}

// formatTemplateTime 统一模板领域时间格式。
func formatTemplateTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// websiteTemplateRoot 返回模板数据根目录。
func websiteTemplateRoot() string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	return filepath.Join(root, "templates")
}

// websiteTemplateOutputRoot 返回产物根目录。
func websiteTemplateOutputRoot() string {
	return filepath.Join(websiteTemplateRoot(), "outputs")
}

// isWebsiteTemplateOwnedPath 限制模板文件和产物只能访问 WorkMesh 模板目录。
func isWebsiteTemplateOwnedPath(value string) bool {
	path, err := filepath.Abs(filepath.Clean(strings.TrimSpace(value)))
	root, rootErr := filepath.Abs(filepath.Clean(websiteTemplateRoot()))
	if err != nil || rootErr != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
