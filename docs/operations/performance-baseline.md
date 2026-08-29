<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 性能基线记录

性能结论必须基于同一主机、同一功能开关和同一负载下的实测。至少记录：空闲 RSS、Goroutine、CPU、SQLite 连接数、HTTP 长连接数、启动耗时、节点切换延迟、同步吞吐和重试恢复时间。

微基准运行：

```powershell
go test ./test/perf -run '^$' -bench . -benchmem -count=3
```

双进程 Core+Agent 与单进程 WorkMesh Server 的实测结果追加到本文件，并注明系统版本、Go 版本、提交号、配置摘要和采样时间。不能将预计节省比例当作验收结论。
