<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 日志审计策略补充（2026-09-07）

本批次只修改 `apps/workmesh-server`。七类日志统一遵循“真实数据源、查询前不改原文、响应和导出时脱敏”的边界；不写入 JSON 模拟数据，不触碰生产 SQLite。

## 七类日志出口

| 类型 | v2 入口 | 数据源 | 敏感字段处理 | 清理策略 |
| --- | --- | --- | --- | --- |
| 操作日志 | `/api/v2/core/logs/operation`、`/api/v2/logs/detail` | SQLite `operation_logs` | message、详情、UA、路径 query、meta 递归脱敏 | 180 天或 100,000 行 |
| 访问日志 | `/api/v2/websites/monitor/logs/*` | OpenResty/Nginx access.log | URI query、Referer、UA 和结构化字段脱敏 | 30 天或 2 GiB，由 logrotate/维护任务执行 |
| 系统日志 | `/api/v2/logs/system/read` | journalctl 或受限系统日志文件 | message、raw 及嵌套字段脱敏，限制 8 MiB/100,000 行 | 30 天或 2 GiB，由 journald/logrotate 执行 |
| 任务日志 | `/api/v2/logs/tasks/search`、`/read` | SQLite 任务表及受限任务文件 | 任务名称、步骤、错误、路径和每行输出脱敏 | 90 天；任务表 100,000 行，明细 500,000 行 |
| 主机日志 | `/api/v2/hosts/ssh/log*` | 允许目录内 auth.log/secure | authMode、message 及 CSV 导出脱敏，地址/用户保留定位 | 30 天或 2 GiB，由 logrotate/维护任务执行 |
| 登录日志 | `/api/v2/core/logs/login`、`/api/v2/logs/detail` | SQLite `login_logs` | agent、message 和详情递归脱敏；IP、用户保留审计定位 | 180 天或 50,000 行 |
| 网站日志 | `/api/v2/websites/log/search` | 站点 access.log/error.log | content 按行脱敏，路径仅返回受控站点路径 | 30 天或 2 GiB，由站点维护任务执行 |

## 实现约束

- `redactLogText` 覆盖 Bearer/Basic、token、cookie、password、secret、API key、私钥和会话字段；结构化 map、`[]any`、`[]string` 均递归处理。
- SQLite 清理在单事务内按年龄后按 `created_at DESC,id DESC` 保留最新记录；缺少可选领域表时跳过该表并继续提交，避免部分迁移导致全局清理失败。
- 清理函数目前是受控调用入口，必须由备份后的低峰维护任务触发；接口不得允许任意表名或任意文件路径。
- 原始 SQLite 和日志文件不原地改写，导出文件使用受限权限；脱敏值固定为 `[REDACTED]`，便于黑盒检测。

## 定向证据

```text
GOWORK=off go test ./node/api -run 'TestRedact|TestPruneRetained' -count=1
PASS
```

该结果仅证明隔离 SQLite、系统/SSH 文件和网站日志响应的脱敏及清理边界；生产环境仍需在维护窗口执行真实路由黑盒验收，并记录清理前后计数、备份路径和制品哈希。
