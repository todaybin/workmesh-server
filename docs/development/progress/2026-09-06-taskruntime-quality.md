<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 任务执行沙箱代码质量整改（2026-09-06）

状态：`[x]` 本批次完成

## 本批次变更

- 为 `node/service/taskruntime/taskruntime.go` 的策略归一化、任务规格校验、状态转换、CLI 执行、输出限制和敏感信息脱敏方法补充中文方法级注释。
- 注释明确任务资源上限、参数白名单、上下文取消、SHA-256 校验和错误脱敏不变量；未改变任务状态机、CLI 参数或持久化格式。
- 将 CLI 适配器完整迁移到 `node/service/taskruntime/backend_cli.go`；核心领域文件由 575 行降至 373 行，CLI 基础设施文件为 242 行，公开符号和调用关系保持不变。

## 验证

```text
gofmt -w node/service/taskruntime/taskruntime.go node/service/taskruntime/backend_cli.go
GOWORK=off go test ./node/service/taskruntime -count=1
git diff --check -- node/service/taskruntime/taskruntime.go node/service/taskruntime/backend_cli.go
```

定向测试和差异检查通过。完整 Go、Race、Vet、Node 合约和路由门禁由唯一集成测试负责人统一执行。

## 后续

- `taskruntime.go` 已低于 500 行；后续按领域需要再拆策略校验，但不为目录形式重复抽象。
- 外部签名任务 CLI 和真实沙盒生命周期仍需部署资源，当前保持 `blocked/not-run`。
