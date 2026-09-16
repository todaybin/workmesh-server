<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# SEC-01 安全与 API 契约审计（2026-09-08）

本轮为只读审计，未修改认证中间件、业务路由或 `/www/apps/1Panel`。扫描脚本：
`test/contract/security-contract-scan.mjs`。

## 兼容占位接口

`node/api/legacy_routes_*.go` 当前注册 735 个 `fallbackRouteHandler`：GET 116、POST 619。
按路径第一级域统计如下：

| 域 | 数量 | 域 | 数量 |
| --- | ---: | --- | ---: |
| websites | 120 | ai | 102 |
| containers | 70 | hosts | 69 |
| databases | 53 | files | 45 |
| toolbox | 39 | core | 38 |
| runtimes | 29 | settings | 26 |
| backups | 20 | apps | 31 |
| alert | 14 | cronjobs | 12 |
| logs | 11 | openresty | 9 |
| workmesh | 6 | cubesandbox | 5 |
| config | 4 | deployment/groups/sites | 7（合计） |
| 其余单路径域 | 23 |  |  |

这些接口运行时应返回 HTTP 501、`details.errCode=NOT_IMPLEMENTED`，不得伪造成功或写入 JSON 模拟数据。上线门禁必须按菜单逐项替换为真实处理器，或在产品范围中明确下线；仅存在注册不代表功能完成。

## 鉴权与流式边界

- `X-WorkMesh-Forwarded: 1` 不能作为信任凭据。外层安全中间件将其交给 `NodeRelay`，由节点透传层校验 HMAC、时间戳、nonce、role epoch；缺失/伪造签名必须拒绝。现有 `node/api/node_relay_test.go` 已覆盖缺失或过期签名。
- WS/SSE 自认证白名单在 `control/api/security_policy.go` 与 `cmd/workmesh-server/main.go` 各维护一份；本轮扫描确认两处均为 6 条：`process/ws`、容器日志 SSE、wget 进度、local/container/ssh terminal。后续新增流式路由必须同步两处并补测试。
- 链路 `GET /api/v2/link/status` 当前处理器未调用 `s.authenticate`，在配置共享密钥时仍可被未签名读取；该项列为高风险待修复，不在本轮修改范围。修复要求：与 handshake/heartbeat/pull/push 一致，在 secret 非空时校验时间戳、nonce、签名和节点身份，并增加未签名 401 回归测试。

## 可执行门禁

```bash
node test/contract/security-contract-scan.mjs
GOWORK=off go test ./control/api ./runtime/link ./node/api -run 'TestSecurityMiddleware|TestNodeRelay|TestLinkServer' -count=1
```
扫描脚本当前断言 fallback 数量仍为 735、WS/SSE 白名单一致、伪造透传回归测试存在；使用 `--strict` 时会在 link status 尚未签名前失败，便于修复后纳入 CI。
