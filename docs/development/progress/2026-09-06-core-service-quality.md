<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 核心服务质量整改（2026-09-06）

状态：`[x] 已完成`

本批为 `control/service/core.go` 的 SQLite 注入、用户与 Passkey 持久化、兼容文件加载、设置恢复及凭据辅助函数补充准确中文注释。公开接口、会话、认证、SQLite 数据格式和无数据库兼容行为均未改变。

该文件当前约 680 行，超过 500 行建议线。认证、Passkey、分组和设置适合后续按职责拆为独立源码文件，但本批限制只修改该源码文件；同文件内机械搬移不会降低单文件风险，因此不做结构迁移。后续拆分需单独覆盖锁边界、SQLite 优先级、兼容文件一次性导入和持久化失败回滚。

验证：

```text
GOWORK=off go test ./control/service -run 'TestCore|Test.*Passkey|Test.*API' -count=1
git diff --check -- control/service/core.go docs/development/progress/2026-09-06-core-service-quality.md
```

未执行全量测试、前端编译或外部环境验收。
