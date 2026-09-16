<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Core 与网站生命周期质量整改（2026-09-07）

状态：`[x]` 本批次职责拆分完成，等待唯一集成测试负责人覆盖当前冻结快照。

## 变更

- `control/service/core_persistence.go` 集中承载 CoreService 的 SQLite/旧文件持久化、Passkey 表恢复和用户快照转换；`core.go` 保留认证、会话、分组和设置业务。
- `node/service/website_lifecycle_operations.go` 承载网站删除和启动/停止/重启操作；`website_lifecycle.go` 保留创建和更新流程。
- 网站生命周期路由、SQLite 写入、WAF 清理和目录回滚行为未改变；未修改前端或 `apps/1Panel`。

## 文件规模

- `control/service/core.go`：397 行。
- `control/service/core_persistence.go`：238 行。
- `node/service/website_lifecycle.go`：496 行。
- `node/service/website_lifecycle_operations.go`：74 行。

## 验证

```text
gofmt -w control/service/core.go control/service/core_persistence.go node/service/website_lifecycle.go node/service/website_lifecycle_operations.go
GOWORK=off go test ./node/service -run 'TestWebsiteService|TestManagedWebsite|TestWebsiteRuntime|TestWAF' -count=1
git diff --check
```

以上定向测试和差异检查已通过。真实 OpenResty/Docker 网站生命周期仍需集成测试负责人按冻结快照执行，不能以模拟资源替代。
