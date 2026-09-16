<!-- SPDX-License-Identifier: GPL-3.0-only -->
# Gateway 与 HTTP/WS 黑盒轮换任务

范围仅限 `runtime/link`、`control/api` 契约测试和脱敏执行模板。不得修改 `apps/1Panel`，不得连接生产主次节点，不得使用固定成功或 JSON 模拟数据。

拆分为四个可独立领取的原子项：

1. 注册/心跳：真实 SQLite 状态、epoch/fencing、重复注册与过期节点拒绝。
2. v2 HTTP：签名、时间戳、nonce、CSRF、错误 envelope 和旧 epoch。
3. WS 终端：本地 TCP 测试握手、身份校验、命令输出、关闭码和资源释放。
4. 黑盒模板：登录后 `/api/v2` 请求方法、字段、状态码、SQLite/配置文件校验点。

每个原子项只运行自身定向测试；全部冻结后由主控统一执行全量门禁。真实主次节点联调、生产写操作和重启保持 `not-run`，直到获得维护窗口与凭据。
