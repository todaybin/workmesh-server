# GATEWAY-03 链路 status 鉴权

日期：2026-09-08
状态：`[x] 已完成`

## 实现

- `GET /api/v2/link/status` 与 `/api/v2/workmesh/link/status` 在配置链路密钥时复用 `authenticate`。
- GET 空请求体参与签名，仍要求 `X-WorkMesh-Timestamp`、`X-WorkMesh-Nonce`、`X-WorkMesh-Signature` 和 `X-WorkMesh-Node-ID`。
- 缺失/错误签名返回 `401`，nonce 重放返回 `409`，保持链路统一 `{"code":"ERR"}` envelope。
- 未配置 secret 时保持本地开发匿名访问兼容行为。

## 验证

```text
GOWORK=off go test ./runtime/link -run 'Test(Link|Gateway|Status|Auth)' -count=1
```

结果：`ok github.com/todaybin/workmesh-server/runtime/link`。

覆盖：匿名拒绝、有效 HMAC、兼容前缀、nonce 重放、secret 为空的开发模式。

## 未覆盖

- 未执行真实 Gateway、生产 systemd 或跨节点网络测试。
- 未执行全量 Go 测试和竞态测试，由主控统一安排。
