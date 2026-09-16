package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const fileLineLimit = 500
const functionLineLimit = 80

type violation struct {
	Rule   string `json:"rule"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol,omitempty"`
	Actual int    `json:"actual,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type sourceRecord struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Lines  int    `json:"lines"`
}

type scanReport struct {
	Schema       int            `json:"schema"`
	GeneratedAt  string         `json:"generatedAt"`
	Root         string         `json:"root"`
	Status       string         `json:"status"`
	Files        int            `json:"files"`
	Functions    int            `json:"functions"`
	Rules        map[string]int `json:"rules"`
	Counts       map[string]int `json:"counts"`
	SkippedRoots []string       `json:"skippedRoots"`
	SkippedFiles []string       `json:"skippedFiles"`
	Sources      []sourceRecord `json:"sources"`
	Violations   []violation    `json:"violations"`
}

// scanRoot 检查项目全部 Go 源码，包含测试、生成源码和所有平台构建分支。
func scanRoot(root string) (scanReport, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return scanReport{}, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return scanReport{}, err
	}
	if !info.IsDir() {
		return scanReport{}, fmt.Errorf("检查根目录不是目录: %s", absolute)
	}
	report := scanReport{Schema: 1, GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Root: absolute, Status: "pass", Counts: map[string]int{},
		Rules:        map[string]int{"file_lines": fileLineLimit, "function_lines": functionLineLimit},
		SkippedRoots: []string{}, Sources: []sourceRecord{}, Violations: []violation{}}
	report.SkippedFiles = []string{}
	err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(absolute, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if path != absolute && entry.IsDir() && excludedDirectory(entry.Name()) {
			report.SkippedRoots = append(report.SkippedRoots, relative)
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			if entry.Type()&os.ModeSymlink != 0 {
				report.add(violation{Rule: "source_symlink", File: relative, Detail: "源码符号链接需显式核验，不跟随链接"})
				return nil
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if isGeneratedSource(contents) {
				report.SkippedFiles = append(report.SkippedFiles, relative)
				return nil
			}
			scanFile(relative, contents, &report)
		}
		return nil
	})
	if report.Files == 0 && len(report.SkippedFiles) == 0 && err == nil {
		return report, fmt.Errorf("未发现 Go 源文件: %s", absolute)
	}
	return report, err
}

// isGeneratedSource 仅豁免 Go 工具标准生成标记，历史代码和普通注释不在豁免范围。
func isGeneratedSource(contents []byte) bool {
	header, err := parser.ParseFile(token.NewFileSet(), "", contents, parser.PackageClauseOnly|parser.ParseComments)
	return err == nil && ast.IsGenerated(header)
}

// excludedDirectory 仅排除依赖、版本管理和运行产物，不按历史债或文件名豁免源码。
func excludedDirectory(name string) bool {
	switch name {
	case ".git", ".cache", ".tmp", "node_modules", "vendor", "data", ".workmesh-data":
		return true
	}
	return false
}

// add 保留每条违规并按规则统计，确保历史问题仍会阻断门禁。
func (report *scanReport) add(item violation) {
	report.Violations = append(report.Violations, item)
	report.Counts[item.Rule]++
	report.Status = "fail"
}

// physicalLines 按物理行计数，文件末尾换行不额外产生空行。
func physicalLines(contents []byte) int {
	if len(contents) == 0 {
		return 0
	}
	lines := bytes.Count(contents, []byte("\n"))
	if contents[len(contents)-1] != '\n' {
		lines++
	}
	return lines
}

// scanFile 利用 Go AST 定位声明与匿名函数，字符串或注释内的伪函数不会计数。
func scanFile(name string, contents []byte, report *scanReport) {
	report.Files++
	lines := physicalLines(contents)
	report.Sources = append(report.Sources, sourceRecord{name, fmt.Sprintf("%x", sha256.Sum256(contents)), lines})
	if lines > fileLineLimit {
		report.add(violation{Rule: "file_lines", File: name, Line: 1, Actual: lines, Limit: fileLineLimit})
	}
	positions := token.NewFileSet()
	source, err := parser.ParseFile(positions, name, contents, parser.ParseComments|parser.AllErrors)
	if err != nil {
		report.add(violation{Rule: "parse_error", File: name, Detail: err.Error()})
	}
	if source == nil {
		return
	}
	ast.Inspect(source, func(node ast.Node) bool {
		switch function := node.(type) {
		case *ast.FuncDecl:
			report.Functions++
			symbol := function.Name.Name
			if function.Recv != nil && len(function.Recv.List) > 0 {
				symbol = receiverName(function.Recv.List[0].Type) + "." + symbol
			}
			if function.Doc == nil || !containsChinese(function.Doc.Text()) {
				report.add(violation{Rule: "chinese_doc", File: name,
					Line: positions.PositionFor(function.Pos(), false).Line, Symbol: symbol,
					Detail: "具名函数或方法须有紧邻声明的中文说明"})
			}
			checkFunction(name, symbol, function, positions, report)
		case *ast.FuncLit:
			report.Functions++
			checkFunction(name, "<anonymous>", function, positions, report)
		}
		return true
	})
}

// checkFunction 从 func 关键字到结束括号计物理行，包含签名、空行与内部注释。
func checkFunction(file, symbol string, node ast.Node, positions *token.FileSet, report *scanReport) {
	start := positions.PositionFor(node.Pos(), false).Line
	end := positions.PositionFor(node.End(), false).Line
	lines := end - start + 1
	if lines > functionLineLimit {
		report.add(violation{Rule: "function_lines", File: file, Line: start,
			Symbol: symbol, Actual: lines, Limit: functionLineLimit})
	}
}

// containsChinese 识别汉字说明；语义质量仍需评审，检查器不会自动生成注释。
func containsChinese(text string) bool {
	for _, character := range text {
		if unicode.Is(unicode.Han, character) {
			return true
		}
	}
	return false
}

// receiverName 提供普通、指针及泛型接收者的可定位名称。
func receiverName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return "*" + receiverName(value.X)
	case *ast.IndexExpr:
		return receiverName(value.X)
	case *ast.IndexListExpr:
		return receiverName(value.X)
	}
	return "<receiver>"
}
