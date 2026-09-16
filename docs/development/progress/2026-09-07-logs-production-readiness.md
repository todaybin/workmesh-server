<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 日志审计生产就绪评估（2026-09-07）

状态：`partial`。本轮完成日志敏感字段脱敏实现、SQLite 保留清理函数及定向测试；未修改
`/www/apps/1Panel`、生产 SQLite、Docker 或 systemd，也未执行生产清理或真实 HTTP 黑盒测试。

本报告服务于“先完成 3~4 个正式使用核心点”的总计划：日志审计属于第四个运维交付点；
系统日志契约和主机 SSH 日志在实现前保持 `blocked`，不能以模拟数据替代。

## 1. 上线优先级

日志不是当前上线的唯一阻断项，但以下顺序应纳入总体验收：

| 优先级 | 项目 | 当前状态 | 正式使用判断 | 解除条件 |
| --- | --- | --- | --- | --- |
| P0 | 系统日志契约 | `blocked` | 前端无法按 1Panel v2 契约读取 | `/logs/system/read` 返回 `source/items/hasMore/nextCursor`，并完成真实文件读取测试 |
| P0 | 主机 SSH 日志 | `blocked` | 三个接口仍为 `501 NOT_IMPLEMENTED` | `/hosts/ssh/log`、`clean`、`export` 使用真实 SSH 日志源并通过权限、清理、下载测试 |
| P0 | 统一 HTTP 黑盒验收 | `not-run` | 无法证明前端可正式使用 | 在可访问服务端口的环境执行登录、鉴权、日志全链路测试并留存状态码/响应摘要 |
| P1 | 操作/登录日志分页与筛选 | `implemented-test-only` | SQL `WHERE/LIMIT/OFFSET` 已接入，仍需生产黑盒确认 | 在可访问生产环境确认大库响应时间、索引命中和契约字段 |
| P1 | 任务日志查询性能 | `implemented-test-only` | 任务与明细日志已按页读取，仍需生产黑盒确认 | 确认真实运行时长任务、边界页和任务状态兼容 |
| P1 | 脱敏策略 | `implemented-test-only` | 响应出口已统一替换敏感值，仍需生产黑盒确认 | 生产环境逐路由确认响应和导出文件均不含秘密；补充可信代理/外部日志验证 |
| P1 | 保留与清理策略 | `implemented-scheduled` | SQLite 清理函数已接入节点后台维护周期，并写入清理结果审计；文件型日志仍由 journal/logrotate 管理 | 在隔离库验证周期触发、备份策略和重复执行幂等 |
| P2 | 状态/方法值规范化 | `partial` | 历史数据展示可能不一致 | 统一 `success/failed`、HTTP 方法枚举；兼容读取旧值并提供一次性修复迁移 |
| P2 | 登录地址补全 | `partial` | 审计信息不完整 | 写入真实远端地址（代理头按可信代理配置解析），并覆盖 IPv4/IPv6 测试 |
| P2 | 告警日志持久化 | `partial` | 当前主要是内存状态 | 建立 SQLite 告警事件表、查询与清理接口，明确是否纳入本次上线范围 |

## 2. 已核实的真实数据与接口

生产 SQLite `/opt/workmesh-server/data/workmesh.db` 只读检查结果：

- `operation_logs`：2392 条；`login_logs`：185 条。
- `runtime_tasks`：31 条；`runtime_task_logs`：18145 条；`app_install_tasks`：66 条。
- `PRAGMA quick_check`、`integrity_check` 均为 `ok`；`foreign_key_check` 无违规。

已接入真实 SQLite 的能力包括面板操作日志、登录日志、应用/运行时任务及任务明细。
网站访问/错误日志和系统日志读取真实 OpenResty/Nginx、systemd/journal 文件；SQLite
仅保存相关元数据。若要求“所有运行态日志统一 SQLite”，需另立导入策略，不能把文件
回退读取误称为 SQLite 闭环。

参考的 1Panel v2 路由契约：

- `/core/logs/login`、`/core/logs/operation`、`/core/logs/clean`
- `/logs/system/files`、`/logs/system/status`、`/logs/system/read`、`/logs/system/services`
- `/logs/tasks/search`、`/logs/tasks/read`、`/logs/tasks/executing/count`
- `/websites/log/search`、`/websites/log/operate`
- `/hosts/ssh/log`、`/hosts/ssh/log/clean`、`/hosts/ssh/log/export`
- `/cronjobs/records/log`、`/files/convert/log`、`/toolbox/ftp/log/search`

## 3. 具体缺口

1. `/logs/system/read` 当前要求 `path` 并返回 `path/content`，前端契约需要
   `source/items/hasMore/nextCursor`；这是系统日志上线前的硬阻断。
2. `/hosts/ssh/log`、`/hosts/ssh/log/clean`、`/hosts/ssh/log/export` 仍由
   `fallbackRouteHandler` 返回 `501`，不能宣称主机日志可用。
3. 操作、登录、任务查询的 SQL 分页已实现并有临时库测试；生产大库的查询计划、耗时和
   长时间任务边界仍未完成黑盒验收。
4. SQLite 保留清理函数已绑定节点后台维护周期并写入 `/internal/log-retention` 操作审计；
   备份确认仍由部署维护流程负责，文件型日志仍依赖 journal/logrotate 或人工维护窗口。
5. 历史操作日志存在 `Success/success`、`Failed/failed` 以及异常方法值（如 `pri`、
   `connect`）；登录日志写入时 `address` 未填充。
6. 脱敏规则已覆盖响应消息、详情、UA、系统/SSH/网站原始日志和任务输出；仍需在生产
   黑盒与导出流程中确认没有遗漏字段。

## 3.1 LOG-03 脱敏字段清单与实现

`node/api/logs_redaction.go` 提供统一响应脱敏函数，保留字段名但将值替换为
`[REDACTED]`。规则覆盖：

- `Authorization: Bearer ...`、`Basic ...`；
- `token`、`access_token`、`refresh_token`、`csrf_token`、`session_id`；
- `cookie`、`set-cookie`；
- `password`、`passwd`、`passphrase`；
- `secret`、`secret_key`、`secret_token`、`credential`；
- `api_key`、`apikey`、`private_key`、`client_secret`。

已接入的输出范围：操作日志、登录日志、任务列表、任务文件/SQLite 明细、系统日志
`items/content/raw`、主机 SSH 日志及 CSV 导出、网站访问日志的 map 字段。筛选仍在
原始数据上执行，响应和导出阶段才脱敏，以免破坏审计检索；共享内存和 SQLite 原文不被
原地改写。

## 3.2 LOG-03 默认保留策略

| 数据源 | 默认保留期 | 最大行数 | 执行方式 |
| --- | ---: | ---: | --- |
| `operation_logs` | 180 天 | 100,000 | `pruneRetainedSQLiteLogs` 事务清理 |
| `login_logs` | 180 天 | 50,000 | 同上 |
| `app_install_tasks` | 90 天 | 100,000 | 同上 |
| `runtime_tasks` | 90 天 | 100,000 | 同上 |
| `runtime_task_logs` | 90 天 | 500,000 | 同上 |
| 系统日志、网站日志、SSH 日志 | 30 天 | 2 GiB | 由 journal/logrotate 或维护任务执行 |

SQLite 清理先按 `created_at` 删除过期行，再保留最新 ID 范围内的最大行数，整个操作在
单个事务中完成。函数当前只提供受控调用入口，尚未绑定自动计划任务，避免未经备份或
人工确认直接清理生产记录。

## 4. 验收记录模板

唯一集成测试负责人应为每个路由记录：HTTP 方法、请求摘要（不含秘密）、状态码、响应
字段摘要、SQLite/文件数据源、域名访问结果、清理前后计数、失败原因及未覆盖项。建议
将记录追加到 `docs/development/progress/integration-test-latest.md`，并保留测试时间、
制品 SHA-256、数据库备份路径和执行环境。

## 5. 上线判定

- P0 任一项未解除：日志审计域不能作为正式可用能力；总体验收应阻止发布。
- P1 项未完成：可在受控小规模环境试运行，但必须设置日志容量监控和人工清理窗口，
  不得宣称长期生产就绪。
- P2 项可在首个稳定版本后通过兼容迁移补齐，但必须在发布说明中明确限制。

## 6. LOG-03 定向验证

```text
go test ./node/api -run 'TestRedact|TestLogResponses|TestSystemAndSSHLogResponses|TestPruneRetainedSQLiteLogs' -count=1
ok
```

测试使用临时真实 SQLite 和临时日志文件，验证脱敏值不回显、CSV 导出不泄露、过期行
被删除且近期行保留；未执行生产数据库清理。
