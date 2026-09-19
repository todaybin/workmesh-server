<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 性能基线记录

性能结论必须基于同一主机、同一功能开关和同一负载下的实测。至少记录：空闲 RSS、Goroutine、CPU、SQLite 连接数、HTTP 长连接数、启动耗时、节点切换延迟、同步吞吐和重试恢复时间。

微基准运行：

```powershell
go test ./test/perf -run '^$' -bench . -benchmem -count=3
```

双进程 Core+Agent 与单进程 WorkMesh Server 的实测结果追加到本文件，并注明系统版本、Go 版本、提交号、配置摘要和采样时间。不能将预计节省比例当作验收结论。

## 2026-09-19 内存治理基线

治理前生产只读采样（容器不计入）：WorkMesh Server 进程 RSS 约 183-257 MiB，cgroup current 约 236-311 MiB，历史 cgroup peak 约 741 MiB。`functional_domain_state.payload` 为 25,685,860 字节，其中 59,752 条 `logs` 占 25,683,467 字节；`app_store_state.payload` 为 3,744,685 字节，其中 264 条应用目录占 3,736,308 字节。该结构会在设置/告警更新时复制，并在保存时完整序列化，是匿名内存和峰值的主要来源。

本轮验收口径：空闲 RSS 不超过 120 MiB，常用工作流后稳定 RSS 小于 150 MiB，活动峰值小于 220 MiB，cgroup 小于 256 MiB，空闲 CPU 小于 1%，Goroutine 和文件描述符无持续增长。systemd 使用 `GOMEMLIMIT=128MiB`、`GOGC=50`、`MemoryHigh=192M`、`MemoryMax=256M`、`MemorySwapMax=64M`。部署后必须在启动、常用工作流和目录缓存过期三个时点补记实测值；当前不能把目标值写成已达成结果。

## 2026-09-19 部署后实测

- 构建与部署：Linux amd64 单二进制，SHA-256 `c87fe3044b4cbf8313da48e48885a883d92928a86ebc324f96ca50d8345748d6`；systemd drop-in 已加载。
- 启动后约 20 秒：RSS 约 92 MiB，cgroup current 约 92 MiB，cgroup peak 约 143 MiB。
- 历史清理并重启后空闲约 1 分钟：RSS/PSS 约 33 MiB，cgroup current 约 10 MiB，cgroup peak 约 13 MiB，swap current 为 0，10 秒窗口 CPU 为 0%，文件描述符 13 个、线程 9 个。`/health` 与 `/ready` 均返回数字 `code=200`。
- 数据规模：`workmesh.db` 从约 58 MiB 降至约 652 KiB；`functional_domain_state.payload` 为 2,105 字节，`app_store_state.payload` 为 181 字节；历史任务/操作/登录表为 0，`schema_migrations` 保留 18 条，启动后新增的 `migration_runs` 仅为 4 条审计记录。
- 以上是当前主机实测，不代表在高并发任务、目录首次加载或真实网站代理请求下已完成峰值验收；后续仍需在维护窗口补充认证管理流、应用目录首次加载和任务/SSE 工作流的三阶段采样。
