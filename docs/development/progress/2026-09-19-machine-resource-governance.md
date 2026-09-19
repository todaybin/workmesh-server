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

## 2026-09-19 部署验收

- [x] 已从提交 `0803bdb` 重新构建前端和 Linux amd64 单二进制，发布包 SHA-256 为 `480506778d98fa9e28772322979e5cdb2315364cb6c25b093c9b7c6c95f7512b`。
- [x] 已通过 `activate-release.sh` 原子替换 `/opt/workmesh-server/bin/workmesh-server`，旧版本已保留为 `workmesh-server.bak.20260919184416-3674262`；systemd MainPID 为 `3674426`。
- [x] `/health`、`/ready`、9999 端口归属和 `/advanced/waf` 前端资源检查通过；运行二进制内嵌版本为 `0803bdb`。
- [x] 部署后稳定空闲采样：RSS/PSS `31.0 MiB`、匿名 PSS `8.4 MiB`、file-backed PSS `22.6 MiB`、Swap `0`、线程 `10`、FD `12`。
- [x] 与部署前同口径基线（RSS/PSS `32.3 MiB`、匿名 PSS `9.2 MiB`、线程 `11`、FD `13`）相比，RSS/PSS 下降约 `1.3 MiB`（`3.9%`），匿名 PSS 下降约 `0.8 MiB`（`8.8%`）；本次为空闲态采样，不代表任务峰值。
- [!] Gateway PostgreSQL 前向迁移、真实 Gateway 登录/注册/心跳和任务峰值资源采样仍需独立维护窗口完成。

## 边界与安全结论

- `site_id` 和账号 UID 仅来自 Gateway 登录 JWT，面板不提交、选择或生成站点归属。
- 同一 Gateway 账号可管理多台机器；一个机器码只能绑定一个 Gateway 账号。
- `/www/apps/workmesh-server` 内部主/子节点继续只用于本地负载均衡和数据库集群，本任务未修改其角色、选举、fencing 或链路协议。
- 本轮 Gateway 仍只提交前向迁移和源码，未执行生产 PostgreSQL 迁移；WorkMesh Server 已按原子发布流程完成二进制替换和 systemd 重启，未修改 Supervisor、Docker 或本机主/子节点选举链路。

## 验证

- `GOWORK=off go test ./runtime/machineid ./runtime/resources ./runtime/gateway ./control/api ./node/service ./cmd/workmesh-server`：通过。
- `GOWORK=off go test ./node/api`：仅两个既有网站代理断言失败。
- `git diff --check`：通过。

## 2026-09-19 资源构成诊断增强

- [x] Linux cgroup v2 资源快照新增 `anon`、`file`、`inactive_file` 和 `swap` 字段；仍按请求/心跳即时读取，不创建常驻采样器，也不执行 `drop_caches`、`swapoff` 或强制换页。
- [x] 主机诊断摘要新增 `cgroupMemoryAnon`、`cgroupMemoryFile`、`cgroupMemoryInactiveFile` 和 `cgroupMemorySwap`，可区分匿名应用内存、可回收文件缓存和实际换出页。
- [x] 新增 cgroup `memory.stat` 解析边界测试，主机诊断测试增加字段契约断言；资源包定向测试、主机诊断定向测试、全仓编译和 `go vet ./...` 通过。
- [!] 本次修改尚未部署到 `/opt/workmesh-server`；线上当前仍运行 `480506778d98fa9e28772322979e5cdb2315364cb6c25b093c9b7c6c95f7512b` 对应的已部署版本，需单独确认后再执行发布激活。
