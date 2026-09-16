<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-09-13 发布切换与自动更新

## 已完成

- 当前单二进制发布包：
  `/www/apps/workmesh-server/release/workmesh-server-linux-amd64`
- 发布包 SHA-256：
  `655207b820fa7e4c74ad1f10275ec428033c02dbfbfb60b9d24ef22b7e1c043a`
- `activate-release.sh` 现在会在替换后的意外错误、systemd 检查失败、服务重启
  失败或 HTTP/WAF 检查失败时尝试恢复旧二进制，并校验恢复后的摘要。
- 新增 `auto-update.sh`，支持 root 本地制品或 HTTPS 制品、Ed25519 验签、
  SHA-256 校验、并发锁、配置和 SQLite/WAL/SHM 备份、systemd 切换、健康检查
  和失败证据保留；支持签名 manifest 和版本降级拒绝。
- 新增 `workmesh-server-update.service` 和
  `workmesh-server-update.timer`。timer 每 6 小时运行一次；自动更新默认关闭。
- `install.sh --apply` 会安装发布切换脚本、自动更新脚本、更新配置示例和
  systemd service/timer。

## 当前未完成

- 本会话无法停止真实生产服务或替换 `/opt/workmesh-server/bin/workmesh-server`：
  当前 PID 1 是 Codex 沙箱，systemd D-Bus 返回 `Operation not permitted`，
  `/opt/workmesh-server/bin` 对会话不可写。
- 因此当前生产仍可能运行旧二进制，必须在真实宿主机执行激活命令并保存
  `systemctl`、运行摘要、`/health`、`/ready` 和 WAF 页面证据。
- Docker/OpenResty、域名 HTTPS、ACME、数据库容器和真实 WAF 拦截仍需在目标主机
  完成现场验收。

## 真实主机执行顺序

```bash
cd /www/apps/workmesh-server
systemctl stop workmesh-server.service
WORKMESH_SERVER_ROOT=/opt/workmesh-server \
WORKMESH_SERVER_BINARY=/www/apps/workmesh-server/release/workmesh-server-linux-amd64 \
WORKMESH_SERVER_SHA_FILE=/www/apps/workmesh-server/release/workmesh-server-linux-amd64.sha256 \
bash deploy/install/activate-release.sh
```

验证：

```bash
sha256sum /opt/workmesh-server/bin/workmesh-server
systemctl is-active workmesh-server.service
curl --fail http://127.0.0.1:9999/health
curl --fail http://127.0.0.1:9999/ready
systemctl is-enabled workmesh-server-update.timer
```

启用自动更新前，复制 `deploy/install/update.env.example` 到
`/opt/workmesh-server/config/update.env`，设置 root-only 公钥和制品源，确认
`WORKMESH_AUTO_UPDATE_ENABLED=1` 后再手动运行一次
`systemctl start workmesh-server-update.service`。不要把
`workmesh-server update version` 当作生产二进制更新命令。
