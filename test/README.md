<!-- SPDX-License-Identifier: LicenseRef-WorkMesh-Pending -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh Server 测试入口

新仓库是独立 Go module；在主仓库工作树中运行 Go 命令时必须关闭上层 `go.work`，并使用项目级缓存：

```powershell
$env:GOWORK='off'
$env:GOCACHE=(Join-Path (Get-Location) '.cache/go-build')
$env:GOMODCACHE=(Join-Path (Get-Location) '.cache/go-mod')
go test ./...
```

测试分层：

- `contract`：从旧 Core/Agent 源码生成 825 条路由基线，并严格校验新服务无缺失或未审查新增路由。
- `integration`：验证真实角色管理、epoch fencing、上下文取消和 Gateway 注册契约。
- `perf`：记录统一运行时角色读取/切换的分配量；发布验收另需实机资源基线。

涉及 Gateway、Docker、远程 SSH 或真实节点的测试必须通过显式环境变量启用，默认不连接外部系统、不修改生产主机。
