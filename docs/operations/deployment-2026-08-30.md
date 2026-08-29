<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-08-30 部署记录

## 制品

- 目标平台：Linux amd64
- 构建提交：`78c29b0`、`c4fa306`（包含节点链路、签名、防重放、fencing 和传输防护；角色共享测试为 `6389bb0`）
- SHA256：`AF45C6EF1C0B793A018C625B40603EEB66E6F96A1FE229E367EFC14AD90C57AE`
- 本地制品：`.tmp/workmesh-server-linux-amd64`

## 主节点旁路验证

主节点 `61.184.12.165:52834` 已完成以下非破坏性操作：

1. 新制品已同步到 `/opt/workmesh-server/bin/workmesh-server`，远端 SHA256 与本地 `AF45C6...C57AE` 一致。
2. 已部署 `/opt/workmesh-server/web/dist` 前端产物并启用 `workmesh-server.service`（`0.0.0.0:9999`）；前端旧品牌标识和外链已清理。
3. `/health`、`/ready`、应用目录、运行时和站点接口均返回 HTTP 200；服务常驻内存约 7.2 MiB。
4. 已停止并禁用 `workmesh-node-core.service`、`workmesh-node-agent.service`，并移除其 systemd 单元和旧二进制；`/opt/workmesh` 数据目录保留。

旧 `workmesh-node-core.service` 与 `workmesh-node-agent.service` 已停止、禁用并移除；旧 `/opt/workmesh` 数据目录保持不变。

5. 新增链路 `handshake`、`heartbeat`、`sync` 和 `fencing` 接口已在远端返回成功；`/api/v2/link/status` 显示主节点角色为 `primary`、`roleEpoch=1`。

## 次节点部署

`162.14.96.198` 同时承担 Gateway，因此采用独立目录 `/opt/workmesh-server-secondary` 和独立单元 `workmesh-server-secondary.service` 共存部署，未修改 Gateway 或 Nginx。次节点监听 `:9999`，角色为 `secondary`，健康与就绪接口均返回 HTTP 200，制品 SHA256 与主节点一致，内存约 18 MiB。

## 切换阻断

- Gateway 登录用户名、密码及节点授权材料尚未提供，节点状态目前为 `pending`（Gateway 返回 `WORKMESH_NODE_NOT_FOUND`），无法完成真实注册、授权和心跳验收。
- Gateway 到节点的注册仍缺少账号/授权材料；次节点已部署但状态为 `pending`。
- 主节点到次节点 `162.14.96.198:9999` 的网络端口当前不可达，需要在云安全组或受控反向代理中放行后才能进行跨机链路验收。
- 主机预检缺少 `/dev/kvm` 且内存约 3.8 GiB，只能以 degraded/restricted 控制面运行，不能标记 CubeSandbox E2E。
- `162.14.96.198` 的 Gateway 服务保持 active，未覆盖其现有进程。

在补齐次节点 SSH/节点角色和 Gateway 凭据、完成主次同步与回滚演练前，不得宣称多节点生产验收完成。

## 回滚

切换失败时可从 `/root/workmesh-pre-switch-20260830-020622.tar.gz` 恢复旧程序和单元；新单元可执行 `systemctl disable --now workmesh-server.service`，不删除 `/opt/workmesh-server` 数据目录。旧数据目录 `/opt/workmesh` 仍保留。
