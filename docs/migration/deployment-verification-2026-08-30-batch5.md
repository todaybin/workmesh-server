<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-08-30 隐藏路由与清单验收

## 本批变更

- 路由扫描器展开旧 Agent helper 注册，纳入 `xpack/monitor` 和 `xpack/waf` 隐藏路由。
- 旧 Core 静态入口纳入基线：`/public/*filepath`、`/favicon.ico`、`/favicon.ico/*filepath` 和 `HEAD` 语义；Swagger 改为无品牌 `/swagger/*any`。
- 新服务注册 xpack 兼容别名，静态资源增加 `/public`、favicon 和无品牌 Swagger 处理。
- 新增隐藏能力扫描器，覆盖旧 Core/Agent 的 `init`、`middleware`、`i18n`、`log`、`cron` 目录，共 95 个源码文件。

## 验证证据

| 项目 | 结果 |
| --- | --- |
| 路由基线 | 870 条，`route-scan` 870/870 通过；允许 62 条新服务扩展路由 |
| 实现报告 | 870 条：以 `function-checklist-generated.md` 为准；xpack 新增别名标记 `compatibility` |
| 隐藏能力扫描 | init 38、middleware 14、i18n 26、log 12、cron 5，章节全部存在 |
| Go 测试 | `go test ./...` 通过 |
| Linux amd64 制品 | SHA256 `A6EE1B1E684852FE6F741CC594B76C8845202A6BFB0D5E75BD658A61579F2048` |
| 双节点制品 | 主/次 `systemd` 均为 `active`，远端二进制 SHA256 与上值一致 |
| 前端与节点闭环 | 主节点 HTML `text/html`、JS `text/javascript`；登录后 `nodes/add`、`nodes/list`、`nodes/del` 真实往返通过；两台 `/health`、`/ready` HTTP 200 |
| 隐藏路由冒烟 | xpack monitor/WAF、无品牌 Swagger 均非 404；公开 favicon 缺少文件时按 404 处理 |

## 生产部署状态

本批制品已替换到 `61.184.12.165:/opt/workmesh-server` 与 `162.14.96.198:/opt/workmesh-server-secondary`，两端均保留替换前二进制备份并已复验服务状态。Gateway 注册仍为 `registration=pending`，两台节点的 `server.env` 缺少有效 Gateway 登录/节点凭据，不能伪造注册成功状态；补齐凭据后需单独复验注册、heartbeat 和跨节点透传。
