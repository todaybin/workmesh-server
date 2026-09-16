<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 节点链路 API

节点链路用于主节点和次节点之间的控制面握手、心跳、增量同步以及角色 fencing。接口由单进程服务注册，不启动额外的 HTTP 服务。

## 路由

| 方法 | 路径 | 成功数据 |
| --- | --- | --- |
| `POST` | `/api/v2/link/handshake` | 远端 `nodeId`、`role`、`roleEpoch`、能力和协议版本 |
| `POST` | `/api/v2/link/heartbeat` | `accepted` 和本机角色快照 |
| `GET` | `/api/v2/link/status` | 本机 `current` 和最近 `peers` |
| `POST` | `/api/v2/link/sync/pull` | `payload`（JSON base64）和最新 `cursor` |
| `POST` | `/api/v2/link/sync/push` | 写入后的 `cursor` |
| `POST` | `/api/v2/link/fencing/check` | `ready` 和本机角色快照 |
| `POST` | `/api/v2/link/fencing/prepare` | `status=prepared` |
| `POST` | `/api/v2/link/fencing/commit` | 递增后的角色快照 |
| `POST` | `/api/v2/link/fencing/abort` | `status=aborted` |

每条路径也提供 `/api/v2/workmesh/link/*` 兼容前缀，fencing 还兼容 `/api/v2/link/fence/*`。成功响应使用数字 `code: 200`，失败响应使用 `code: "ERR"` 和 HTTP 错误状态。

## 身份认证

设置 `WORKMESH_LINK_SECRET` 后，除 `GET /status` 外的接口必须带 `X-WorkMesh-Node-ID`、`X-WorkMesh-Timestamp`、`X-WorkMesh-Nonce` 和 `X-WorkMesh-Signature`。签名算法是 HMAC-SHA256，签名原文为：

```text
METHOD + "\n" + PATH + "\n" + TIMESTAMP + "\n" + NONCE + "\n" + BODY
```

服务端允许 5 分钟时钟偏差，同一节点 nonce 的有效期为 10 分钟。客户端传输失败或收到 408、425、429、5xx 时按指数退避重试，并为每次重试重新签名。

## 角色 fencing

请求体使用 `role.Transition`：`operationId`、`nodeId`、`from`、`to` 和 `expectedEpoch`。`prepare` 会锁定操作 ID，`commit` 只提交当前待处理操作；epoch 不匹配、节点身份不匹配或目标角色非法均返回 409。`role.Manager` 与 `/api/v2/core/nodes/role` 共用，因此切换后的 epoch 会立即反映到控制面查询。

## 同步游标

请求体的 `stream` 只允许字母、数字、`.`、`_`、`-`，最长 128 字节；同步数据最大 8 MiB。任务流（`task`、`tasks`、`task.*`、`tasks.*`）必须同时携带当前 `roleEpoch`，旧 epoch 或缺少 epoch 返回 409，防止角色切换后旧节点继续回写任务。落后游标写入不同内容返回 409，重复写入相同内容视为幂等成功，调用方应在冲突后先 `pull` 再合并。
