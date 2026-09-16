<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 当前源码冻结交接（2026-09-06）

状态：`[>]` 等待唯一集成测试负责人执行完整门禁。

## 冻结范围

本次冻结包含主机运维、计划任务、角色控制器、Gateway 和节点透传质量批次；没有前端、`apps/1Panel` 或生产配置写入。

| 项目 | 当前值 |
| --- | --- |
| tracked diff 文件集合 SHA-256 | `8ffca803e22bad8b4bb387b9845ad6902ba83785f7018c42a6df705e70ab7759` |
| untracked 文件集合 SHA-256 | `b1970cdbff1c0bb191dd0bc5337624fe07f35641d10d341e4ca3bcd17207646d` |
| Go 源码集合 SHA-256 | `ccda5d08828f54a8cab109b09fe5575e5ff455120ef2c45fa77662bd40a367a0` |
| 质量报告 | 340 个文件、2761 个函数、903 项违规 |

## 唯一门禁顺序

```text
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
git diff --check
node --test test/contract/*.test.mjs
node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest docs/inventory/route-inventory-1panel.json
SQLite quick_check/integrity_check、systemd、/health、/ready、OpenResty nginx -t
```

测试负责人开始前和结束后必须重新计算上述集合哈希；哈希变化则本轮结果不得作为当前源码快照验收。
