<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-09-23 首页概览与主机监控

## 目标

首页系统信息、CPU、负载、磁盘和流量与 1Panel 口径一致。主机监控页能显示历史曲线，监控设置能生效。

## 状态

- [x] 已完成内核版本、系统类型、CPU 差值、负载公式、磁盘 Bfree/inode 和指定分区 IO。
- [x] 已完成监控设置契约、历史表 `0017-host-monitor`、采样循环和 search/clean。
- [x] 已完成定向测试：`node/api` 的 `TestHostMonitor|TestDashboard|TestHostOperationalRoutes|TestHostMonitorSettings` 通过；`TestUnifiedSchemaMigrationsKeepDatabaseOrder` 通过。
- [ ] 未开始生产发布。发布需要单独确认。历史曲线要等新进程采样后才有数据。

## 验证

```text
GOWORK=off go test ./node/api -count=1 -timeout 180s -run 'TestHostMonitor|TestDashboard'
GOWORK=off go test ./cmd/workmesh-server -count=1 -timeout 180s -run 'TestUnifiedSchemaMigrations'
```

## 下一步

跑通上述测试后，把结果写回本文件和 `docs/development/STATUS.md`。
