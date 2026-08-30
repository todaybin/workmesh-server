<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 路由契约迭代策略

871 条路由契约作为 WorkMesh Server 的兼容基线保留。冻结的是公开契约，不是业务实现：

- 方法、路径参数、鉴权方式、成功/错误 envelope 和分页字段保持向后兼容。
- 每条路由必须继续替换真实业务实现，禁止用 `MIGRATION_PENDING`、固定空数组或兼容处理器冒充完成。
- 新能力优先在同一路径上向前兼容扩展；确需改变语义时，新增 `/api/v3` 或明确版本参数，并保留旧路径一个完整迁移周期。
- 下线流程必须先在清单中标记 `deprecated`（仅文档状态，不改变五种实现状态），发布至少一个版本并记录调用方迁移情况，再提交删除审批。
- 路由扫描、实现扫描和隐藏功能扫描是每次发布的门禁；扫描结果不能覆盖实际集成测试结论。

## 状态与责任

`implemented` 只有在真实处理、错误处理、持久化依据、测试和本地集成验证全部具备时才能使用。云端凭据、真实 Docker/OpenResty、PTY、ACME 挑战等外部条件不足时，保持 `partial` 或 `pending`，并在清单中写明阻塞原因，不伪造成功。

## 发布检查

```powershell
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test -count=1 ./..."
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go vet ./..."
node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/route-scan.mjs check --legacy apps/workmesh-node --project apps/workmesh-server --manifest apps/workmesh-server/test/contract/routes.json
node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/implementation-scan.mjs --legacy apps/workmesh-node --project apps/workmesh-server --manifest apps/workmesh-server/test/contract/routes.json --out .tmp/implementation-status.json --markdown apps/workmesh-server/docs/migration/function-checklist-generated.md
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "Set-Location apps/workmesh-server/web; npm.cmd run type-check; npm.cmd run build:pro"
```

