<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Website Template Store 结构质量整改（2026-09-07）

状态：`[x]` 本批次完成

## 本批次变更

- `node/api/website_templates_store.go` 从 719 行降至 417 行，保留模板/产物 SQLite CRUD、真实渲染、预览和路径安全校验。
- 新增 `node/api/website_templates_migration.go`（228 行），集中承载关系表迁移、旧 `website_extension_state` JSON 一次性导入和兼容字段解析。
- 新增 `node/api/website_templates_repository_base.go`（98 行），集中承载 API DTO、仓储初始化和幂等建表。
- 保持 `/api/v2` 路由、请求/响应字段、SQLite 表名与索引、真实模板文件渲染及目录安全约束不变；未修改 `/www/apps/1Panel`、前端或生产环境。

## 验证

```text
gofmt -w node/api/website_templates_store.go node/api/website_templates_migration.go node/api/website_templates_repository_base.go
go test ./node/api -run 'WebsiteTemplate|Template' -count=1
git diff --check -- node/api/website_templates_store.go node/api/website_templates_migration.go node/api/website_templates_repository_base.go
```

结果：相关 `node/api` 模板测试通过（1.395s），差异检查通过。全量门禁交由唯一集成测试负责人在源码冻结后执行。

## 备注

- 旧 JSON 只作为一次性迁移输入，运行态模板和产物仍统一从 SQLite 与真实文件目录读取。
- 删除模板/产物时的真实目录清理和路径归属校验保持原有行为。
