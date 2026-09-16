<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 质量门禁与路由基线

日期：2026-09-05

## 状态

- [x] 新增无依赖 Go AST 质量检查器：`test/quality/`。
- [x] 生成机器可读失败报告：`docs/inventory/quality-report.json`。
- [x] 生成 `/www/apps/1Panel` 的 HTTP 路由基线：`759` 条，初始接口状态为 `not-run`。
- [x] `route-scan.mjs generate` 已增加路径保护：`--legacy` 目录只读，禁止将清单输出到参考项目目录内。
- [x] 新增前端 HTTP/WS 静态调用清单：`389` 条（HTTP `385`、WS `4`），全部初始为 `not-run`。
- [x] 为 AST 检查器补充边界、中文注释、匿名函数、生成文件和解析错误单测。
- [x] 为路由扫描器补充真实版本、原路径、源码哈希和 `v1-to-v2` 判定单测。
- [!] 2026-09-05 最新质量检查发现 `1300` 条违规：文件/函数长度和具名函数中文注释问题，未用历史债豁免，也未自动插入假注释；`files_routes.go` 已从 2295 行降至 86 行，`files_advanced_handler.go` 已降至 484 行，`apps.go`、`containers.go`、Dashboard 和 AI 执行模块也已按职责拆分，但中文注释债务仍需继续清理。
- [>] 已执行健康、鉴权边界、安全入口和站点入口 HTTP smoke；759 条业务路由与 389 条前端调用仍按 `not-run` 管理，尚未伪造为全量通过。

## 实测命令

`GOWORK=off GOCACHE=$PWD/.cache/go-build go run ./test/quality -root . -out docs/inventory/quality-report.json`

结果：退出码 `1`，`Go 文件=254`、`函数=2491`、`违规=1300`（当前仍包含大量 `chinese_doc`、`file_lines` 和 `function_lines` 违规）。

`node test/contract/route-scan.mjs generate --legacy /www/apps/1Panel --out docs/inventory/route-inventory-1panel.json`

结果：生成 `759` 条清单，方法分布 `GET=138`、`POST=617`、`HEAD=4`。

`node test/contract/frontend-inventory.mjs --frontend /www/apps/1Panel/frontend --out docs/inventory/frontend-api-inventory.json`

结果：提取 `389` 条调用字面量，HTTP `385`、WS `4`；排除 `node_modules/dist`，保留每个源文件 SHA-256。

`GOWORK=off GOCACHE=$PWD/.cache/go-build go test ./test/quality` 与 `node --test test/contract/route-scan.test.mjs` 均通过。

## 参考项目只读边界

`apps/1Panel/frontend` 和 `/www/apps/1Panel` 仅用于读取菜单、HTTP/WS 调用和旧路由注册，不能作为 WorkMesh 的工作副本，也不能写入生成文件、补丁或测试结果。`route-scan.mjs check` 只读取 `--legacy` 与 WorkMesh 清单并扫描当前项目；`generate` 的输出必须位于 WorkMesh 项目之外的参考目录，脚本会拒绝写入 `--legacy` 内部路径。

## 后续

由协调者按报告规则分批修复规范债务；修复业务源码须绑定对应领域任务，不在本任务中自动重排函数或批量补注释。HTTP/WS 真实结果记录见 [`2026-09-05-http-ws-test-matrix.md`](2026-09-05-http-ws-test-matrix.md)，后续先覆盖登录后网站、运行时、任务日志和终端 WS，再扩展到全量菜单和参数变体。
