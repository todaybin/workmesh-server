<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 仪表盘与系统监控接口

新服务在单进程内提供旧版 `/api/v2/dashboard/*` 路径。接口统一返回 `{code: 200, data: ...}`，数据来源为当前节点本机运行时，不依赖数据库或远程 Agent。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | `/api/v2/dashboard/base/os` | 操作系统、架构和内核信息 |
| GET | `/api/v2/dashboard/base/{ioOption}/{netOption}` | 主机基础信息与当前资源快照 |
| GET | `/api/v2/dashboard/current/{ioOption}/{netOption}` | 当前 CPU、负载、内存和设备快照 |
| GET | `/api/v2/dashboard/current/node` | 当前节点快照 |
| GET | `/api/v2/dashboard/current/top/cpu` | CPU 进程排行 |
| GET | `/api/v2/dashboard/current/top/mem` | 内存进程排行 |
| GET | `/api/v2/dashboard/quick/option` | 快捷入口 |
| POST | `/api/v2/dashboard/quick/change` | 更新快捷入口（轻量节点内存态） |
| GET | `/api/v2/dashboard/app/launcher` | 应用启动器列表 |
| POST | `/api/v2/dashboard/app/launcher/option` | 启动器选项 |
| POST | `/api/v2/dashboard/app/launcher/show` | 更新启动器显示状态 |
| POST | `/api/v2/dashboard/system/restart/{operation}` | 接收重启请求并返回 accepted |

Linux 节点优先读取 `/proc/loadavg`、`/proc/meminfo` 和 `/proc/uptime`；Windows 等平台保留字段并返回可用的运行时信息。重启接口不会在 HTTP 请求线程中终止服务，实际生命周期操作应由部署管理器执行。
