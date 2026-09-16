<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Gateway 结构质量整改（2026-09-06）

状态：`[x]` 本批次完成

## 本批次变更

- `control/api/gateway.go` 保留 Gateway HTTP 路由、状态机和请求处理逻辑，行数由 728 行降至 446 行。
- 新增 `control/api/gateway_persistence.go`，集中承载 Gateway 地址校验、SQLite/兼容快照加载、原子保存和解绑清理；未改变 SQLite 表结构、兼容文件字段或授权数据。
- 新增 `control/api/gateway_runtime.go`，集中承载节点自动注册、30 秒心跳和环境变量登录；保留原有锁顺序、绑定复用和断线降级语义。
- 未修改 `/www/apps/1Panel`、前端、OpenResty/WAF 配置、路由路径、HTTP 方法、请求字段或响应 envelope。

## 验证

```text
gofmt -w control/api/gateway.go control/api/gateway_persistence.go control/api/gateway_runtime.go
go test ./control/api -run 'Gateway|gateway' -count=1
git diff --check -- control/api/gateway.go control/api/gateway_persistence.go control/api/gateway_runtime.go
```

结果：定向 Gateway 测试通过，差异检查通过。全量门禁须由唯一集成测试负责人在源码冻结后执行。

## 后续

- 真实 Gateway 登录、次节点心跳、任务透传和角色切换仍需要真实凭据与环境，未使用模拟数据替代。
