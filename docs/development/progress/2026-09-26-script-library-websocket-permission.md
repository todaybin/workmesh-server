<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 脚本库 WebSocket 权限修复

## 目标

修复脚本库点击执行后提示无权限并自动退出的问题，使运行协议与 1Panel 的 `GET/WS /api/v2/core/script/run` 兼容。

## 已完成

- [x] 管理员本地 Session 可执行已审核脚本；服务调用保留 `WORKMESH_COMMAND_TOKEN` 兼容入口。
- [x] `/api/v2/core/script/run` 支持非升级能力探测和受限 WebSocket 终端，持续返回输出并接受输入、心跳和窗口调整。
- [x] 系统脚本固定为已审核白名单；脚本仍必须通过 `script_id` 查找，不能提交任意命令。
- [x] WebSocket 路由加入 Server 和安全中间件的流式升级白名单，避免响应包装导致自动断开。
- [x] 更新脚本执行契约文档。

## 验证

- `gofmt -w node/api/core_resources.go node/api/script_system.go cmd/workmesh-server/main.go control/api/security_policy.go`
- `GOWORK=off go test ./node/api -run 'TestScriptRun|TestTerminal|TestStreamAuth' -count=1`

## 风险与未覆盖

- KVM、Docker、Firewall 等系统脚本会修改宿主机，未在本环境执行；部署前仍需管理员确认目标系统和资源条件。
- 未执行生产 `9999` 发布或线上脚本安装验收。
