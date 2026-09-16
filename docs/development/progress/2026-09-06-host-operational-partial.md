<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 主机运维代码拆分（2026-09-06）

状态：`[x]` 本批次结构拆分完成，待下一轮完整集成门禁覆盖。

## 当前变更

- 已将主机监控设置持久化、旧 JSON 一次性导入和原子文件保存逻辑移至 `node/api/host_operational_state.go`。
- 已将网卡/磁盘选项及监控设置读写路由移至 `node/api/host_monitor_interfaces.go`。
- 已将计划任务接口按职责移至 `node/api/cron_routes.go`，再分为 CRUD（创建/查询、更新/状态、分组/删除）、记录查询和执行/导入导出工具注册函数；各注册函数保持短小，便于人工审阅。
- `host_container_cron.go` 当前 260 行，`cron_routes.go` 274 行，`host_operational_state.go` 158 行，`host_monitor_interfaces.go` 84 行；所有文件低于 500 行。
- 已修复并行拆分中产生的重复 `hostOperationalStatePath` 定义及缺失 import；主机、容器、计划任务定向测试通过。
- 本批次未改变任何 `/api/v2/cronjobs/*` 路径、HTTP 方法、请求字段或响应封装；原有 17 条计划任务路由已逐条迁移至新注册函数。

## 验证

```text
gofmt -w node/api/host_container_cron.go node/api/cron_routes.go node/api/host_operational_state.go
GOWORK=off go test ./node/api -run 'Test.*(Host|Container|Cron|Dashboard)' -count=1
GOWORK=off go test ./node/service -run 'TestCronjob|TestNextRun' -count=1
git diff --check
```

## 下一步

已完成主机监控和计划任务路由拆分；下一步由唯一集成测试负责人重跑完整门禁，覆盖 `/api/v2/hosts/monitor/*`、防火墙状态、真实 `/proc` 读取行为以及计划任务 CRUD/记录/导入导出契约。计划任务真实执行仍依赖隔离的系统脚本和调度资源，未用模拟数据替代。
