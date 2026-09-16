<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Gateway 本地链路就绪（2026-09-07）

状态：`[x] 本地专项通过，真实主次节点联调阻塞`。

本批次覆盖 `GATEWAY-LOCAL-01`～`04`，只使用进程内 `httptest`、本地 TCP 和临时 SQLite；未访问生产 Gateway、未写入生产库、未修改 `/www/apps/1Panel`，未执行全量门禁。

## 交付范围

| 子任务 | 验收内容 | 结果 |
| --- | --- | --- |
| GATEWAY-LOCAL-01 | SQLite 角色状态持久化、epoch 单调递增、任务流 CAS | 通过 |
| GATEWAY-LOCAL-02 | 本地 TCP HMAC 握手、nonce 重放拒绝、节点身份一致性、超时、断线重连和连接释放 | 通过 |
| GATEWAY-LOCAL-03 | `/api/v2/workmesh/gateway/register` v2 envelope、重复注册幂等、其他节点冲突、权限和 CSRF | 通过 |
| GATEWAY-LOCAL-04 | 任务创建/完成/失败快照、幂等键重试、旧 epoch 拒绝、当前 epoch 回写、SQLite 持久化 | 通过 |

## 代码约束

- `SyncCursor` 新增可选 `roleEpoch`；普通控制面同步可以省略以保持现有 v2 客户端兼容。
- `task`、`tasks`、`task.*` 和 `tasks.*` 流必须携带当前 `roleEpoch`，旧 epoch 或缺失 epoch 返回 HTTP 409，不写入 SQLite。
- 任务状态继续以同步流的真实 SQLite `link_sync` 表保存；相同游标和相同内容重试视为幂等，不增加版本。
- 握手、心跳在启用共享密钥时要求签名节点头与正文 `nodeId` 一致，防止有效密钥被用于冒充其他节点。
- Gateway 重复注册返回与首次注册相同的 `registered/gatewayUrl/nodeId/status/bindingId` v2 数据结构；访问令牌不会出现在响应。

## 定向验证

```text
gofmt -w runtime/link/contract.go runtime/link/http_client.go runtime/link/server_handlers.go runtime/link/task_fencing.go runtime/link/gateway_local_test.go control/api/gateway_register.go control/api/gateway_contract_test.go
GOWORK=off go test ./runtime/link ./control/api -run 'GatewayLocal|GatewayV2Registration|Link|Gateway|Fenc|Sync' -count=1
ok   github.com/todaybin/workmesh-server/runtime/link  0.263s
ok   github.com/todaybin/workmesh-server/control/api   0.048s
```

新增测试文件：

- `runtime/link/gateway_local_test.go`
- `control/api/gateway_contract_test.go`

## 仍未完成

- `[!] GATEWAY-REAL-01`：没有已批准的真实 Gateway 地址、节点注册凭据和隔离网络，未执行注册、心跳、签名和断线恢复。
- `[!] GATEWAY-REAL-02`：没有可回收的主/次节点和真实任务，未执行跨节点任务透传、超时、重试、fencing 和恢复。
- `[ ]` 主控需在本批次源码冻结后统一执行一次 `go test ./...`、Race、Vet、Node 契约和路由门禁；本专项未运行这些命令。

节点 HTTP 透传已增加受控断线恢复：GET/HEAD/OPTIONS 以及带 `Idempotency-Key` 的写请求可在传输错误后重试，普通写请求不会自动重放；每次尝试使用新的 nonce 和签名。定向测试位于 `node/api/node_relay_test.go`，真实双节点网络仍需单独验收。

Gateway 本地 HTTP 客户端新增断线恢复回归：`TestGatewayHeartbeatDisconnectRecoveryKeepsBinding` 模拟注册成功、Gateway 暂时不可达和恢复，确认绑定保留、状态转离线后通过心跳恢复且不会重复注册。

本地通过证据不能替代生产上线结论；在真实凭据和维护窗口到位前，Gateway 状态必须继续标记为 `blocked/not-run`。
