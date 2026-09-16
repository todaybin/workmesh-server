<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 1Panel HTTP 路由基线（2026-09-05）

## 扫描结果

- 来源：`/www/apps/1Panel`，仅读取 `core/router`、`agent/router`、`core/init/router`、`agent/init/router` 下 Go 源码。
- 生成命令：`node test/contract/route-scan.mjs generate --legacy /www/apps/1Panel --out docs/inventory/route-inventory-1panel.json`。
- 精确路由数：`759`（去重后的 HTTP 方法 + 路径组合）。
- 方法分布：`GET=138`、`POST=617`、`HEAD=4`；当前扫描未发现 `PUT/PATCH/DELETE/OPTIONS/Any`。
- 输出清单：`docs/inventory/route-inventory-1panel.json`。
- 清单字段包含 `originalPath`、`originalVersion`、`versionMigration`、`sourceSHA256` 和 `status`；原始私有组前缀从 `/www/apps/1Panel/*/init/router/router.go` 的 `PrivateGroup` 读取，不把所有路由预设为 v2。
- 当前状态：所有条目初始标记为 `not-run`；本次仅完成源码路由清点，没有请求 1Panel 或 WorkMesh 服务。

## v2 路径规则

- Core 路由按 `/api/v2/core` 展开，Agent 路由按 `/api/v2` 展开。
- 初始化路由保留源码注册的原始路径，例如 `/health`、`/ready`、静态资源和根页面路径。
- 1Panel Swagger 品牌前缀按扫描器既有兼容规则映射为 `/swagger/*any`。
- 清单中的 `source` 是相对当前工作区的源码来源位置，不能作为运行时 API 地址。

## 扫描限制

- 扫描器解析 Gin 的显式 `Group`、常见 helper、链式 `Group/Use`、标准库 `HandleFunc`、项目显式 `register/waf` 以及已知 `StaticFS` 特殊路径。
- 动态拼接路径、运行时反射注册、未覆盖的自定义注册器、WebSocket 升级目标、SSE/下载流和参数 `type/operate` 的业务分支不由本清单推导，必须在接口矩阵中另行登记。
- 清单只证明“源码路由存在”，不证明 handler 实现、鉴权、SQLite 副作用、响应契约或真实功能可用。
- 后续对 `/www/apps/1Panel/frontend` 的调用扫描需单独合并，用于补充页面实际使用的请求参数、WS 路径和同一路由多操作变体。

## 验收状态字段

接口测试记录使用 `not-run | pass | fail | blocked | skipped`。只有具备真实请求、脱敏响应、状态码、持久化/外部副作用和失败分支证据时，才能改为 `pass`。
