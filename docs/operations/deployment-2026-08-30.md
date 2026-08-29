<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-08-30 部署记录

## 制品

- 目标平台：Linux amd64
- 构建提交：`80a3821` 及其父提交
- SHA256：`f6138253dfd60e1400711221e986b8087bbb0002c4cce8434461d629fad6db9d`
- 本地制品：`.tmp/workmesh-server-linux-amd64`

## 主节点旁路验证

主节点 `61.184.12.165:52834` 已完成以下非破坏性操作：

1. 同步二进制到 `/opt/workmesh-server/bin/workmesh-server`。
2. 刷新 `/etc/systemd/system/workmesh-server.service` 并执行 `daemon-reload`。
3. 使用 `WORKMESH_SERVER_ADDR=127.0.0.1:19999` 临时启动。
4. `/health` 与 `/ready` 均返回 `{"code":200}`。
5. 临时进程已停止；systemd 保持 `disabled/inactive`。

现有 `workmesh-node-core.service` 与 `workmesh-node-agent.service` 未停止，生产端口和数据保持不变。

## 切换阻断

- Gateway 登录用户名和密码尚未提供，无法完成节点注册、授权和心跳验收。
- `162.14.96.198` 当前运行 Gateway 服务，不是可确认的次节点；不能覆盖其现有进程。
- 主机无 `/dev/kvm` 且内存低于 CubeSandbox 建议值，只能以 degraded 控制面运行。

在补齐次节点 SSH/节点角色和 Gateway 凭据、完成主次同步与回滚演练前，不得执行旧服务卸载脚本或启用生产 systemd。

## 回滚

切换失败时保持旧服务运行；新单元可执行 `systemctl disable --now workmesh-server.service`，不删除 `/opt/workmesh-server` 数据目录。旧服务卸载必须显式设置 `WORKMESH_CONFIRM_OLD_UNINSTALL=REMOVE_OLD_WORKMESH_NODE`，并在人工验收后执行。
