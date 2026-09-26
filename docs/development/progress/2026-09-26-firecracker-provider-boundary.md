<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Firecracker Provider 接入边界

## 任务协议

- 目标：按推荐方案为 Server 建立 Firecracker Provider 的可审查边界，不把 ForgeVM 当作安全边界。
- 范围：`node/service/taskruntime`、Sandbox 架构文档、开发状态及主仓工作台账。
- 输入契约：固定的 Firecracker/kernel/rootfs/KVM/workspace/cgroup 路径和有界 vCPU、内存、磁盘参数；启动参数通过 argv 传递。
- 输出契约：路径/资产准入结果和每任务 VMM 启动参数计划；未实现 guest 执行链时返回 `FirecrackerUnavailableError`。
- 触达资源：WorkMesh Server Sandbox Provider、内部架构文档和进度台账。
- 风险等级：L3 RuntimeProvider 隔离边界；用户已明确批准按推荐方案执行。本切片不启用生产运行器。
- 人工审批：实际执行权限、网络/磁盘隔离策略及生产发布仍需对应阶段审批。
- 验收命令：`go test ./node/service/taskruntime`、`go vet ./node/service/taskruntime`、目标 diff 检查。

## 结果

- 增加 Firecracker 固定路径、kernel/rootfs、KVM 和资源参数校验；启动计划仅返回固定 argv，不经过 Shell。
- 增加单任务 machine-config 生成器：rootfs 只读、workspace image 可写、固定 boot args、可选预创建网络设备和 vsock UDS；workspace image 必须位于受控 workspace root 内。
- 增加 `FirecrackerTaskBackend` 接口边界和单任务 machine-config 生成器：rootfs 只读、workspace image 可写、CPU/内存、固定 boot args、可选预创建网络设备和 vsock UDS；workspace image 必须位于受控 workspace root 内。配置生成不创建镜像，也不施加磁盘 quota。
- 增加 guest 请求/响应帧编解码：4 字节大端长度前缀、1 MiB 帧上限、操作白名单、argv 数量/长度限制、拒绝截断及尾随 JSON；此处不打开 vsock 连接。
- guest agent 服务、vsock 实际连接、网络后端、磁盘 quota、inspect 和回收未实现，能力探测及所有执行生命周期仍失败关闭，不会把 JSON 或协议测试误报为隔离能力。
- 增加架构说明，明确 Firecracker、runsc、ForgeVM 的边界和进入生产前的验收门槛。
- 未修改默认 `forgevm/gvisor` 策略、CLI 装配、公开 API、网站配置或生产服务；本机 `/dev/kvm` 缺失，不执行 Firecracker E2E。

## 验证

- `node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server test ./node/service/taskruntime -count=1 -timeout 180s`：通过。
- `node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server vet ./node/service/taskruntime`：通过。
- `node scripts/with-dev-env.mjs -- env GOWORK=off GOOS=windows GOARCH=amd64 go -C apps/workmesh-server test ./node/service/taskruntime -c`：通过；临时交叉编译产物已清理。
- `git -C apps/workmesh-server diff --check`：通过。
- 本机 `/dev/kvm` 不存在，因此未启动 Firecracker、gVisor、ForgeVM 或任何线上服务；未操作生产 `9999`、网站进程、Docker Socket。

## 后续

1. 将 machine-config 与真实 workspace image/磁盘 quota 创建器接线，并在创建失败时回滚临时资产。
2. 实现 guest agent 服务及 Server 侧 vsock 连接，补请求取消、时限、响应关联和有界流读取。
3. 接通 cgroup v2、独立网络 allowlist、真正磁盘 quota 与任务树回收。
4. 在隔离 KVM 节点运行超限、逃逸、网络及重启恢复黑盒测试。
5. 独立实现无需 Docker Socket 的 gVisor OCI Provider，并把 ForgeVM 保持为适配/测试选项。
