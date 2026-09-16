<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 测试职责与执行规则

## 目的

为避免多个智能体重复占用服务器资源，开发与测试严格分离：开发智能体只修改代码或文档，集成测试智能体统一执行测试并生成证据。

## 职责分工

| 角色 | 允许操作 | 不允许操作 |
| --- | --- | --- |
| 开发智能体 | 修改后端代码、SQLite 迁移、文档、测试用例；执行格式化和静态阅读 | 不得自行启动全量 Go、race、Node 合约、前端构建或生产重启 |
| 集成测试智能体 | 统一执行 Go 测试、race、vet、Node 合约、路由契约、SQLite 完整性、部署后健康检查 | 不修改业务代码，不修改 `apps/1Panel/frontend`，不伪造业务结果 |
| 主协调智能体 | 分配任务、审核差异、处理失败、批准最终部署 | 不重复执行集成测试负责人已经完成的同一批测试 |

## 后端变更后的唯一测试顺序

1. `GOWORK=off go test ./...`
2. `GOWORK=off go test -race ./...`
3. `GOWORK=off go vet ./...`
4. `git diff --check`
5. `node --test test/contract/*.test.mjs`
6. `node test/contract/route-scan.mjs check ...`
7. SQLite 只读完整性、服务健康和 WAF 检查

只有集成测试负责人可以将结果写入 `progress/integration-test-latest.md`。

## 前端规则

`apps/1Panel/frontend` 是只读参考项目，用于提取菜单、HTTP/WS 路由和参数契约。前端没有发生改动时，不执行 `npm run type-check` 或 `npm run build:pro`；不得因为后端变更重复编译前端。

## 真实业务验收

登录后的 385 条 HTTP、4 条 WS、六类运行环境、网站设置、主次节点和 Let's Encrypt HTTP-01 必须使用真实凭据、真实 SQLite、真实 Docker/OpenResty/ACME 资源执行。缺少外部资源时记录为 `blocked` 或 `not-run`，禁止使用 JSON fixture、固定成功响应或模拟空数组替代。
