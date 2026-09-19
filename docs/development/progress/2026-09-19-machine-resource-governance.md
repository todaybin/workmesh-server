<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-09-19 节点机器身份与资源生命周期治理

## 状态

- [x] Gateway 设备身份已与面板内部主/子节点拓扑分离；设备 ID 由本机机器码派生，内部 `primary/secondary` 不上传。
- [x] Linux、Windows 和其他平台已提供版本化硬件指纹采集；Linux 排除 bridge、veth、Docker、隧道和虚拟网卡。
- [x] Gateway 注册、心跳、本地 SQLite 绑定和 Ed25519 身份已关联机器码；镜像复制或硬件身份变化会停用旧 Token、bindingId 和签名身份并要求重新登录。
- [x] 心跳复用既有周期采集进程与 cgroup 资源快照，Gateway 软策略支持 `observe/enforce`，且只能收紧本地硬上限。
- [x] 计划任务无启用项时不创建固定 ticker，改为唤醒 channel 和最近到期单次 timer；无自动续期证书时不创建证书续期 goroutine。
- [x] 机器身份、Gateway 协议、控制面、资源策略、计划任务和启动入口定向测试通过。
- [!] 完整 `node/api` 仍有两个既有网站代理测试依赖已停止创建的 `nginx/proxy/root.conf`，与本任务无关。

## 边界与安全结论

- `site_id` 和账号 UID 仅来自 Gateway 登录 JWT，面板不提交、选择或生成站点归属。
- 同一 Gateway 账号可管理多台机器；一个机器码只能绑定一个 Gateway 账号。
- `/www/apps/workmesh-server` 内部主/子节点继续只用于本地负载均衡和数据库集群，本任务未修改其角色、选举、fencing 或链路协议。
- 本轮只提交前向迁移和源码，不执行生产 PostgreSQL 迁移，不修改 systemd、Supervisor、Docker 或生产服务。

## 验证

- `GOWORK=off go test ./runtime/machineid ./runtime/resources ./runtime/gateway ./control/api ./node/service ./cmd/workmesh-server`：通过。
- `GOWORK=off go test ./node/api`：仅两个既有网站代理断言失败。
- `git diff --check`：通过。
