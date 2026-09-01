// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxLegacyJSONBytes int64 = 64 << 20

// LegacyDomain 标识旧 JSON 所属的业务域。
type LegacyDomain string

const (
	// LegacyDomainGroups 表示分组数据。
	LegacyDomainGroups LegacyDomain = "groups"
	// LegacyDomainWebsites 表示网站数据。
	LegacyDomainWebsites LegacyDomain = "websites"
	// LegacyDomainDomains 表示网站域名数据。
	LegacyDomainDomains LegacyDomain = "domains"
	// LegacyDomainSSL 表示证书、CA 和 ACME 相关数据。
	LegacyDomainSSL LegacyDomain = "ssl"
)

// LegacyImportStatus 描述单个旧数据源的导入结果。
type LegacyImportStatus string

const (
	// LegacyImportImported 表示导入成功。
	LegacyImportImported LegacyImportStatus = "imported"
	// LegacyImportSkipped 表示相同源摘要已经成功导入。
	LegacyImportSkipped LegacyImportStatus = "skipped"
	// LegacyImportPending 表示已发现文件，但尚未注册领域 handler。
	LegacyImportPending LegacyImportStatus = "pending"
	// LegacyImportNotFound 表示候选源文件不存在。
	LegacyImportNotFound LegacyImportStatus = "not_found"
	// LegacyImportFailed 表示读取、校验或领域导入失败。
	LegacyImportFailed LegacyImportStatus = "failed"
)

// LegacyJSONSource 描述一个只读旧 JSON 文件及其已校验内容。
type LegacyJSONSource struct {
	Domain LegacyDomain    `json:"domain"`
	Path   string          `json:"path"`
	SHA256 string          `json:"sha256"`
	Data   json.RawMessage `json:"-"`
}

// LegacyJSONSourceSpec 描述领域与旧文件名的映射，可由后续功能域继续扩展。
type LegacyJSONSourceSpec struct {
	Domain   LegacyDomain
	FileName string
}

// LegacyJSONHandler 在同一数据库事务内把一个旧 JSON 源转换为目标业务表。
type LegacyJSONHandler interface {
	Domain() LegacyDomain
	Import(context.Context, *sql.Tx, LegacyJSONSource) (int64, error)
}

// LegacyJSONHandlerFunc 将函数适配为 LegacyJSONHandler。
type LegacyJSONHandlerFunc struct {
	DomainName LegacyDomain
	ImportFunc func(context.Context, *sql.Tx, LegacyJSONSource) (int64, error)
}

// Domain 返回函数 handler 负责的业务域。
func (h LegacyJSONHandlerFunc) Domain() LegacyDomain { return h.DomainName }

// Import 执行函数 handler。
func (h LegacyJSONHandlerFunc) Import(ctx context.Context, tx *sql.Tx, source LegacyJSONSource) (int64, error) {
	if h.ImportFunc == nil {
		return 0, errors.New("旧 JSON 导入函数不能为空")
	}
	return h.ImportFunc(ctx, tx, source)
}

// LegacyImportItem 是单个旧 JSON 文件的导入报告。
type LegacyImportItem struct {
	Domain        LegacyDomain       `json:"domain"`
	Path          string             `json:"path"`
	SHA256        string             `json:"sha256,omitempty"`
	Status        LegacyImportStatus `json:"status"`
	ImportedCount int64              `json:"importedCount"`
	Error         string             `json:"error,omitempty"`
}

// LegacyImportReport 汇总一次旧 JSON 扫描和导入结果。
type LegacyImportReport struct {
	Version    int                `json:"version"`
	SourceDir  string             `json:"sourceDir"`
	Status     LegacyImportStatus `json:"status"`
	StartedAt  time.Time          `json:"startedAt"`
	FinishedAt time.Time          `json:"finishedAt"`
	Items      []LegacyImportItem `json:"items"`
}

// LegacyJSONImportOptions 配置旧 JSON 扫描范围和领域 handler。
type LegacyJSONImportOptions struct {
	SourceDir string
	Sources   []LegacyJSONSourceSpec
	Handlers  []LegacyJSONHandler
	// ArchiveImported 在启动迁移成功后将已导入源移动到 data/backups/legacy-<timestamp>。
	ArchiveImported bool
}

// DefaultLegacyJSONSources 返回当前兼容的分组、网站、域名和 SSL 文件清单。
func DefaultLegacyJSONSources() []LegacyJSONSourceSpec {
	return []LegacyJSONSourceSpec{
		{Domain: LegacyDomainGroups, FileName: "groups.json"},
		{Domain: LegacyDomainWebsites, FileName: "websites.json"},
		{Domain: LegacyDomainDomains, FileName: "website-domains.json"},
		{Domain: LegacyDomainSSL, FileName: "ssl.json"},
		{Domain: LegacyDomainSSL, FileName: "website-acme.json"},
		{Domain: LegacyDomainSSL, FileName: "website-ca.json"},
		{Domain: LegacyDomainSSL, FileName: "website-ca-ssls.json"},
	}
}

// ImportLegacyJSON 扫描旧 JSON，并通过已注册 handler 幂等导入；源文件始终保持只读且不会被删除。
func (s *Store) ImportLegacyJSON(ctx context.Context, options LegacyJSONImportOptions) (LegacyImportReport, error) {
	report := LegacyImportReport{Version: 1, StartedAt: time.Now().UTC(), Status: LegacyImportImported}
	if s == nil || s.db == nil {
		return report, errors.New("SQLite 存储未打开")
	}
	sourceDir := strings.TrimSpace(options.SourceDir)
	if sourceDir == "" {
		return report, errors.New("旧 JSON 数据目录不能为空")
	}
	absDir, err := filepath.Abs(filepath.Clean(sourceDir))
	if err != nil {
		return report, fmt.Errorf("解析旧 JSON 数据目录失败: %w", err)
	}
	report.SourceDir = absDir
	sources := options.Sources
	if len(sources) == 0 {
		sources = DefaultLegacyJSONSources()
	}
	handlers := make(map[LegacyDomain]LegacyJSONHandler, len(options.Handlers))
	for _, handler := range options.Handlers {
		if handler == nil || strings.TrimSpace(string(handler.Domain())) == "" {
			return report, errors.New("旧 JSON handler 的业务域不能为空")
		}
		if _, exists := handlers[handler.Domain()]; exists {
			return report, fmt.Errorf("旧 JSON 业务域重复注册: %s", handler.Domain())
		}
		handlers[handler.Domain()] = handler
	}

	// 固定扫描顺序使迁移报告和跨领域依赖稳定，避免 map 顺序影响导入结果。
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].Domain == sources[j].Domain {
			return sources[i].FileName < sources[j].FileName
		}
		return sources[i].Domain < sources[j].Domain
	})
	var firstErr error
	for _, spec := range sources {
		item, importErr := s.importLegacyJSONFile(ctx, absDir, spec, handlers[spec.Domain])
		report.Items = append(report.Items, item)
		if importErr != nil && firstErr == nil {
			firstErr = importErr
		}
	}
	report.FinishedAt = time.Now().UTC()
	if firstErr != nil {
		report.Status = LegacyImportFailed
	} else {
		report.Status = LegacyImportNotFound
		for _, item := range report.Items {
			switch item.Status {
			case LegacyImportPending:
				report.Status = LegacyImportPending
			case LegacyImportImported:
				if report.Status != LegacyImportPending {
					report.Status = LegacyImportImported
				}
			case LegacyImportSkipped:
				if report.Status == LegacyImportNotFound {
					report.Status = LegacyImportSkipped
				}
			}
		}
		if options.ArchiveImported {
			if err := archiveImportedSources(absDir, report); err != nil {
				report.Status = LegacyImportFailed
				return report, err
			}
		}
	}
	return report, firstErr
}

func archiveImportedSources(sourceDir string, report LegacyImportReport) error {
	target := filepath.Join(sourceDir, "backups", "legacy-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	for _, item := range report.Items {
		if item.Status != LegacyImportImported && item.Status != LegacyImportSkipped {
			continue
		}
		if _, err := os.Stat(item.Path); err != nil {
			continue
		}
		if err := os.MkdirAll(target, 0o750); err != nil {
			return fmt.Errorf("创建旧 JSON 归档目录失败: %w", err)
		}
		if err := os.Rename(item.Path, filepath.Join(target, filepath.Base(item.Path))); err != nil {
			return fmt.Errorf("归档旧 JSON %s 失败: %w", item.Path, err)
		}
	}
	return nil
}

func (s *Store) importLegacyJSONFile(ctx context.Context, sourceDir string, spec LegacyJSONSourceSpec, handler LegacyJSONHandler) (LegacyImportItem, error) {
	item := LegacyImportItem{Domain: spec.Domain, Path: filepath.Join(sourceDir, filepath.Clean(spec.FileName))}
	if strings.TrimSpace(string(spec.Domain)) == "" || !validRelativeSource(spec.FileName) {
		item.Status = LegacyImportFailed
		item.Error = "旧 JSON 源定义无效"
		return item, errors.New(item.Error)
	}
	data, digest, err := readLegacyJSON(item.Path)
	if errors.Is(err, os.ErrNotExist) {
		item.Status = LegacyImportNotFound
		return item, nil
	}
	if err != nil {
		item.Status, item.Error = LegacyImportFailed, err.Error()
		return item, err
	}
	item.SHA256 = digest
	if handler == nil {
		item.Status = LegacyImportPending
		return item, nil
	}
	var exists int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM legacy_imports
		WHERE source_type = 'json' AND source_path = ? AND source_sha256 = ? AND domain = ? AND status = 'imported'`,
		item.Path, item.SHA256, string(item.Domain)).Scan(&exists)
	if err != nil {
		item.Status, item.Error = LegacyImportFailed, err.Error()
		return item, fmt.Errorf("检查旧 JSON 导入记录失败: %w", err)
	}
	if exists > 0 {
		item.Status = LegacyImportSkipped
		return item, nil
	}

	startedAt := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		item.Status, item.Error = LegacyImportFailed, err.Error()
		return item, err
	}
	count, err := handler.Import(ctx, tx, LegacyJSONSource{Domain: item.Domain, Path: item.Path, SHA256: item.SHA256, Data: data})
	if err == nil {
		item.ImportedCount = count
		item.Status = LegacyImportImported
		encoded, _ := json.Marshal(item)
		_, err = tx.ExecContext(ctx, `INSERT INTO legacy_imports
			(source_type, source_path, source_sha256, domain, status, imported_count, report_json, started_at, finished_at)
			VALUES('json', ?, ?, ?, 'imported', ?, ?, ?, ?)
			ON CONFLICT(source_type, source_path, source_sha256, domain) DO UPDATE SET
			status = excluded.status, imported_count = excluded.imported_count,
			report_json = excluded.report_json, error = '',
			started_at = excluded.started_at, finished_at = excluded.finished_at`,
			item.Path, item.SHA256, string(item.Domain), count, string(encoded), startedAt.Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	}
	if err != nil {
		_ = tx.Rollback()
		item.Status, item.Error = LegacyImportFailed, err.Error()
		s.recordFailedLegacyImport(ctx, item, startedAt)
		return item, fmt.Errorf("导入旧 JSON %s 失败: %w", item.Path, err)
	}
	if err := tx.Commit(); err != nil {
		item.Status, item.Error = LegacyImportFailed, err.Error()
		s.recordFailedLegacyImport(ctx, item, startedAt)
		return item, fmt.Errorf("提交旧 JSON %s 失败: %w", item.Path, err)
	}
	return item, nil
}

func (s *Store) recordFailedLegacyImport(ctx context.Context, item LegacyImportItem, startedAt time.Time) {
	encoded, _ := json.Marshal(item)
	_, _ = s.db.ExecContext(ctx, `INSERT INTO legacy_imports
		(source_type, source_path, source_sha256, domain, status, imported_count, report_json, error, started_at, finished_at)
		VALUES('json', ?, ?, ?, 'failed', 0, ?, ?, ?, ?)
		ON CONFLICT(source_type, source_path, source_sha256, domain) DO UPDATE SET
		status = excluded.status, report_json = excluded.report_json, error = excluded.error,
		started_at = excluded.started_at, finished_at = excluded.finished_at`,
		item.Path, item.SHA256, string(item.Domain), string(encoded), item.Error,
		startedAt.Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
}

func validRelativeSource(name string) bool {
	if strings.TrimSpace(name) == "" || filepath.IsAbs(name) {
		return false
	}
	clean := filepath.Clean(name)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func readLegacyJSON(path string) (json.RawMessage, string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", errors.New("旧 JSON 源必须是普通文件")
	}
	if info.Size() < 0 || info.Size() > maxLegacyJSONBytes {
		return nil, "", fmt.Errorf("旧 JSON 超出大小限制: %d", info.Size())
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxLegacyJSONBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxLegacyJSONBytes {
		return nil, "", errors.New("旧 JSON 超出大小限制")
	}
	if !json.Valid(data) {
		return nil, "", errors.New("旧 JSON 内容无效")
	}
	sum := sha256.Sum256(data)
	return json.RawMessage(data), hex.EncodeToString(sum[:]), nil
}
