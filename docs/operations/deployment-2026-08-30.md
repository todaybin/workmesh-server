<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-08-30 部署记录

## 制品

- 目标平台：Linux amd64
- 构建提交：`5711e93`（包含 Compose、运行时、终端、SSH、toolbox、应用目录和 sites 接口）
- SHA256：`EFF04589400C6E6EA1A8A0AD9C437F213DEA45B9DA9F89FD3CE9D477327B0F29`
- 本地制品：`.tmp/workmesh-server-linux-amd64`

## 主节点旁路验证

主节点 `61.184.12.165:52834` 已完成以下非破坏性操作：

1. 新制品已同步到 `/opt/workmesh-server/bin/workmesh-server`，远端 SHA256 与本地 `EFF045...B0F29` 一致。
2. 已部署 `/opt/workmesh-server/web/dist` 前端产物并启用 `workmesh-server.service`（`0.0.0.0:9999`）。
3. `/health`、`/ready`、应用目录、运行时和站点接口均返回 HTTP 200；服务常驻内存约 6.4 MiB。
4. 已停止并禁用 `workmesh-node-core.service`、`workmesh-node-agent.service`，并移除其 systemd 单元和旧二进制；`/opt/workmesh` 数据目录保留。

现有 `workmesh-node-core.service` 与 `workmesh-node-agent.service` 未停止，生产端口和数据保持不变。

## 切换阻断

- Gateway 登录用户名和密码尚未提供，节点状态目前为 `pending`，无法完成注册、授权和心跳验收。
- `162.14.96.198` 当前是 Gateway 主机，未提供可覆盖的次节点地址；未在该主机上部署 Node，避免影响 Gateway。
- 主机预检缺少 `/dev/kvm` 且内存约 3.8 GiB，只能以 degraded/restricted 控制面运行，不能标记 CubeSandbox E2E。
- `162.14.96.198` 当前运行 Gateway 服务，不是可确认的次节点；不能覆盖其现有进程。
- 主机无 `/dev/kvm` 且内存低于 CubeSandbox 建议值，只能以 degraded 控制面运行。

在补齐次节点 SSH/节点角色和 Gateway 凭据、完成主次同步与回滚演练前，不得执行旧服务卸载脚本或启用生产 systemd。

## 回滚

切换失败时可从 `/root/workmesh-pre-switch-20260830-020622.tar.gz` 恢复旧程序和单元；新单元可执行 `systemctl disable --now workmesh-server.service`，不删除 `/opt/workmesh-server` 数据目录。旧数据目录 `/opt/workmesh` 仍保留。
