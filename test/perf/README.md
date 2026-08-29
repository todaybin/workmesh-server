<!-- SPDX-License-Identifier: LicenseRef-WorkMesh-Pending -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 性能基准

基准测试记录统一运行时角色读取和切换的分配量，作为单进程低开销优化的微基线。它不模拟吞吐，也不替代双进程/单进程实机对比。

运行：

```powershell
go test ./test/perf -run '^$' -bench . -benchmem -count=3
```

发布验收还必须记录 RSS、Goroutine、CPU、SQLite 连接、HTTP 长连接、启动时间和节点同步延迟，并保存到 `docs/operations/performance-baseline.md`。
