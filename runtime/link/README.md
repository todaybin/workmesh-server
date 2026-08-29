<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 节点通信边界

节点之间只同步控制面声明的数据和版本游标，不覆盖对端 Docker、数据库、文件、进程和正在执行的本地任务。连接应使用 HTTPS；开发环境可以使用 HTTP 测试服务器。节点共享密钥只用于 HMAC 签名，不能写入日志或接口响应。

## 控制面接口

`Server.Register` 将以下接口挂载到统一 `http.ServeMux`，每个接口同时提供 `/api/v2/link/*` 和 `/api/v2/workmesh/link/*` 两个兼容前缀：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | `/handshake` | 交换节点 ID、角色 epoch、协议版本和能力 |
| POST | `/heartbeat` | 更新远端在线状态 |
| GET | `/status` | 查看本机角色及最近心跳节点 |
| POST | `/sync/pull` | 按 `stream + version` 拉取最新快照 |
| POST | `/sync/push` | 以 compare-and-set 游标写入快照 |
| POST | `/fencing/check` | 检查角色切换的 epoch 和目标角色 |
| POST | `/fencing/prepare` | 锁定一个待提交的角色切换操作 |
| POST | `/fencing/commit` | 校验待提交操作并递增本机 epoch |
| POST | `/fencing/abort` | 释放待提交的角色切换操作 |

`/fence/{action}` 是 fencing 路径的兼容别名。fencing 的 `prepare`、`commit`、`abort` 只允许通过同一个 `role.Manager` 修改角色，旧 `roleEpoch` 会返回 HTTP 409，从而阻止双主写入。

## 请求签名

配置 `WORKMESH_LINK_SECRET` 后，除公开状态查询外的链路请求必须带以下请求头：

* `X-WorkMesh-Node-ID`
* `X-WorkMesh-Timestamp`（Unix 秒，也兼容 RFC3339）
* `X-WorkMesh-Nonce`
* `X-WorkMesh-Signature`

签名原文固定为 `METHOD + "\n" + PATH + "\n" + TIMESTAMP + "\n" + NONCE + "\n" + BODY`，使用 HMAC-SHA256 后编码为小写十六进制。服务端默认只接受 5 分钟内的时间戳，同一节点的 nonce 在 10 分钟内不能重复。客户端每次重试都会重新生成时间戳、nonce 和签名。

## 增量同步

同步流名称仅允许字母、数字、点、下划线和短横线，最长 128 字节；请求体最长 8 MiB。`push` 使用游标 compare-and-set：游标落后且内容不同返回 HTTP 409 `同步游标冲突`，相同内容的重复提交视为幂等成功。`pull` 没有新版本时返回空 `payload` 和当前游标。

## 客户端重试

`NewHTTPClient` 默认单次超时 10 秒、最多重试 2 次，指数退避从 100ms 开始。网络错误、408、425、429 和 5xx 会重试；调用方取消上下文后立即停止。需要测试或定制传输时使用 `NewHTTPClientWithOptions` 注入 `HTTPDoer`、时钟、nonce 生成器和重试参数，其中 `MaxRetries: 0` 表示不重试。
