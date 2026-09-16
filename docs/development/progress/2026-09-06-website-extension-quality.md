<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 网站扩展接口可读性整改（2026-09-06）

状态：`[x]` 已完成本批次

## 本批次变更

- 将 `website_extensions.go` 中网站扩展请求解析、兼容字段读取、ID/数字转换、记录复制和分页逻辑迁移到 `node/api/website_extensions_helpers.go`。
- 为迁移后的 7 个具名函数补充有业务含义的中文方法注释，说明请求体上限、兼容字段、分页上限和数据复制目的。
- 保留原函数签名、调用关系、`/api/v2` 路由注册、响应 envelope 和 SQLite 持久化流程；未引入 JSON 模拟业务数据。
- 原文件中的已有路由和存储逻辑未做语义调整；`registerWebsiteExtensionRoutes` 仍需后续批次继续拆分过长分发函数。
- 本次继续将显式 GET/默认页面处理器移至 `node/api/website_extensions_routes.go`，将统一 POST 分发器移至 `node/api/website_extensions_post.go`；入口文件仅保留路由编排。
- `website_extensions.go` 当前 16 行，`website_extensions_post.go` 366 行，`website_extensions_routes.go` 66 行，`website_extensions_helpers.go` 123 行；单文件均低于 500 行。

## 验证

```text
gofmt -w node/api/website_extensions.go node/api/website_extensions_post.go node/api/website_extensions_routes.go node/api/website_extensions_helpers.go
GOWORK=off go test ./node/api -run '^TestWebsiteExtension' -count=1
git diff --check -- node/api/website_extensions.go node/api/website_extensions_helpers.go
```

结果：代码格式化和差异检查通过；网站扩展定向测试因并行新增的 `node/api/apps_catalog_test.go` 存在未使用 import 而无法编译，未修改该代理文件。修复该独立编译阻断后应立即重跑定向测试。

## 风险和后续

- `registerWebsiteExtensionRoutes` 仍包含较长的 GET/POST 统一分发器，后续应按领域拆到独立的 handler 文件；拆分时必须保持 ServeMux 的通配路由优先级和持锁/持久化时序。
- 质量报告 `docs/inventory/quality-report.json` 尚未在本批次重新生成，旧报告中的注释计数不能作为本次结果依据。
