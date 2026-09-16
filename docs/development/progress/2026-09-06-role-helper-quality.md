<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 角色控制器辅助逻辑质量整改（2026-09-06）

状态：`[x]` 本批次完成，待完整集成门禁覆盖。

## 本批次变更

- 将角色节点数字 ID、收藏布尔值转换和本机节点条目构造移至 `control/api/role_helpers.go`。
- `role.go` 从 521 行降至 488 行；角色选举、epoch、fencing、节点 CRUD、鉴权和 SQLite 写入行为保持不变。

## 验证

```text
gofmt -w control/api/role.go control/api/role_helpers.go
GOWORK=off go test ./control/api -run 'Test.*(Role|Node|Fencing)' -count=1
git diff --check
```

结果：角色控制器定向测试和差异检查通过。全量门禁由唯一集成测试负责人执行。
