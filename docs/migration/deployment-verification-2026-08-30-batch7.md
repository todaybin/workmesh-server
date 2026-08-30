<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-08-30 前端同步与最终双节点验收

## 已部署制品

| 制品 | SHA256 | 目标 |
| --- | --- | --- |
| 后端 Linux amd64 | `26BD606B53369D442A2B43F615537E33ACA6141CA3B2AC5BEAA52F0C5BA15FA3` | 主 `/opt/workmesh-server/bin/workmesh-server`；次 `/opt/workmesh-server-secondary/bin/workmesh-server` |
| 前端 `web/dist` 归档 | `5C00592E1FF62173FA5EFA65A8F3E9DAC3881C61F954A7D1248D25F18EB52AB7` | 主/次 `web/dist`，旧目录均已备份 |

## 公网验收

- 主、次 `/health` 和 `/ready` 均 HTTP 200。
- 主节点首页返回 `text/html; charset=utf-8`。
- 当前首页引用的 JS 返回 `text/javascript; charset=utf-8`，不再出现 MIME 为 `application/json` 的模块加载错误。
- `admin/admin` 登录成功；临时次节点执行 `nodes/add -> nodes/list -> nodes/del` 往返成功，测试数据已删除。
- AI Agent 创建、列表、概览、删除往返成功。
- xpack Monitor/WAF 和 `/swagger/index.html` 均不返回 404。

## 未完成授权项

`/api/v2/workmesh/gateway/status` 在主、次节点均为 `registration=pending`，原因是 Gateway 返回 `WORKMESH_NODE_NOT_FOUND`。节点服务和本机多节点管理已经可用，但云端 Gateway 注册/心跳/任务透传仍需有效 Gateway 用户会话或节点 Bootstrap 凭据；未提供凭据前不伪造注册成功状态。

