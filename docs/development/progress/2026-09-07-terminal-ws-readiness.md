<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 终端 WebSocket 定向就绪记录

日期：2026-09-07

## 范围

本批次只覆盖 WorkMesh Server 本地终端 WebSocket，不修改 `/www/apps/1Panel`，不执行生产 Docker、SSH、systemd 或数据库写操作。前端调用契约核对确认使用以下 v2 端点：

- `GET /api/v2/hosts/terminal/local`
- `GET /api/v2/hosts/terminal/container`
- `GET /api/v2/hosts/terminal/ssh`

前端使用浏览器原生 RFC6455 WebSocket，连接参数包含 `cols`、`rows`，输入帧为 JSON `type=cmd` + base64，窗口变化为 `type=resize`，心跳为 `type=heartbeat`。

## 实现与修复

- 握手现在校验 `Connection: Upgrade`、`Sec-WebSocket-Version: 13` 和 16 字节随机 nonce，避免接受不完整或伪造的升级请求。
- 跨来源握手明确返回 HTTP 403 和 `WEBSOCKET_ORIGIN_DENIED`。
- WebSocket 关闭操作改为幂等，多个清理路径不会重复写关闭帧或重复关闭连接。
- 本地终端命令退出后等待输出泵完成，再关闭会话，确保短命令的最后输出不会被截断。
- 会话完成时主动关闭网络连接，读帧循环不会因客户端不再发送数据而长期占用。

## 定向测试

命令：

```text
GOWORK=off go test ./node/api -run '^TestTerminalWebSocket' -count=5
```

结果：通过，3 个 TCP 闭环测试重复 5 次；覆盖真实本地 shell 输出、RFC6455 101 响应及 `Sec-WebSocket-Accept`、令牌认证、Origin 拒绝、版本/nonce 校验、掩码输入、命令回显和关闭释放。

补充定向协议测试：

```text
GOWORK=off go test ./node/api -run 'TestTerminalWebSocket|TestStreamAuthAndOrigin|TestWebSocket' -count=1
```

其中既有 `TestTerminalResizeCallbackAndBounds` 使用 `net.Pipe` 且无读端，单测会等待其 30 秒读超时后通过；这属于既有测试实现，不代表生产终端等待。新增 TCP 测试耗时约 0.03 秒。

## 状态

本地终端 WebSocket 协议与资源释放达到 `partial/ready-for-integration`。容器终端依赖真实 Docker，SSH 终端依赖真实主机凭据，跨节点 WS 和生产黑盒仍未验收；不得据此宣称 P3 全部上线。全量 Go/race/vet 门禁由主控统一执行。
