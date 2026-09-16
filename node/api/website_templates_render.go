// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	websiteTemplateOutputTextLimit = 8 << 20
	websiteTemplateOutputFileLimit = 64 << 20
)

// renderWebsiteTemplateContent 替换模板正文中的变量，未提供变量时使用空字符串。
func renderWebsiteTemplateContent(content string, values map[string]string) string {
	return websiteTemplateVariablePattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := websiteTemplateVariablePattern.FindStringSubmatch(match)
		if len(parts) == 2 {
			return values[parts[1]]
		}
		return ""
	})
}

// readWebsiteTemplateIndex 从 ZIP 中读取最短路径的 index.html/index.htm 并渲染变量。
func readWebsiteTemplateIndex(path string, values map[string]string) (string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", errors.New("ZIP 模板文件无法读取")
	}
	defer reader.Close()
	var main *zip.File
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		name := strings.ToLower(filepath.Base(entry.Name))
		if (name == "index.html" || name == "index.htm") && (main == nil || len(entry.Name) < len(main.Name)) {
			main = entry
		}
	}
	if main == nil {
		return "", errors.New("ZIP 模板缺少 index.html")
	}
	if main.UncompressedSize64 > websiteTemplateOutputTextLimit {
		return "", errors.New("模板首页超过预览大小限制")
	}
	part, err := main.Open()
	if err != nil {
		return "", err
	}
	defer part.Close()
	content, err := io.ReadAll(io.LimitReader(part, websiteTemplateOutputTextLimit+1))
	if err != nil {
		return "", err
	}
	if len(content) > websiteTemplateOutputTextLimit {
		return "", errors.New("模板首页超过预览大小限制")
	}
	return renderWebsiteTemplateContent(string(content), values), nil
}

// renderWebsiteTemplateToDir 将单文件或 ZIP 模板渲染到产物目录。
func renderWebsiteTemplateToDir(template websiteTemplateRecord, values map[string]string, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return err
	}
	if template.Type == "single" {
		return os.WriteFile(filepath.Join(outputDir, "index.html"), []byte(renderWebsiteTemplateContent(template.Content, values)), 0o640)
	}
	reader, err := zip.OpenReader(template.FilePath)
	if err != nil {
		return errors.New("ZIP 模板文件无法读取")
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if err := renderWebsiteTemplateEntry(entry, outputDir, values); err != nil {
			return err
		}
	}
	return nil
}

// renderWebsiteTemplateEntry 安全解压并渲染 ZIP 条目，拒绝目录穿越和超大文件。
func renderWebsiteTemplateEntry(entry *zip.File, outputDir string, values map[string]string) error {
	cleanRoot, err := filepath.Abs(outputDir)
	if err != nil {
		return err
	}
	target := filepath.Join(cleanRoot, filepath.FromSlash(entry.Name))
	rel, err := filepath.Rel(cleanRoot, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("ZIP 模板路径无效: %s", entry.Name)
	}
	if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("ZIP 模板不允许符号链接: %s", entry.Name)
	}
	if entry.FileInfo().IsDir() {
		return os.MkdirAll(target, 0o750)
	}
	if entry.UncompressedSize64 > websiteTemplateOutputFileLimit {
		return fmt.Errorf("ZIP 模板文件过大: %s", entry.Name)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	part, err := entry.Open()
	if err != nil {
		return err
	}
	defer part.Close()
	content, err := io.ReadAll(io.LimitReader(part, websiteTemplateOutputFileLimit+1))
	if err != nil {
		return err
	}
	if len(content) > websiteTemplateOutputFileLimit {
		return fmt.Errorf("ZIP 模板文件过大: %s", entry.Name)
	}
	if websiteTemplateTextFile(entry.Name) {
		content = []byte(renderWebsiteTemplateContent(string(content), values))
	}
	return os.WriteFile(target, content, 0o640)
}
