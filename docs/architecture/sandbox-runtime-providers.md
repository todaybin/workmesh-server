<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Sandbox Runtime Provider 架构

## 目标边界

WorkMesh Server 持有 Sandbox 生命周期、任务工作区和运行结果。Firecracker、gVisor 或其他运行器只能作为 Server 注入的 Runtime Provider；Agent 和项目不能直接访问宿主执行接口。ForgeVM 可以作为兼容适配层或测试运行器，但不单独构成安全边界。

Linux 生产节点优先使用 Firecracker microVM。没有 KVM 的受限节点可选择经黑盒验收的 gVisor `runsc`；无合格隔离 Provider 时，任务创建返回不可用，不回退到宿主进程或普通 Docker。

## Provider 契约

Server 使用 `workmesh.sandbox.v1` 固定生命周期协议，并要求能力探测和逐任务执行回执。Provider 必须以不可变二进制摘要登记，通过参数数组调用，不得经 Shell 拼接。创建回执须匹配任务的工作区、隔离级别、网络策略及 CPU、内存、PID、磁盘上限。

Firecracker Provider 还须在任务启动前完成以下工作：

- 为每个任务创建独立 VMM、API socket、guest 工作盘和生命周期句柄。
- 绑定 kernel、rootfs、KVM 设备及项目工作区，并限制 CPU、内存、PID、磁盘和执行时间。
- 使用受控 vsock guest agent 执行任务命令，Server API 不得直接执行宿主命令。
- 将 guest 网络接入独立网络后端，默认拒绝外网；配置 allowlist 时在宿主网络边界执行。
- 使用 cgroup v2 限制 VMM/guest 进程树，并在取消、超时、崩溃和重启恢复时有界回收。
- 收集有界日志和 Artifact，清理任务临时盘；工作区、共享 cache 和 Artifact 按各自保留策略管理。

## 当前实现状态

`taskruntime` 已有通用 CLI 协议、能力/逐任务回执校验、项目 workspace 布局和 Linux cgroup v2 适配层。Firecracker 当前可校验固定资产与路径、生成单任务 machine-config JSON、生成 argv 启动计划，并编解码有界 guest 请求帧；这些逻辑不创建 workspace image、磁盘 quota、网络设备或 VMM。它尚未连接 vsock agent、执行命令、恢复 VMM 状态或回收 guest 进程，因此不声明任何可执行隔离能力，也不能用于生产任务。

gVisor 直连 OCI `runsc` Provider 尚未实现。当前通用 CLI 的能力字段仍来自 CLI 自述，必须由隔离黑盒测试验证后才可作为生产准入依据。ForgeVM 社区项目不能替代这些验证，也不得通过 Docker Socket 接入不可信任务。

## 启用门槛

Firecracker 进入可执行状态前须同时满足：

1. 节点有可访问的 `/dev/kvm`、固定摘要的 Firecracker、受审查 kernel/rootfs 和 delegated cgroup v2。
2. 已实现逐任务 VMM 配置、vsock 执行代理、网络隔离、磁盘 quota、进程树限制和重启 inspect。
3. 超 CPU/内存/PID/磁盘/时限、工作区逃逸、网络绕过、guest 退出、Server 重启和清理失败测试均通过。
4. CLI 能力与创建回执来自真实执行设置，不能因配置文件存在而声明能力。
5. 完成隔离节点发布与回滚演练后，才可由运维配置显式启用。

本机缺少 `/dev/kvm` 时只运行 Provider 单元测试；不得宣称 Firecracker E2E 已通过。开发和验证不得连接生产 `9999`、操作 Docker Socket、重启网站或切换线上服务。
