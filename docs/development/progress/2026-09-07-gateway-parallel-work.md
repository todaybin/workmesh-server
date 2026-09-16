<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Gateway 并行任务拆分（2026-09-07）

## 目标

验证主站点与次站点之间的注册、心跳、签名、epoch/fencing、任务透传和断线恢复。所有测试使用真实 SQLite 与本地 TCP；没有节点凭据时不得伪造跨节点成功。

## 子任务

| ID | 边界 | 任务 | 交付证据 |
| --- | --- | --- | --- |
| GATEWAY-LOCAL-01 | `runtime/link` 内存/SQLite store | 注册幂等、节点角色、epoch 单调递增、旧 epoch 拒绝 | 定向 Go 测试和状态转移表 |
| GATEWAY-LOCAL-02 | `runtime/link` 本地 TCP | 签名请求、心跳超时、重放请求、断线重连和资源释放 | 本地 TCP 测试、关闭码和连接计数 |
| GATEWAY-LOCAL-03 | `control/api` Gateway handler | v2 请求/响应 envelope、错误码、权限和 CSRF 边界 | HTTP httptest 与路由矩阵 |
| GATEWAY-LOCAL-04 | 任务透传 | 任务创建、幂等键、完成/失败回写、旧 epoch 任务拒绝 | 临时 SQLite 任务记录和日志断言 |
| GATEWAY-REAL-01 | 真实主/次节点 | 注册、心跳和签名链路 | 脱敏请求/响应、节点状态和时间戳 |
| GATEWAY-REAL-02 | 真实跨节点任务 | 透传、超时、重试、fencing 和恢复 | 两端 SQLite、任务日志、真实 stderr |

## 执行顺序

1. 先并行执行 `GATEWAY-LOCAL-01`～`04`，不接触生产网络和 systemd。
2. 四项本地测试通过后，由主控统一执行一次 Go 全量/race/vet。
3. 获得主次节点凭据和隔离网络后，再执行 `GATEWAY-REAL-01`、`02`；每一步失败立即停止并保存证据。
4. 真实联调完成前，Gateway 状态保持 `partial/not-run`，不能以本地测试替代生产通过。

## 资源与禁止项

- 需要：主节点管理员会话、次节点地址和凭据、可回收的测试任务、维护窗口。
- 禁止：修改 `/www/apps/1Panel`、写生产 SQLite、替换生产二进制、重启 systemd、创建不可回收容器。
- 唯一集成负责人：主控。子任务只运行定向测试并提交证据。
