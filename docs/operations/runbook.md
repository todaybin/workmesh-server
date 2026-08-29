<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 运维与验收手册

## 安装与启动

每台主机安装同一版本独立二进制，配置 `WORKMESH_SERVER_ADDR`、`WORKMESH_DATA_DIR`、`WORKMESH_NODE_ID`、`WORKMESH_NODE_ROLE` 和 Gateway 地址。服务默认监听 `:9999`，由 systemd 或等效进程管理器托管。

配置 `WORKMESH_GATEWAY_URL`、`WORKMESH_GATEWAY_ID`、`WORKMESH_GATEWAY_SECRET`、`WORKMESH_GATEWAY_USERNAME` 和 `WORKMESH_GATEWAY_PASSWORD` 后，服务启动会先换取 Gateway JWT，再自动注册当前节点，并每 30 秒发送一次心跳。注册失败时节点保持 `pending` 状态并记录原因；Gateway 不可用不会阻塞 `/health`，但云端任务必须等待授权恢复。

## 首次启用

先完成本机账号登录，再使用 Gateway 账号完成当前节点注册。主节点 `61.184.12.165` 必须注册；`162.14.96.198` 当前是 Gateway 主机，不得覆盖部署为次节点。真正次节点地址确定后，必须为每个节点分别注册，不可由主节点代注册。注册成功前只显示健康、登录和授权页面。

## 多机验收

在主节点多机管理中填写次节点 IP、端口和通信凭据，验证握手、证书、能力、心跳、增量同步和任务状态。执行主转次、次转主、重复请求、网络中断、旧主恢复、凭据轮换和失败回滚；确认 `role_epoch` fencing 阻止双主写入。

## 故障处理

Gateway 暂时不可达时，已注册节点继续执行本机功能，云端任务进入等待或失败重试状态。未注册节点不得绕过授权。排查时优先查看结构化日志中的 request ID、node ID、role epoch 和任务 ID，不记录密码、令牌或私钥。
