<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-08-30 AI 状态接口与双节点验收

## 本批变更

- Agent 列表从固定空数组改为读取 `ai.json` 持久化状态。
- Agent 渠道绑定状态从渠道配置和 Agent 的 `channels` 字段派生。
- Agent 概览返回真实 Agent、账号、渠道、Skill、任务和会话计数。
- Agent 删除检查返回实际引用对象。
- 实现扫描器支持 `--manifest` 并按 GET/HEAD 语义识别静态资源，避免误报。

## 验证结果

| 项目 | 结果 |
| --- | --- |
| Go 测试与 vet | `go test ./...`、`go vet ./...` 通过 |
| 路由契约 | 870/870 通过 |
| 实现状态 | 870 条：`implemented 475`、`partial 226`、`compatibility 169` |
| Linux amd64 制品 | SHA256 `26BD606B53369D442A2B43F615537E33ACA6141CA3B2AC5BEAA52F0C5BA15FA3` |
| 双节点服务 | 主/次 systemd 均 `active`，远端二进制哈希一致 |
| 前端 | 首页 `text/html`，JS `text/javascript` |
| 节点管理 | 登录后添加、列表查询、删除次节点真实往返通过 |
| AI Agent | 创建、列表、概览、删除真实往返通过 |
| 隐藏路由 | xpack Monitor/WAF 与无品牌 Swagger 均非 404 |

## 未闭环项

两台节点 Gateway 状态仍为 `registration=pending`，Gateway 返回 `WORKMESH_NODE_NOT_FOUND`。远端缺少有效 Gateway 登录/节点授权凭据；补齐凭据后必须复验注册、heartbeat 和跨节点任务透传。剩余 `partial/compatibility` 接口必须继续按逐路由清单完成真实副作用验收。

