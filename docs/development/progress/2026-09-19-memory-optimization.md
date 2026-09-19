<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-09-19 单进程内存治理

## 状态

- [x] 已确认主要常驻内存来自 `functional_domain_state.logs`（59,752 条、约 25.7 MB JSON）和启动即加载的应用目录（264 条、约 3.7 MB JSON）。
- [x] `domainState` 不再保存日志，启动不再预读 1,000 条操作日志；任务日志和状态只以 SQLite 任务表为事实来源，告警日志只查询 `operation_logs` 的告警来源。
- [x] 应用目录拆到 `app_catalog_cache`，启动只加载已安装应用、忽略项和商店设置；目录首次访问加载，空闲 `limits.cacheTTL` 后主动释放，已安装应用保存不再序列化目录。
- [x] `limits` 已接入任务、文件转换、SSE、AI、任务日志最大读取量和目录 TTL；文件任务日志扫描只保留请求页，`latest` 使用固定大小窗口。
- [x] pprof 直接写入 gzip 响应；诊断摘要新增 heap/sys/released/GC、RSS/PSS、Go memory limit 与 cgroup current/high/max。
- [x] 新增 `maintenance reset-history --confirm-delete-history`：清理指定历史表、功能域旧日志、旧目录缓存、Cron 运行记录和 WorkMesh 自有任务/服务日志，并执行 checkpoint/VACUUM；不创建备份，不触碰账户和业务资源表。
- [x] 新增 systemd `20-resources.conf` 安装来源，限制 Go heap、cgroup 内存和 swap。
- [x] 60k 日志清理、业务配置保留、目录懒加载/过期、只读不写和限额耗尽定向测试已通过；`go vet ./...`、编译门禁、性能微基准、`git diff --check` 和安装脚本语法检查通过。
- [x] 已停止生产服务，使用新二进制执行 `maintenance reset-history --confirm-delete-history`；历史表清零、旧 JSON 日志/目录清除、任务日志删除、WAL checkpoint/VACUUM 完成，账号/站点/业务配置保留。
- [x] 已部署新二进制（SHA-256 `c87fe3044b4cbf8313da48e48885a883d92928a86ebc324f96ca50d8345748d6`），安装 `20-resources.conf` drop-in 并重启；`/health`、`/ready` 返回 200。
- [x] 部署后空闲采样：RSS/PSS 约 33 MiB，cgroup current 约 10 MiB、peak 约 13 MiB，swap current 为 0；数据库从约 58 MiB 缩至约 652 KiB。
- [!] 完整 `go test ./node/api ./cmd/workmesh-server` 仍有两个既有网站代理用例失败（当前分支停止生成 `nginx/proxy/root.conf`，与本次内存改动无关）；内存治理及 CLI 定向测试通过。

## 生产验收

部署前再次确认没有真实安装、备份或证书任务运行。历史任务表中现有 `running` 状态可能是陈旧记录，不能仅凭状态字段判断。生产清理必须在服务停止后，从 `/opt/workmesh-server` 工作目录执行候选二进制并显式传入确认参数。

验收阈值：空闲 RSS <= 120 MiB；常用工作流后稳定 RSS < 150 MiB；活动峰值 < 220 MiB；cgroup < 256 MiB；空闲 CPU < 1%；Goroutine/FD 无持续增长。历史数据删除不可回滚，代码和 systemd 配置可以回滚。
