<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Go 质量门禁

`main.go` 和 `scan.go` 提供无第三方依赖的 AST 检查器。它递归扫描 Go 源文件，检查物理文件不超过 500 行、函数（含匿名函数）不超过 80 行，以及具名函数/方法是否有中文紧邻注释。历史问题不豁免，检查器不会自动插入注释。

```text
GOWORK=off GOCACHE=$PWD/.cache/go-build go run ./test/quality -root . -out docs/inventory/quality-report.json
```

退出码 `0` 表示通过，`1` 表示规则违规，`2` 表示扫描或报告错误。JSON 报告包含源码 SHA-256、规则计数和逐条失败位置。
