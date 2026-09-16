<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# CoreService 结构质量整改（2026-09-07）

状态：`[x]` 本批次完成

## 本批次变更

- `control/service/core.go` 从 608 行降至 303 行，仅保留服务模型、认证会话、用户和 API 配置业务。
- 新增 `control/service/core_persistence.go`（238 行），承载 SQLite 表初始化、用户/Passkey 快照读写、旧 JSON 一次性导入和兼容文件原子保存。
- 新增 `control/service/core_passkeys.go`（85 行），承载 Passkey 注册挑战、重复凭据校验和删除回滚。
- 新增 `control/service/core_groups.go`（104 行），承载分组、核心设置及 SQLite 恢复逻辑。
- 保持 SQLite 表名、字段、一次性迁移条件、认证错误文本、会话/API Key 语义不变；未修改 HTTP 路由、前端或 `/www/apps/1Panel`。

## 验证

```text
gofmt -w control/service/core.go control/service/core_persistence.go control/service/core_passkeys.go control/service/core_groups.go
go test ./control/service -count=1
git diff --check -- control/service/core.go control/service/core_persistence.go control/service/core_passkeys.go control/service/core_groups.go
```

结果：`control/service` 定向测试通过，差异检查通过。全量 Go/Race/Vet 门禁交由唯一集成测试负责人在源码冻结后执行。

## 备注

- 工作区中的 `control/service/core_persistence_test.go` 存在其他并行变更，本批次未覆盖或回滚。
- 真实管理员登录、外部 Gateway 和生产数据库验收仍需集成阶段真实资源，未使用模拟业务数据替代。
