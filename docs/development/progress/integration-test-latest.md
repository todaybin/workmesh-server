<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

## 2026-09-07 P2/P3 专项集成门禁

本轮三个并行专项已交付，由主控统一执行一次门禁：日志查询已改为 SQLite 条件分页；升级流程完成隔离制品验签、双启动迁移幂等和审计哈希演练；终端 WebSocket 完成本地 TCP 握手、鉴权、命令输出、关闭和资源释放测试。未修改 `/www/apps/1Panel`，未执行生产写操作或前端编译。

| 检查 | 结果 |
| --- | --- |
| `GOWORK=off go test ./...` | 通过 |
| `GOWORK=off go test -race ./...` | 通过 |
| `GOWORK=off go vet ./...` | 通过 |
| `node --test test/contract/*.test.mjs` | 7/7 通过 |
| `node test/contract/route-scan.mjs check test/contract/routes.json` | 759 条路由通过 |
| `git diff --check` | 通过 |
| 日志 SQL 分页定向测试 | 通过 |
| 隔离 SQLite 迁移/验签演练 | 通过 |
| 终端 WebSocket TCP 定向测试（5 次） | 通过 |

本轮结果仅表示源码、隔离迁移和本地 WS 契约达到集成门禁；真实管理员登录后的 385 条 HTTP、4 条 WS、六类运行时、网站全类型、ACME、主次节点和生产替换仍需外部凭据及维护窗口，继续保持 `not-run/blocked`。

### 审计策略专项补充

`AUDIT-03` 已完成操作/访问/系统/任务/主机/登录/网站七类日志的真实数据源、响应/导出脱敏、保留上限和 SQLite 清理边界；隔离测试 `TestRedact*`、`TestPruneRetained*` 通过。清理仅允许维护任务受控调用，不接受任意表名或路径。生产清理和黑盒路由仍待维护窗口。

### 最终门禁复核（源码冻结后）

审计策略变更纳入后再次执行唯一集成门禁：`GOWORK=off go test ./...`、`go test -race ./...`、`go vet ./...`、Node 契约 7/7、759 条路由扫描和 `git diff --check` 全部通过。前端未变更，因此未重新编译前端。

终端错误帧随后独立到 `terminal_errors.go`（仅结构调整），重新执行同一套 Go/race/vet、Node 契约和路由门禁，结果仍全部通过；当前源码集合已冻结。

系统日志分页/文件读取随后独立到 `functional_system_logs.go`，`functional_logs.go` 降至 465 行；日志定向测试通过，质量扫描确认超长文件为 0，剩余违规为超长函数和缺失中文文档注释。该结构变更后的 Go/race/vet、Node 契约、759 条路由和 diff 门禁再次全部通过。

应用目录版本比较已将内嵌解析闭包提取为 `parseAppVersion`，并补齐目录读取/远程刷新函数注释；应用定向测试通过，质量违规进一步降至 860 后又随日志结构快照更新为 860（超长文件 0）。该批次后的完整门禁再次全部通过。

容器请求入口已将无需 Docker 命令的查询路由提取到 `handleContainerQuery`，并补齐容器状态解析辅助函数注释；容器定向测试通过，质量扫描降至 854。该批次后的 Go 全量/race/vet、Node 契约、759 条路由和 diff 门禁全部通过。

# 集成测试负责人最新门禁记录

## 2026-09-07 SYSLOG-01 定向验证

`node/api/functional_logs.go` 的系统日志读取已完成兼容修复：完全空请求继续返回 HTTP 400，显式 v2 参数支持真实 journal/file 数据源、分页游标和过滤。定向命令 `GOWORK=off go test ./node/api -run 'TestFunctionalLogValidation|TestLogsRuntime'` 通过（`ok`, 0.029s）。本次尚未执行全量门禁；全量测试仍由主控在源码冻结后统一执行。

## 2026-09-07 18:01 登录后只读黑盒抽样

本次仅使用本机管理员登录会话读取真实数据，未执行创建、删除、启停、重启、证书签发或 Docker 操作；Session/CSRF 值未写入记录。登录入口 `POST /api/v2/core/auth/login` 返回 HTTP 200、`code=200`、`mfaStatus=disabled`，随后 `GET /api/v2/core/auth/current` 返回 HTTP 200，角色为 `ADMIN`。

| 请求 | HTTP/业务结果 | 真实数据摘要 | 结论 |
| --- | --- | --- | --- |
| `POST /api/v2/websites/search`（`page=1,pageSize=20`，带 CSRF） | 200 / `code=200` | `total=1`；`znmp.sopvip.com`，`type=static`，`status=running`，HTTPS，`websiteSSLId=1` | 只证明查询契约和存量读取 |
| `POST /api/v2/runtimes/search`（`page=1,pageSize=20`，带 CSRF） | 200 / `code=200` | `total=6`；Go、Node、Python、Java、.NET、PHP 均有 SQLite 记录，当前状态均为 `Stopped` | 只证明查询契约和存量读取；未证明容器可用 |
| `POST /api/v2/logs/search`（`page=1,pageSize=20`，带 CSRF） | 200 / `code=200` | `total=40741`；返回真实访问/操作日志，含时间、路径、状态、用户和延迟 | 只证明日志读取；七类日志闭环仍待专项验收 |
| 同三条 POST（无 `X-CSRF-Token`） | 403 / `CSRF_INVALID` | 统一错误 envelope，未产生业务副作用 | CSRF 防护符合预期 |

本抽样确认 API 监听 `*:9999`，`/health` 和 `/ready` 均 HTTP 200；没有把响应中的真实存量记录推断为完整功能通过。正式放行仍需按 [`2026-09-07-formal-execution-plan.md`](2026-09-07-formal-execution-plan.md) 执行 P0/P1/P2/P3 的真实写操作和回滚证据。

## 2026-09-07 15:29 冻结工作区完整集成门禁通过（沙盒任务职责拆分后）

状态：[x] 当前冻结快照自动化门禁通过；真实外部业务验收仍为 `blocked/not-run`。测试前后 Go 源码集合 SHA-256 均为 `a29a613a375b1e3fa7a4952cf9765e86a70e3e56e4d2736cb71de7bc2628e317`，期间未有源码写入；未修改 `apps/1Panel`、前端、OpenResty 配置或生产数据库。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | 所有 Go 包退出码 0，`node/api` 69.111s |
| `GOWORK=off go test -race ./...` | pass | 所有 Go 包退出码 0，`node/api` 121.108s，未报告数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7，0 失败 |
| 1Panel 路由扫描 | pass | 759 条基线路由通过，192 条扩展路由兼容允许 |
| `git diff --check` | pass | 退出码 0 |
| 生产只读检查 | pass | systemd active；`/health=200`、`/ready=200`；OpenResty `nginx -t` 成功 |

本轮将 `taskHandler` 拆为沙盒任务创建和生命周期操作两个职责函数；质量扫描为 362 个 Go 文件、2897 个函数、844 项违规（中文注释 758、超长函数 86），代码规范门禁仍未通过。管理员登录后的 385 条 HTTP、4 条 WS、六类运行时、网站真实访问、主次节点、HTTP-01 签发/续期和升级迁移仍缺少真实凭据或隔离资源，继续标记 `blocked/not-run`，没有用模拟数据替代。

## 2026-09-07 15:15 冻结工作区完整集成门禁通过（最终 AI/应用拆分快照）

状态：[x] 当前冻结快照自动化门禁通过；真实外部业务验收仍为 `blocked/not-run`。测试前后 Go 源码集合 SHA-256 均为 `d366e237d2e48073d94b8898ae4b1464b4c8d07c070c289226b8c9dcea53dd14`，期间未有源码写入；未修改 `apps/1Panel`、前端、OpenResty 配置或生产数据库。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | 所有 Go 包退出码 0，`node/api` 69.284s |
| `GOWORK=off go test -race ./...` | pass | 所有 Go 包退出码 0，`node/api` 120.542s，未报告数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7，0 失败 |
| 1Panel 路由扫描 | pass | 759 条基线路由通过，192 条扩展路由兼容允许 |
| `git diff --check` | pass | 退出码 0 |
| 生产只读检查 | pass | systemd active；`/health=200`、`/ready=200`；OpenResty `nginx -t` 成功 |

本轮完成 `node/api/ai_post_routes.go` 分派拆分，并补充 AI/MCP/沙盒/任务入口中文职责注释；定向 AI/App/Task 测试通过。质量扫描为 362 个 Go 文件、2895 个函数、845 项违规（中文注释 758、超长函数 87），代码规范门禁仍未通过。管理员登录后的 385 条 HTTP、4 条 WS、六类运行时、网站真实访问、主次节点、HTTP-01 签发/续期和升级迁移仍缺少真实凭据或隔离资源，继续标记 `blocked/not-run`，没有用模拟数据替代。

## 2026-09-07 15:02 冻结工作区完整集成门禁通过（AI/应用路由拆分后）

状态：[x] 当前冻结快照自动化门禁通过；真实外部业务验收仍为 `blocked/not-run`。测试前后 Go 源码集合 SHA-256 均为 `6a478e14834f8bedbaaad5baf379915efcfe58d62fa45662b5e07de1c3b90736`，期间未有源码写入；未修改 `apps/1Panel`、前端、OpenResty 配置或生产数据库。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | 所有 Go 包退出码 0，`node/api` 67.154s |
| `GOWORK=off go test -race ./...` | pass | 所有 Go 包退出码 0，`node/api` 120.811s，未报告数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7，0 失败 |
| 1Panel 路由扫描 | pass | 759 条基线路由通过，192 条扩展路由兼容允许 |
| `git diff --check` | pass | 退出码 0 |
| 生产只读检查 | pass | systemd active；`/health`、`/ready` HTTP 200；OpenResty `nginx -t` 成功 |

本轮新增应用生命周期分派文件 `node/api/apps_lifecycle.go`、`node/api/apps_lifecycle_post.go` 和 AI 分派文件 `node/api/ai_post_routes.go`；定向 App/AI 测试通过。质量扫描刷新为 362 个 Go 文件、2895 个函数、853 项违规（中文注释 766、超长函数 87），代码规范门禁仍未通过。管理员登录后的 385 条 HTTP、4 条 WS、六类运行时、网站真实访问、主次节点、HTTP-01 签发/续期和升级迁移仍缺少真实凭据或隔离资源，继续标记 `blocked/not-run`，没有用模拟数据替代。

## 2026-09-07 01:40 冻结工作区完整集成门禁通过（网站测试拆分后）

状态：[x] 当前冻结快照自动化门禁通过；真实外部业务验收仍为 `blocked/not-run`。测试前后 Go 源码集合 SHA-256 均为 `5956a7decb8294046773cad5af97839a342bc82bba5d7f4c2f95ecc01de01284`，期间未有源码写入；未修改 `apps/1Panel`、前端、OpenResty 配置或生产数据库。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | 所有 Go 包退出码 0，`node/api` 67.085s |
| `GOWORK=off go test -race ./...` | pass | 所有 Go 包退出码 0，`node/api` 120.382s，未报告数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7，0 失败 |
| 1Panel 路由扫描 | pass | 759 条基线路由通过，192 条扩展路由兼容允许 |
| `git diff --check` | pass | 退出码 0 |
| 生产只读检查 | pass | systemd active；`/health`、`/ready` HTTP 200；OpenResty `nginx -t` 成功 |

当前质量扫描为 352 个 Go 文件、2768 个函数、886 项违规（中文注释 785、超长函数 101）；代码规范门禁仍未清零。管理员登录后的 385 条 HTTP、4 条 WS、六类运行时、网站真实访问、主次节点、HTTP-01 签发/续期和升级迁移仍缺少真实凭据或隔离资源，继续标记 `blocked/not-run`，没有用模拟数据替代。

## 2026-09-07 00:31 冻结工作区完整集成门禁通过（结构拆分后）

状态：[x] 当前源码冻结快照的自动化门禁通过；真实外部业务验收仍为 `blocked/not-run`。本轮在代理停止写入后执行，测试输入中的 Go 源码集合 SHA-256 为 `20e1981be66e3091999636b2536f845237473db2e966af592f2220fb938f5a18`，测试前后完全一致。未修改 `apps/1Panel`、前端、OpenResty 配置或生产数据库。

### 自动化门禁

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | 所有 Go 包退出码 0；`node/api`、`node/service` 及集成包通过 |
| `GOWORK=off go test -race ./...` | pass | 所有 Go 包退出码 0；未报告数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7，0 失败 |
| `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest docs/inventory/route-inventory-1panel.json` | pass | 759 条基线路由通过，192 条扩展路由兼容允许；参考项目只读 |
| `git diff --check` | pass | 退出码 0 |

### 生产只读检查

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| systemd | pass | `workmesh-server` 为 `active` |
| `/health` | pass | HTTP 200，`status=ok` |
| `/ready` | pass | HTTP 200，`status=ready` |
| OpenResty | pass | `docker exec workmesh-openresty-waf nginx -t` 成功 |

### 当前质量与未覆盖项

质量扫描结果为 344 个 Go 文件、2764 个函数、900 项违规（中文注释 796、超长文件 4、超长函数 100），因此代码规范门禁仍未清零。登录后 385 条 HTTP、4 条 WS、六类运行时真实生命周期、六类网站设置访问、主/次 Gateway、Let's Encrypt HTTP-01 正式签发/续期和软件更新迁移仍缺少真实凭据或隔离资源，保持 `blocked/not-run`，没有使用模拟 JSON 或固定响应替代。

后续仍由唯一集成测试负责人执行真实资源验收；取得授权后按 HTTP、WS、运行时、网站、主次节点、ACME、升级回滚矩阵逐项记录请求参数、响应 envelope、状态码、资源副作用和回滚结果。

## 2026-09-06 21:33 冻结工作区完整集成门禁通过（快照前后一致）

状态：[x] 当前冻结工作区的唯一完整集成门禁通过；真实外部业务验收仍为 `blocked/not-run`。本轮于 `2026-09-06 21:21:28 CST` 记录测试输入快照，依次执行 Go 全量、race、vet、Node 合约、759 条路由、diff check 和生产只读检查，于 `21:33:01 CST` 结束并复核哈希。没有修改业务代码、前端或 `apps/1Panel`，没有编译前端、部署、重启服务或写入生产库。

### 测试输入快照

| 项目 | 开始及结束结果 |
| --- | --- |
| Go/模块/合约输入集合 SHA-256 | `d188169a7d7a7a70927b03445267576c729dcada6d76d7e986e8e0459e7117b4`，前后一致 |
| Git HEAD | `49319f469c0ccf66ce127419b857af7edba7c59b`，前后一致 |
| 生成范围 | `*.go`、`go.mod`、`go.sum`、`*.mjs`、`*.json`；排除 `web/`、依赖、文档、构建产物、运行数据和合约结果 |

集合摘要按路径排序后对逐文件 `sha256sum` 再计算 SHA-256；本报告最后写入不计入测试输入集合。此前关注的 taskruntime、role 和 containers 等源码均在同一集合内，测试前后未变化。

### 自动化门禁

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | 退出码 0；`node/api` 67.232s、`node/service` 11.020s；其余包通过、缓存通过或无测试 |
| `GOWORK=off go test -race ./...` | pass | 退出码 0；`node/api` 117.725s、`node/service` 19.689s；未报告数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7，0 失败，总耗时 3.403s |
| `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest test/contract/routes.json` | pass | 759 条基线路由通过，192 条扩展路由兼容允许；只读参考项目 |
| `git diff --check` | pass | 退出码 0；报告更新前后无空白错误 |

### 生产只读检查

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| systemd | pass | `active/running`，`MainPID=3160229`、`NRestarts=0`、`ExecMainStatus=0`；启动时间 `2026-09-06 21:23:35 CST` |
| `/health` | pass | HTTP 200，返回 `status=ok` |
| `/ready` | pass | HTTP 200，返回 `status=ready` |
| OpenResty | pass | `docker exec workmesh-openresty-waf nginx -t` syntax ok、test successful |
| SQLite 只读 | pass | URI `mode=ro`；`quick_check=ok`、`integrity_check=ok`、外键违规 0；`schema_migrations=15`、`migration_runs=121` |

### 未覆盖的真实业务验收

登录后 385 条 HTTP、4 条 WS 业务消息、Go/Node/Python/Java/.NET/PHP 运行时完整生命周期、PHP FPM/扩展/配置/站点访问、六类网站及 `*.cs.sopvip.com` 全量访问、主/次节点 Gateway/心跳/任务透传、Let's Encrypt HTTP-01 正式签发/安装/续期仍为 `blocked/not-run`。当前缺少有效管理员会话、次节点凭据及隔离外部运行时/ACME 资源，本轮没有使用模拟 JSON、固定响应或静态契约结果替代真实验收。

源码快照未变化时不重复完整门禁；后续需先准备真实凭据和隔离资源，再由唯一集成测试负责人按分组记录每个接口、WS 消息、资源副作用和回滚结果。

## 2026-09-06 21:02 冻结工作区完整集成门禁通过

状态：[x] 当前冻结源码快照门禁通过；真实外部业务验收仍为 `blocked/not-run`。开发写入停止后，本轮于 `2026-09-06 20:57:29 CST` 记录快照，执行唯一完整集成门禁，并于 `21:02:20 CST` 记录结束快照。测试前后源码集合哈希一致，证明以下结果覆盖同一工作区快照。

本轮没有修改业务代码、前端或 `apps/1Panel`，没有编译前端、部署、重启服务或写入生产数据库；只在最后更新本报告。

### 冻结快照哈希

Git HEAD 前后均为 `49319f469c0ccf66ce127419b857af7edba7c59b`。四个重点文件和选定测试输入集合前后 SHA-256 均一致：

| 对象 | 开始及结束 SHA-256 |
| --- | --- |
| `node/service/taskruntime/taskruntime.go` | `f8825ab85c92b54c3ed7c50bc6719855bd95cc54c13c415f8ec561f0f39211d6` |
| `node/service/taskruntime/backend_cli.go` | `27cd176bf0c3dd4dbd3885b46336653291fbcd66ac086b891479886ee38e4e9e` |
| `control/api/role.go` | `e6336591c04e5f82847a0c8ead8d6416e45c5ab81de8b79d1badcbf7d5951775` |
| `node/api/containers_operations.go` | `319e92e1e0a540dbf6f823a858371579f8122261e34acb4b4a632b5b59a3cf37` |
| 选定 Go/合约输入集合 | `149cbf8404e0e7bcd8a345d0c02e97d608d620db3b7e40fc90bd532a031eabea` |

输入集合仍使用此前约定的 `rg --files --hidden` 路径排序、逐文件 SHA-256 汇总方式，排除文档、前端依赖、构建产物、运行数据和合约结果目录；报告更新不计入集合。

### 自动化门禁

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | 退出码 0；`node/api` 68.346s、`node/service` 11.910s；其余可测试包通过、缓存通过或无测试 |
| `GOWORK=off go test -race ./...` | pass | 退出码 0；`node/api` 119.444s、`node/service` 19.466s；未报告数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7，0 失败，总耗时 6.073s |
| `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest test/contract/routes.json` | pass | 759 条基线路由通过，192 条扩展路由兼容允许；参考目录只读 |
| `git diff --check` | pass | 退出码 0；报告更新前后检查无空白错误 |

### 生产只读核对

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| systemd | pass | `active/running`，`MainPID=3119094`、`NRestarts=0`、`ExecMainStatus=0`；启动时间 `2026-09-06 20:59:28 CST` |
| `/health` | pass | HTTP 200，`{"code":200,"data":{"status":"ok"}}` |
| `/ready` | pass | HTTP 200，`{"code":200,"data":{"status":"ready"}}` |
| OpenResty | pass | `docker exec workmesh-openresty-waf nginx -t` syntax ok、test successful |
| SQLite 只读 | pass | URI `mode=ro`；`quick_check=ok`、`integrity_check=ok`、外键违规 0；`schema_migrations=15`、`migration_runs=113` |

候选模板与生产二进制本轮未重新部署；现有两份文件 SHA-256 均为 `4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f`。该一致性和健康检查不证明当前工作区源码已经包含在生产制品中。

### 未覆盖的真实外部验收

登录后 385 条 HTTP 业务接口、4 条 WS 业务消息序列、Go/Node/Python/Java/.NET/PHP 运行环境完整生命周期、PHP FPM/扩展/配置/站点访问、六类网站及 `*.cs.sopvip.com` 全量访问、主/次节点 Gateway/心跳/任务透传、Let's Encrypt HTTP-01 正式签发/安装/续期仍为 `blocked/not-run`。当前缺少有效管理员会话、次节点凭据和隔离外部运行时/ACME 资源，本轮没有使用模拟 JSON、固定响应或静态路由结果代替真实验收。

源码快照未变化时不重复本完整门禁；下一步由主控基于该冻结快照组织需要的构建/部署和隔离真实业务验收，再由唯一测试负责人记录逐接口、逐 WS 消息及资源副作用证据。

## 2026-09-06 20:39 冻结快照完整门禁通过（开始/结束哈希一致）

状态：[x] 已完成当前冻结源码快照的完整门禁；[!] 真实外部业务验收仍为 `blocked/not-run`。主控确认源码冻结后，唯一集成测试负责人于 `2026-09-06 20:31:13 CST` 记录开始快照，执行完整门禁与生产只读检查，于 `20:39:35 CST` 记录结束快照；下面四文件与所选测试输入集合的 SHA-256 前后完全一致。本轮结果替代此前受并行写入影响的最终源码门禁结论，历史失败的快照检查仍保留供追溯。

### 冻结快照证据

Git HEAD 前后均为 `49319f469c0ccf66ce127419b857af7edba7c59b`。工作区含未提交修改，不能仅以 HEAD 代替源码快照；本轮另验证以下 SHA-256：

| 对象 | 开始及结束 SHA-256 | 前后比对 |
| --- | --- | --- |
| `node/service/taskruntime/taskruntime.go` | `f8825ab85c92b54c3ed7c50bc6719855bd95cc54c13c415f8ec561f0f39211d6` | 一致 |
| `node/service/taskruntime/backend_cli.go` | `27cd176bf0c3dd4dbd3885b46336653291fbcd66ac086b891479886ee38e4e9e` | 一致 |
| `control/api/role.go` | `e6336591c04e5f82847a0c8ead8d6416e45c5ab81de8b79d1badcbf7d5951775` | 一致 |
| `node/api/containers_operations.go` | `f3c3f2f067c63c508ea881111071fe7e4d0579a64be39d502491e36ad36258ae` | 一致 |
| 所选测试输入集合 | `3a3f22b5f072ce9aa51ca8be84a07b5308e1fed487d0fa62e4fc611e79056dec` | 一致 |

集合摘要的可复现命令如下，文档变更不包含在该集合内；本轮没有编译前端：

```bash
rg --files --hidden -g '*.go' -g 'go.mod' -g 'go.sum' -g '*.mjs' -g '*.json' \
  -g '!web/**' -g '!node_modules/**' -g '!.git/**' -g '!.cache/**' -g '!.tmp/**' \
  -g '!.build/**' -g '!data/**' -g '!.workmesh-data/**' -g '!docs/**' \
  -g '!test/contract/results/**' | LC_ALL=C sort | xargs -d '\n' sha256sum | sha256sum
```

### 自动化门禁

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | 退出码 0；`node/api` 68.916s、`node/service` 11.568s、`taskruntime` 0.006s；其余包通过、缓存通过或无测试 |
| `GOWORK=off go test -race ./...` | pass | 退出码 0；`node/api` 119.223s、`node/service` 20.253s、`taskruntime` 1.033s；未报告数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7，0 失败，4.434s |
| `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest test/contract/routes.json` | pass | 759 条基线路由通过，192 条扩展路由兼容允许；只读参考项目 |
| `git diff --check` | pass | 退出码 0；报告更新后再次检查 |

### 已部署服务只读检查

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| systemd | pass | `active/running`，PID 3078097，`NRestarts=0`、`ExecMainStatus=0`，启动时间 `2026-09-06 20:35:20 CST` |
| health / ready | pass | 均 HTTP 200，分别返回 `status=ok`、`status=ready` |
| OpenResty | pass | `docker exec workmesh-openresty-waf nginx -t`：syntax ok、test successful |
| SQLite 只读 | pass | URI `mode=ro`；`quick_check=ok`、`integrity_check=ok`、外键违规 0，`schema_migrations=15`、`migration_runs=105` |
| 模板候选与生产制品 | 一致 | SHA-256 均为 `4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f`；此项不证明当前源码已编入部署制品 |

本负责人只更新集成报告和状态入口，未修改业务代码、前端或 `apps/1Panel`，未编译前端、部署、重启服务或写入生产数据库。本轮生产检查只证明已部署制品基础健康，与冻结源码门禁分别记录。

### 未覆盖与下一步

登录后的 385 条 HTTP 业务接口、4 条 WS 业务消息序列、六类运行时完整生命周期（含 PHP FPM/扩展/配置）、六类网站及 `*.cs.sopvip.com` 全量访问、主次节点 Gateway/心跳/任务透传、Let's Encrypt HTTP-01 正式签发/安装/续期，继续保持 `blocked/not-run`。本轮未提供真实管理员会话、次节点凭据及完整可回滚外部资源，不用模拟 JSON、固定成功或静态契约覆盖代替真实验收。

下一步由主控基于已验证冻结快照组织构建/部署（如需要）及隔离真实业务验收，并由唯一测试负责人记录结果；源码不变时不重复完整门禁。任何后续源码写入均需重新确认受影响门禁范围。

## 2026-09-06 20:26 最终快照复验：命令通过，但快照漂移使最终验收无效

状态：[!] 阻塞。测试前记录时间为 `2026-09-06 20:19:33 CST`，测试后哈希检查时间为 `20:26:51 CST`。本轮所有命令通过，但期间又出现 taskruntime 源码写入，不能将本轮结果标记为覆盖当前最终源码。之前 20:14 章节也因测试期间存在并行写入，不能作为最终快照发布依据。

### 快照证据

Git HEAD 前后均为 `49319f469c0ccf66ce127419b857af7edba7c59b`；工作区未提交源码另以 SHA-256 校验。集合摘要覆盖 `rg --files --hidden` 返回的 `*.go`、`go.mod`、`go.sum`、`*.mjs`、`*.json`，排除 `web/`、`node_modules/`、`.git/`、`.cache/`、`.tmp/`、`.build/`、`data/`、`.workmesh-data/`、`docs/`、`test/contract/results/`，路径经 `LC_ALL=C sort` 排序后对逐文件 `sha256sum` 输出再次计算 SHA-256。

| 对象 | 开始 SHA-256 | 结束 SHA-256 | 结论 |
| --- | --- | --- | --- |
| `control/api/role.go` | `e6336591c04e5f82847a0c8ead8d6416e45c5ab81de8b79d1badcbf7d5951775` | 同开始 | 未变化 |
| `node/api/containers_operations.go` | `f3c3f2f067c63c508ea881111071fe7e4d0579a64be39d502491e36ad36258ae` | 同开始 | 未变化 |
| 所选测试输入集合 | `a942d3f5601b72f6a887ff4f2c95cc7108612ec2949972bf23e18ae31735c212` | `3a3f22b5f072ce9aa51ca8be84a07b5308e1fed487d0fa62e4fc611e79056dec` | 已变化，不可验收 |

最新源码写入时间：`node/service/taskruntime/taskruntime.go` 为 `20:26:46.317 CST`，`node/service/taskruntime/backend_cli.go` 为 `20:26:46.318 CST`。结束时两文件 SHA-256 分别为 `f8825ab85c92b54c3ed7c50bc6719855bd95cc54c13c415f8ec561f0f39211d6`、`27cd176bf0c3dd4dbd3885b46336653291fbcd66ac086b891479886ee38e4e9e`。本负责人没有修改它们。

### 本轮实际执行结果（不等同最终快照通过）

| 检查 | 命令结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | `node/api` 68.845s、`node/service` 12.251s、`control/api` 0.109s；全部包退出码 0 |
| `GOWORK=off go test -race ./...` | pass | `node/api` 119.613s、`node/service` 19.392s；未报告数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0 |
| `node --test test/contract/*.test.mjs` | pass | 7/7，4.998s |
| `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest test/contract/routes.json` | pass | 759 条，192 条扩展路由兼容允许 |
| `git diff --check` | pass | 退出码 0 |
| systemd | pass | `active/running`，PID 3056333，`NRestarts=0`、`ExecMainStatus=0`，启动时间 `20:22:47 CST` |
| health / ready | pass | 均 HTTP 200，分别 `status=ok`、`status=ready` |
| OpenResty | pass | `docker exec workmesh-openresty-waf nginx -t` 成功 |
| SQLite 只读 | pass | `mode=ro`；quick/integrity 均 `ok`，外键违规 0，迁移版本记录 15、运行记录 97 |

候选模板制品与生产制品 SHA-256 仍均为 `4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f`；只代表文件一致和现有生产基础健康，不证明包含当前工作区源码。本轮未改业务代码、前端、参考项目，没有部署、重启或写入生产库。

下一步：主控先冻结所有业务源码写入，再由唯一集成测试负责人记录冻结快照并重新门禁；若仍在持续写入，先完成开发再测试，避免无效重测。真实登录后 385 条 HTTP、4 条 WS、六类运行时/网站、主次节点及 ACME 验收继续为 `blocked/not-run`，不以模拟业务数据代替。

## 2026-09-06 20:14 应用测试注释批次完整门禁与生产只读核对

状态：[x] 已完成本轮集成门禁；[!] 真实外部业务验收仍阻塞。本轮由唯一集成测试负责人复核应用测试注释批次后的当前工作区，没有修改业务代码、编译前端或修改 `apps/1Panel`，没有部署、重启服务或写入生产数据库。

### 自动化门禁

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | 退出码 0；`node/api` 68.006s、`node/service` 11.647s，其余包通过、缓存通过或无测试 |
| `GOWORK=off go test -race ./...` | pass | 退出码 0；`node/api` 120.141s、`node/service` 19.745s、`control/api` 1.245s；未报告数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7 通过，0 失败，总耗时 5.075s |
| `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest test/contract/routes.json` | pass | 759 条基线路由通过，192 条扩展路由兼容允许；只读参考项目 |
| `git diff --check` | pass | 退出码 0，无输出 |

### 生产只读核对

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| systemd | pass | `active/running`，`MainPID=3035051`、`NRestarts=0`、`ExecMainStatus=0`；启动时间 `2026-09-06 20:11:07 CST` |
| `/health`、`/ready` | pass | 均 HTTP 200，分别返回 `status=ok`、`status=ready` |
| OpenResty | pass | `docker exec workmesh-openresty-waf nginx -t`：syntax ok、test successful |
| SQLite | pass | URI `mode=ro`；`quick_check=ok`、`integrity_check=ok`、外键违规 0；`schema_migrations=15`、`migration_runs=89` |
| 候选与生产制品 | 一致 | `.build/workmesh-server-linux-amd64-template` 与 `/opt/workmesh-server/bin/workmesh-server` SHA-256 均为 `4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f`；哈希一致不等同于证明制品包含所有当前源码变更 |

### 未覆盖与下一步

登录后 385 条 HTTP 业务接口、4 条 WS 业务消息序列、Go/Node/Python/Java/.NET/PHP 六类运行时生命周期（含 PHP FPM/扩展/配置）、六类网站及 `*.cs.sopvip.com` 全量访问、主/次节点 Gateway/心跳/任务透传、Let's Encrypt HTTP-01 正式签发/安装/续期，继续保持 `blocked/not-run`。本轮未提供有效管理员会话、次节点凭据及完整可回滚外部资源，未以模拟 JSON、固定业务响应或静态路由覆盖替代真实验收。

本轮只更新此报告及状态入口；源码未发生新变化时不重复运行上述完整门禁。下一步由主控组织具备真实凭据和隔离资源的业务验收，再交唯一测试负责人记录每项实际请求与结果。

## 2026-09-06 19:57 taskruntime 注释批次完整门禁与发布后只读核对

本轮复核 `node/service/taskruntime` 注释批次及当前工作区，未修改业务代码、未编译前端、未写入或修改 `apps/1Panel`，未使用模拟业务数据。由于前端未变更，本轮没有执行前端构建。

### 自动化门禁

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | 所有可测试 Go 包通过；`node/api` 67.896s、`node/service` 11.367s、`taskruntime` 0.006s；其余包通过或无测试 |
| `GOWORK=off go test -race ./...` | pass | 所有可测试 Go 包通过；`node/api` 119.939s、`node/service` 20.067s、`taskruntime` 1.038s；未发现数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7 通过，0 失败，运行 3.496s |
| `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest test/contract/routes.json` | pass | 759/759 路由通过，192 条扩展路由兼容允许；仅读取参考目录 |
| `git diff --check` | pass | 退出码 0，无空白错误 |

### 发布后只读检查

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| systemd | pass | `active/running`，`MainPID=3005777`，`NRestarts=0`，`ExecMainStatus=0`；`ActiveEnterTimestamp=Sun 2026-09-06 19:53:36 CST` |
| `/health` | pass | HTTP 200，`{"code":200,"data":{"status":"ok"}}` |
| `/ready` | pass | HTTP 200，`{"code":200,"data":{"status":"ready"}}` |
| OpenResty | pass | `docker exec workmesh-openresty-waf nginx -t` syntax ok、test successful |
| SQLite 只读完整性 | pass | URI `mode=ro`；`quick_check=ok`、`integrity_check=ok`、外键违规 0；`schema_migrations=15`、`migration_runs=85` |
| 候选与生产制品一致性 | pass | `.build/workmesh-server-linux-amd64-template` 与 `/opt/workmesh-server/bin/workmesh-server` SHA-256 均为 `4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f` |
| `apps/1Panel` 修改检查 | pass | `git diff --name-only -- apps/1Panel` 无输出；参考项目仅被只读扫描 |

### 仍未运行的真实外部验收

登录后 385 条 HTTP 业务接口、4 条 WS 业务消息序列、Go/Node/Python/Java/.NET/PHP 六类运行环境完整生命周期、PHP FPM/扩展/配置/站点访问、六类网站及 `*.cs.sopvip.com` 全量访问、主/次节点 Gateway/心跳/任务透传、Let's Encrypt HTTP-01 正式签发/安装/续期仍保持 `blocked/not-run`。原因是缺少有效管理员会话、次节点凭据和完整可回滚的外部运行时及 ACME 资源；本轮没有用模拟 JSON、固定响应或空数组替代真实验收。

## 2026-09-06 19:36 gateway 注释批次完整门禁与发布后核对

本轮复核 `control/api/gateway.go` 中文方法注释及当前代码批次，未修改业务代码、未编译前端、未写入或修改 `apps/1Panel`。

### 自动化门禁

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | `control/api` 0.103s、`node/api` 69.814s、`node/service` 12.783s；其余 Go 包通过或无测试 |
| `GOWORK=off go test -race ./...` | pass | `control/api` 1.264s、`node/api` 121.245s、`node/service` 22.655s；未发现数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7 通过 |
| `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest test/contract/routes.json` | pass | 759/759 路由通过，192 条扩展路由兼容允许；仅读取参考目录 |
| `git diff --check` | pass | 退出码 0 |

### 发布后只读检查

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| systemd/健康 | pass | `active/running`，`NRestarts=0`；`/health`、`/ready` 均 HTTP 200 |
| OpenResty | pass | `docker exec workmesh-openresty-waf nginx -t` syntax/test successful |
| SQLite | pass | `quick_check=ok`、`integrity_check=ok`、外键违规 0；`schema_migrations=15`、`migration_runs=77` |

### 未运行真实凭据项目

登录后 385 条 HTTP、4 条 WS 消息、六类 Go/Node/Python/Java/.NET/PHP 运行时生命周期、六类网站及 `*.cs.sopvip.com` 全量访问、主/次节点 Gateway/心跳/任务透传、Let's Encrypt HTTP-01 仍保持 `blocked/not-run`。当前缺少真实管理员会话、次节点凭据和可回滚的外部运行时/ACME 资源，未使用模拟 JSON 或固定响应替代。

## 2026-09-06 19:27 当前代码批次完整门禁与发布后只读核对

本轮复核 website_extensions helper 拆分、apps.go 质量注释及质量报告刷新后的当前工作区。未修改业务代码、未编译前端、未写入或修改 `apps/1Panel`。

### 自动化门禁

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | `node/api` 68.236s、`node/service` 12.280s；其余 Go 包通过或无测试 |
| `GOWORK=off go test -race ./...` | pass | `node/api` 118.614s、`node/service` 20.144s；未发现数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7 通过 |
| `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest test/contract/routes.json` | pass | 759/759 路由通过，192 条扩展路由兼容允许；仅读取参考目录 |
| `git diff --check` | pass | 退出码 0 |

### 发布后只读检查

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| systemd 与健康端点 | pass | `active/running`，`NRestarts=0`；`/health`、`/ready` 均 HTTP 200 |
| OpenResty | pass | `docker exec workmesh-openresty-waf nginx -t` syntax/test successful |
| 已配置站点访问 | pass | `Host: znmp.sopvip.com` HTTP 200，返回真实站点 HTML |
| SQLite | pass | `quick_check=ok`、`integrity_check=ok`、外键违规 0；`schema_migrations=15`、`migration_runs=69` |

### 未运行真实凭据项目

以下项目仍保持 `blocked/not-run`：登录后 385 条 HTTP、4 条 WS 业务消息、Go/Node/Python/Java/.NET/PHP 运行时完整生命周期、六类网站及 `*.cs.sopvip.com` 全量访问、主/次节点 Gateway/心跳/任务透传、Let's Encrypt HTTP-01。原因是当前没有可用管理员会话、次节点凭据或可回滚的外部运行时/ACME 资源；没有使用模拟 JSON、固定成功或空数组替代真实验收。

## 2026-09-06 18:43 WAF SQLite 持久化最终门禁与发布后核对

本轮仅复核 WAF SQLite 持久化收口，未修改业务代码、未编译前端、未写入或修改 `apps/1Panel`。

### WAF 定向测试

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `node/service` WAF 定向测试 | pass | `TestWAFSQLiteDoesNotWriteSidecars`、`TestWAFLegacySidecarsImportedOnce`、`TestWAFNoDBSidecarsRemainCompatible`，3/3，1.015s |
| `node/api` WAF 定向测试 | pass | `TestWebsiteWAFRoutesCRUD`、`TestWebsiteWAFTestDetectsSample`，2/2，1.405s |
| WAF 数据源结论 | pass | SQLite 启用时站点/全局/访问列表写入关系表，不生成 JSON sidecar；旧 sidecar 只导入一次；无 DB 测试路径保持兼容 |

### 完整门禁

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | pass | `node/api` 70.057s、`node/service` 13.722s；其余 Go 包通过或无测试 |
| `GOWORK=off go test -race ./...` | pass | `node/api` 118.187s、`node/service` 19.503s；未发现数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7 通过 |
| `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest test/contract/routes.json` | pass | 759/759 路由通过，192 条扩展路由兼容允许；仅只读参考项目 |
| `git diff --check` | pass | 退出码 0 |

### 发布后只读检查

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| systemd/健康 | pass | `active/running`，`NRestarts=0`，`/health` 和 `/ready` 均 HTTP 200 |
| OpenResty | pass | `nginx -t` syntax/test successful |
| 已配置站点访问 | pass | `Host: znmp.sopvip.com` HTTP 200，返回真实站点 HTML |
| SQLite 完整性 | pass | `quick_check=ok`、`integrity_check=ok`、外键违规 0、`schema_migrations=15`、`migration_runs=57`，最新迁移 `0015-website-template-relational` |
| 候选与生产一致性 | pass | `.build/workmesh-server-linux-amd64-template` 与 `/opt/workmesh-server/bin/workmesh-server` SHA-256 均为 `4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f` |

本轮证明 WAF SQLite 持久化、完整源码门禁和当前候选发布后基础设施均通过；登录后的全量 HTTP/WS、六类运行时真实生命周期、主/次节点闭环和正式 ACME HTTP-01 仍按清单保持 `blocked/not-run`，不使用模拟数据替代。

## 2026-09-06 18:11 发布候选与待验收清单只读核对

本轮只检查 `workmesh-server` 当前发布候选、生产运行状态和待验收条件，未修改业务代码、未编译前端、未写入或修改 `apps/1Panel`。

### 发布候选状态

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 发布候选文件 | pass | `.build/workmesh-server-linux-amd64-template`，SHA-256=`4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f` |
| 生产二进制一致性 | pass | `/opt/workmesh-server/bin/workmesh-server` SHA-256 与候选完全一致：`4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f` |
| 发布备份 | pass | `/opt/workmesh-server/backups/deploy-20260906T090558Z-website-template`，包含旧二进制、候选 hash、SQLite 主库及 WAL/SHM、OpenResty 配置 |
| systemd | pass | `workmesh-server.service=active/running`，`NRestarts=0`，`MainPID=2786246` |
| 健康与就绪 | pass | `/health` HTTP 200，`status=ok`；`/ready` HTTP 200，`status=ready` |
| OpenResty | pass | `docker exec workmesh-openresty-waf nginx -t` syntax/test successful |
| SQLite 完整性 | pass | `quick_check=ok`、`integrity_check=ok`、外键违规 0；`schema_migrations=15`，包含 `0015-website-template-relational`，`migration_runs=49` |
| 候选构建来源 | observe | Go 构建元数据 revision=`49319f469c0ccf66ce127419b857af7edba7c59b`、`vcs.modified=true`；候选来自 dirty 工作区，后续可复现发布仍需固定 release commit |

### 待验收状态

| 范围 | 状态 | 原因/边界 |
| --- | --- | --- |
| 后端源码自动化门禁 | pass | 最近一次网站、模板、鉴权、监控日志定向测试及全量 Go/race/vet 均通过 |
| 759 条参考路由契约 | pass | `routes.json` 来源 `/www/apps/1Panel`，只读 route-scan 已通过 759/759；未修改参考项目 |
| 前端 HTTP 385 条登录后业务接口 | blocked | 当前未发现有效管理员 Cookie/Token；不能用静态清单或模拟 JSON 代替真实请求 |
| WS 4 条业务消息序列 | blocked | 缺少有效登录会话及可验证的终端/容器/SSH 外部资源；不能伪造消息结果 |
| Go/Node/Python/Java/.NET/PHP 运行环境生命周期 | blocked | 缺少可安全创建和清理的真实运行时资源；PHP FPM、扩展、配置和站点访问未验证 |
| 六类网站及域名访问闭环 | blocked | 缺少完整真实测试资源和登录会话；`*.cs.sopvip.com` 正式域名业务矩阵仍未逐项执行 |
| 主站点/次站点 Gateway、心跳、任务透传 | blocked | 次节点凭据/可达性和跨节点真实环境未提供 |
| Let's Encrypt HTTP-01 签发/安装/续期 | not-run | 没有本轮专用 ACME 账户、DNS/域名验证窗口或可回滚的真实证书测试资源；禁止用自签或模拟结果替代 |

当前候选已部署且基础设施健康，但以上 `blocked/not-run` 项仍不能宣称正式上线业务验收完成。执行这些项目需要真实管理员会话、次节点凭据、可回滚的运行时/Docker 资源及 ACME 验证资源；前端无变更，不需要重新编译。

## 2026-09-06 17:43 鉴权兼容修复后最终门禁

本轮复核主控完成的 `/api/v2/websites/auths` 同路径查询/更新兼容修复，并重新执行唯一集成测试流水线。未修改业务代码、未编译前端、未修改 `apps/1Panel`。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 正式路由基线来源 | pass | `test/contract/routes.json` 的 `generatedFrom=/www/apps/1Panel`，共 759 条路由 |
| 只读路由契约扫描 | pass | `route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest test/contract/routes.json`：759/759 通过，192 条扩展路由兼容允许；未写入参考项目 |
| 网站/鉴权/监控日志定向测试 | pass | `node/api` 16.645s；`TestWebsiteManagedSettingFragmentsAndRollback`、`TestWebsiteMonitorLogsReadSiteFile`、分析统计、网站配置、SSL/OpenResty/WAF 全部通过 |
| `node/service` 定向测试 | pass | 11.282s；网站、SSL、WAF、OpenResty、运行时相关测试全部通过 |
| `internal/storage` 定向迁移测试 | pass | 0.318s；SQLite 启动、旧数据导入幂等、迁移 checksum 检查全部通过 |
| `GOWORK=off go test ./...` | pass | `node/api` 66.580s、`node/service` 10.494s；其余 Go 包通过或无测试 |
| `GOWORK=off go test -race ./...` | pass | `node/api` 116.254s、`node/service` 17.287s；未发现数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7 通过 |
| `git diff --check` | pass | 退出码 0 |

本轮证明鉴权同路径兼容修复后的源码自动化门禁通过，同时确认正式路由基线已从旧 `apps/workmesh-node` 切换为只读 `/www/apps/1Panel`。不等于登录后的全部 HTTP/WS 业务接口、生产二进制部署或全量线上业务验收已完成。

## 2026-09-06 17:35 路由基线切换与网站监控日志门禁复核

本轮先只读校验正式路由基线，再执行全量 Go 门禁。`test/contract/routes.json` 已确认来源切换为 `/www/apps/1Panel`，共 759 条路由；执行 `route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest test/contract/routes.json` 结果为 `759/759` 通过，192 条扩展路由兼容允许。未写入或修改 `apps/1Panel`。

网站监控日志重点定向测试此前已通过：带 `websiteID` 时读取真实站点 `logs/access.log`，并验证 monitor logs search/detail/stat/clear 及旧网站日志接口兼容。

本轮全量门禁在普通 Go 测试阶段阻塞，首个失败如下：

```text
node/api: TestWebsiteManagedSettingFragmentsAndRollback
/api/v2/websites/auths 未生成 nginx/auth_basic/managed.conf
```

当前路由实现中 `/api/v2/websites/auths` 是读取接口（`ListWebsiteAuths`），更新接口为 `/api/v2/websites/auths/update`；失败测试却向 `/auths` 提交 `username/password` 并要求生成鉴权配置片段。由于 `GOWORK=off go test ./...` 未通过，本轮按门禁顺序未继续执行 race、vet、Node 合约和 diff 检查，避免把后续结果与普通测试失败混淆。该失败与本轮监控日志改动无直接关系，需主控先统一鉴权接口/测试契约后重新开始全量门禁。

## 2026-09-06 17:29 网站监控日志变更门禁

本轮仅验证网站监控日志改动，未修改业务代码、未编译前端、未触碰 `apps/1Panel`。重点覆盖带 `websiteID` 时从真实站点 `logs/access.log` 读取，以及网站监控日志 search/detail/stat/clear 的响应链路和旧网站日志接口兼容。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| Go 格式检查 | pass | 监控日志相关 `analytics.go`、`website_monitor_logs.go`、测试文件均已 `gofmt`；未修改无关的既有格式问题 |
| 监控日志重点定向测试 | pass | `TestAnalyticsReadsBoundedAccessLogAndAggregatesMetrics`、`TestWebsiteMonitorLogsReadSiteFile`、`TestWebsiteExtendedFieldsAndLogs`、`TestZNMPStaticWebsiteAllInterfaces` 等全部通过，`node/api` 5.651s |
| 网站/SSL/OpenResty/SQLite 定向测试 | pass | `node/api` 18.033s、`node/service` 8.705s、`internal/storage` 0.599s；模板、WAF、文件、网站配置及迁移测试全部通过 |
| `GOWORK=off go test ./...` | pass | `node/api` 70.099s、`node/service` 10.791s；其余 Go 包通过或无测试 |
| `GOWORK=off go test -race ./...` | pass | `node/api` 119.363s、`node/service` 17.346s；未发现数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7 通过，包含 route-scan 本地保护与版本映射测试 |
| `git diff --check` | pass | 退出码 0 |
| 759 条参考项目路由扫描 | not-run | 主控要求本轮不触碰 `apps/1Panel`，未执行需要读取该目录的 `route-scan.mjs check`；不能以本地 route-scan 单测替代 |

本轮自动化结果证明网站监控日志代码和本地真实文件读取链路通过；不等于已完成登录后的全量 HTTP/WS 业务验收，也不等于生产二进制已包含本轮改动。

## 2026-09-06 16:58 模板关系化与 0015 迁移复核

本轮仅复核主控在 16:47 后加入的 `WebsiteTemplateMigration`/`0015-website-template-relational` 变更，未修改业务代码、未编译前端、未写入或修改 `apps/1Panel`。迁移已在 `cmd/workmesh-server/main.go` 启动迁移列表中注册，关系表包含 `website_templates`、`website_template_outputs` 及 `website_template_migrations`，旧 `website_extension_state` 模板数据通过完成标记进行一次性、幂等导入。

门禁结果：

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 模板/网站定向 Go 测试 | pass | `node/api` 17.117s；模板旧 JSON 一次性迁移、持久化预览、产物 SQLite CRUD、真实 ZIP 上传、非法 ZIP 拒绝及网站/SSL/OpenResty/WAF 测试全部通过 |
| 启动迁移/SQLite 存储检查 | pass | `TestOpenConfiguresSQLiteAndCreatesSchemaIdempotently`、`TestImportLegacyJSONIsIdempotentAndPreservesSource`、`TestApplyMigrationsRejectsChecksumConflict` 全部通过（0.227s） |
| `GOWORK=off go test ./...` | pass | `node/api` 66.273s；`node/service` 11.789s；其余 Go 包通过或无测试 |
| `GOWORK=off go test -race ./...` | pass | `node/api` 121.114s；`node/service` 22.112s；未发现数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `node --test test/contract/*.test.mjs` | pass | 7/7 通过 |
| 只读路由扫描 | pass | `759/759` 路由通过，`192` 条扩展路由兼容允许；仅读取 `/www/apps/1Panel`，未写入 |
| `git diff --check` | pass | 退出码 0 |

本轮证明 0015 的关系化模板结构、启动迁移注册、旧数据幂等导入和自动化门禁通过；不等于已完成登录后的全量 HTTP/WS 业务验收，也不等于该变更已经部署到生产二进制。

## 2026-09-06 16:43 本轮门禁与兼容修复

本轮在不读取或修改 `apps/1Panel`、不改前端、不使用模拟业务数据的前提下，修复并验证了四个真实契约问题：

- 默认页面更新接口兼容旧调用方只提交 `websiteID/content` 的请求；缺省类型按历史行为落到 `index`，显式类型仍严格限制为 `404`、`domain404`、`index`、`php`、`stop`。
- 网站模板上传复合测试改为真实 `multipart/form-data` ZIP 上传，接口仍拒绝缺少文件或非法 ZIP。
- 代理状态切换读取并合并 SQLite 中已有代理配置，兼容前端只提交 `id/name/status` 的请求，不要求重复提交 `proxyPass`。
- 修正代理文件名校验中的原始字符串误用；合法名称不再因包含字母 `n` 被错误拒绝，路径穿越和控制字符仍被拒绝。

门禁结果：

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 网站/SSL/OpenResty/SQLite 定向 Go 测试 | pass | `node/service` 9.276s；`node/api` 17.012s；`internal/storage` 0.134s |
| `GOWORK=off go test ./...` | pass | `node/api` 66.033s；`node/service` 11.322s；其余 Go 包通过或无测试 |
| `GOWORK=off go test -race ./...` | pass | `node/api` 112.227s；`node/service` 18.510s；未发现数据竞争 |
| `GOWORK=off go vet ./...` | pass | 退出码 0，无输出 |
| `git diff --check` | pass | 退出码 0 |
| `node --test test/contract/*.test.mjs` | pass | 7/7 通过；路由扫描输出改为独立临时目录，仍保持禁止写入只读参考树的保护 |

本轮结果只证明源码自动化门禁和本地真实 SQLite/文件/OpenResty 逻辑通过，不扩大解释为已完成登录后 HTTP/WS 业务验收。生产黑盒、六类运行环境完整生命周期、主/次节点跨节点闭环、真实 Let's Encrypt HTTP-01 签发仍按下方清单保持 `not-run`/`blocked`，没有用模拟数据替代。

### 本轮生产只读状态（2026-09-06 16:43 CST）

- `workmesh-server.service`：`active`，`NRestarts=0`；`/health` 和 `/ready` 均 HTTP 200。
- `docker exec workmesh-openresty-waf nginx -t`：通过。
- `/opt/workmesh-server/data/workmesh.db`：只读 `quick_check=ok`、`integrity_check=ok`、外键违规 0 条。
- 生产二进制仍为 SHA-256 `bfd9f0d6dd74a0133774e5081e2f7f43a0a49cb8d7656311390457c64f6d21c9`；本轮后端兼容修复尚未部署，不能将本轮源码门禁结果表述为线上已生效。

- 只读复核时间：2026-09-06 16:11（Asia/Shanghai）
- 当前发布状态：生产已运行包含当前工作区迁移审计改动的制品；源码工作区仍为 dirty，未形成新的 commit。

## 最新生产制品与源码一致性只读复核（2026-09-06 16:11 CST）

本轮按要求没有执行测试、构建、部署或任何数据库写入，也没有读取/修改 `apps/1Panel`。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 当前源码 HEAD | 观察 | `49319f469c0ccf66ce127419b857af7edba7c59b`；工作区 `244` 个 dirty 文件，最新 Go 源码 mtime `15:45:39` |
| 生产二进制 SHA-256 | 通过 | `bfd9f0d6dd74a0133774e5081e2f7f43a0a49cb8d7656311390457c64f6d21c9`；mtime `15:55:02`；大小 `26448045` bytes |
| 生产二进制 Go 构建元数据 | 通过（revision 级） | `vcs.revision=49319f469c0ccf66ce127419b857af7edba7c59b`、`vcs.modified=true`；与当前 HEAD 一致，明确为 dirty 工作区构建 |
| 当前 workspace `.build/workmesh-server-linux-amd64` | 观察 | SHA-256=`93cab71c9ee4bed820eb8aee8ea08f88b65fd8110191c28ffb2ef35bcb86ccb2`，不是当前生产制品 |
| systemd | 通过 | `workmesh-server.service=active`，`MainPID=2602114`，`NRestarts=0`，启动时间 `15:55:02 CST` |
| `/health` / `/ready` | 通过 | 分别 HTTP 200，`status=ok` / `status=ready` |
| 最新发布备份 | 通过 | `/opt/workmesh-server/backups/deploy-20260906T155453+0800-migration-audit` 存在；包含旧二进制、配置、SQLite（含 WAL/SHM）、OpenResty compose 和网站归档 |
| 备份旧二进制 SHA-256 | 观察 | `b54a9bf395e0a9f2766a944390dca149baa76260bb24a95d9bf7369ac281f45b`；与当前生产制品不同，说明已发生原子替换 |
| 迁移审计制品绑定 | 通过 | 当前 SQLite `migration_runs` 最近 4 条均记录 artifact SHA-256=`bfd9f0d6dd74a0133774e5081e2f7f43a0a49cb8d7656311390457c64f6d21c9`，状态均为 `noop` |

### 部署结论

- 当前生产制品已经部署，不存在“当前源码尚未部署”的直接证据；二进制内嵌 revision 与当前 HEAD 相同，且标记 `vcs.modified=true`。
- 由于工作区包含 244 个未提交文件，Go 构建元数据只能证明 HEAD 和 dirty 状态，不能仅凭 revision 证明每一个未提交文件都进入了二进制；后续若要可复现发布，应先形成明确 release commit 或保存构建文件清单/哈希。
- 最新备份目录为 `/opt/workmesh-server/backups/deploy-20260906T155453+0800-migration-audit`；当前生产制品哈希为 `bfd9f0d6dd74a0133774e5081e2f7f43a0a49cb8d7656311390457c64f6d21c9`。

- 状态：[>] 当前源码自动化门禁通过；发布前只读与 SQLite 升级核对完成；真实业务验收仍有未覆盖项
- 更新时间：2026-09-06 15:09（Asia/Shanghai）
- 负责人：唯一集成测试负责人
- 范围：后端及契约验证；本轮未修改业务代码、`apps/1Panel/frontend` 或生产配置，未重新编译前端。

## 本轮发布前门禁复核与 SQLite 升级核对（2026-09-06 15:08:16 CST）

上轮门禁后检测到 `node/service/ssl_acme.go`、`ssl_sync.go` 及 `ssl_sync_test.go` 新增/修改，因此按规则重新执行普通 Go、定向网站/SSL、race、vet、Node 契约、路由和无凭据边界门禁。随后完成发布前只读核对；本轮没有修改业务代码、前端、生产配置或 SQLite。

### 门禁结果

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 网站/SSL 定向 Go 测试 | 通过 | `node/service` 10.743s；`node/api` 6.211s；SSL 持久化、证书校验、自签续期、DNS 兼容、网站 CRUD/WAF/OpenResty 均通过 |
| `GOWORK=off go test ./...` | 通过 | `node/api` 40.721s；`node/service` 9.074s；所有 Go 包通过 |
| `GOWORK=off go test -race ./...` | 通过 | `node/api` 66.458s；`node/service` 16.074s；未发现 race |
| `GOWORK=off go vet ./...` | 通过 | 退出码 0，无输出 |
| `git diff --check` | 通过 | 退出码 0 |
| `node --test test/contract/*.test.mjs` | 通过 | 7/7 通过 |
| 1Panel 路由扫描 | 通过 | 759 条通过；191 条扩展路由兼容允许 |
| HTTP 无凭据冒烟 | 通过 | 8/8 通过 |
| HTTP/WS 边界执行器 | 通过 | 7/7 通过 |

### SQLite 迁移/升级只读核对

当前运行库 `/opt/workmesh-server/data/workmesh.db` 使用项目自有 `schema_migrations` 账本；源码注册的 13 个迁移 ID 与数据库实际 13 个 ID 完全一致：

- 缺失迁移：`[]`；额外迁移：`[]`；
- 13/13 checksum 长度为 64，13/13 有 `applied_at`；
- `legacy_imports=4`，全部状态为 `imported`，旧 JSON 仅保留迁移审计；
- `migration_runs=0`，表示当前启动迁移器没有写入升级运行记录，不能把它误称为已有升级审计闭环；
- `PRAGMA user_version=0`，应用实际以 `schema_migrations` 版本账本为准；
- `quick_check=ok`、`integrity_check=ok`、外键违规 0、63 张表；
- 关键字段/表（运行时 payload/created_at、任务日志、网站 SSL/WAF/设置、操作/登录日志）均已存在，当前没有待应用迁移。

这次是只读核对，没有在生产库执行迁移、checkpoint 或升级写入；新制品正式部署时必须先备份二进制、配置和 SQLite（含 WAL/SHM），由启动迁移器在同一进程内执行并在 `/ready` 成功后复核。

### 发布前状态与阻塞项

- 当前生产二进制 SHA-256=`93cab71c9ee4bed820eb8aee8ea08f88b65fd8110191c28ffb2ef35bcb86ccb2`，mtime `2026-09-06 12:23:23 CST`，仍早于本轮源码变更；因此不能将当前运行制品视为最新源码发布结果。
- 当前服务仍 `active`，`NRestarts=0`；`/health`、`/ready`、`znmp.sopvip.com` HTTP/HTTPS 和 OpenResty 语法均通过。
- 发布阻塞：需要主控使用当前通过门禁源码重新构建、备份并原子部署，然后由本负责人执行部署后只读核对；不得跳过 SQLite 备份或把旧制品健康结果当作新制品验证。
- `nginx -t` 仍有 `worker_connections` 超过 `nofile=1024` 警告；不是语法失败，但上线前应评估资源限制。

## 本轮最终门禁收口（2026-09-06 14:40:58 CST）

主控确认普通 `GOWORK=off go test ./...` 已通过后，本负责人执行剩余唯一门禁和发布后只读核对。本轮没有修改业务代码、前端、生产配置或 SQLite。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | 通过（主控确认） | 当前源码普通全量 Go 测试通过；随后 race 对同一源码完成编译和测试 |
| `GOWORK=off go test -race ./...` | 通过 | 所有可测试 Go 包通过；`node/api` 66.113s、`node/service` 15.417s；未发现 race |
| `GOWORK=off go vet ./...` | 通过 | 退出码 0，无输出 |
| `git diff --check` | 通过 | 退出码 0 |
| `node --test test/contract/*.test.mjs` | 通过 | 7/7 通过，0 失败 |
| `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest docs/inventory/route-inventory-1panel.json` | 通过 | 759 条路由通过；191 条扩展路由兼容允许 |
| HTTP 无凭据冒烟 | 通过 | 8/8 真实边界请求通过 |
| HTTP/WS 边界执行器 | 通过 | 7/7 通过，包含本地终端 WS 未授权边界 |

本轮没有重新执行前端 type-check/build；前端没有发生变更。

## 本轮发布后只读核对（2026-09-06 14:40:58 CST）

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 当前生产二进制 | 观察 | SHA-256=`93cab71c9ee4bed820eb8aee8ea08f88b65fd8110191c28ffb2ef35bcb86ccb2`；mtime `2026-09-06 12:23:23 CST` |
| systemd | 通过 | `active`，`MainPID=2479227`，`NRestarts=0`；本次启动 `14:37:53 CST` |
| `/health` / `/ready` | 通过 | 分别 HTTP 200，返回 `status=ok` / `status=ready` |
| `znmp.sopvip.com` HTTP / HTTPS | 通过 | 分别 HTTP 200；HTTPS 使用本地 `--resolve` |
| OpenResty `nginx -t` | 通过 | syntax successful；有 `worker_connections` 超过 `nofile=1024` 警告 |
| 无效代理残留 | 通过 | 未发现 `proxy_pass http://;` 或 `proxy_pass http://127.0.0.1:9` |
| SQLite 只读完整性 | 通过 | `quick_check=ok`、`integrity_check=ok`、外键违规 0、63 张表 |
| SQLite 关键计数 | 观察 | `schema_migrations=13`、`operation_logs=2099`、`login_logs=175`、`runtime_task_logs=18145`、`runtime_records=6`、`runtime_tasks=31`、`websites=1`、`website_domains=1`、`website_ssls=1`、`website_waf_sites=1` |

只读核对未写数据库、未修改配置、未重启服务。

## 最新 SSL/ACME 定向门禁（2026-09-06 13:07）

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| `GOWORK=off go test ./node/service ./node/api -run 'Test(Website|SSL|ProbeOpenResty|ParseOpenResty|ContainerImage|OpenResty|DNS)' -count=1 -v` | 失败（编译阻塞） | `node/service/ssl.go:17:2: "time" imported and not used` |
| `syncIssuedCertificate` / `updateReadyMessage` | 已恢复 | 已由 `node/service/ssl_sync.go` 提供；此前相关未定义错误已消失 |
| `crypto/x509` | 已恢复 | `node/service/ssl.go` 已导入；此前 `undefined: x509` 已消失 |
| 全量 Go/race/vet | 未执行 | 定向测试未通过，按顺序不重复无效全量测试 |

本轮没有修改业务代码、前端或生产配置。当前阻塞仅需移除 `ssl.go` 中未使用的 `time` import（SSL 逻辑已拆至其他文件），修复后由本负责人重新执行定向测试，再收口全量门禁。

## 本轮最新 HTTPS/ACME 发布后核对（2026-09-06 13:01:27 CST）

本轮先执行受影响的 `node/service` 网站/SSL 定向测试，编译阶段失败，因此按顺序不再重复全量 Go/race/vet。随后仅对当前已发布制品、OpenResty、真实站点和 SQLite 做只读检查；未修改业务代码、前端、生产配置或数据库。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 网站/SSL 定向 Go 测试 | 失败（编译阻塞） | `node/service/ssl.go:338:2: declared and not used: item` |
| 最新源码修复状态 | 观察 | `website.go` 已有 `fmt`；`website_load_runtime.go` 已使用 `websiteLoadedString`；但 `ssl.go` 仍有未使用局部变量 |
| 最新源码与生产制品 | 阻塞 | `node/service/ssl.go` mtime `12:57:38`，生产二进制 mtime `12:23:23`；当前制品不是该未通过编译源码的可验证发布结果 |
| systemd | 通过 | `active`，`MainPID=2318545`，`NRestarts=0` |
| `/health` / `/ready` | 通过 | 分别 HTTP 200，返回 `status=ok` / `status=ready` |
| `znmp.sopvip.com` HTTP / HTTPS | 通过 | 分别 HTTP 200；HTTPS 使用本地 `--resolve` TLS/HTTP 200 |
| `docker exec workmesh-openresty-waf nginx -t` | 通过 | 配置语法成功；存在 `worker_connections` 超过 `nofile=1024` 警告 |
| 无效代理残留 | 通过 | 未发现 `proxy_pass http://;` 或 `proxy_pass http://127.0.0.1:9` |
| SQLite 只读 | 通过 | `quick_check=ok`、`integrity_check=ok`、外键违规 0、63 张表 |

生产二进制 SHA-256=`93cab71c9ee4bed820eb8aee8ea08f88b65fd8110191c28ffb2ef35bcb86ccb2`。生产健康和 HTTPS 结果仅证明已部署旧制品运行正常，不替代最新源码的编译验证。

## 本轮发布后只读核对（2026-09-06 12:55:21—12:56:14 CST）

本轮按照“源码未继续变化时不重复全量测试”的要求，仅检查当前生产进程、真实 SQLite、OpenResty 和站点配置；没有执行 Go/Node 全量测试，没有重启服务，没有写入 SQLite，也没有修改业务代码或前端。

| 检查 | 结果 | 当前证据 |
| --- | --- | --- |
| 源码修复状态 | 观察 | `website.go` 已导入 `fmt`；`website_load_runtime.go` 使用唯一的 `websiteLoadedString`，不再与备份模块的 `stringValue` 重名 |
| 当前发布制品 | 通过 | `/opt/workmesh-server/bin/workmesh-server` SHA-256=`93cab71c9ee4bed820eb8aee8ea08f88b65fd8110191c28ffb2ef35bcb86ccb2`，mtime `2026-09-06 12:23:23 CST` |
| systemd | 通过 | `workmesh-server.service=active`，`MainPID=2318545`，`NRestarts=0` |
| `/health` | 通过 | HTTP 200，`{"code":200,"data":{"status":"ok"}}` |
| `/ready` | 通过 | HTTP 200，`{"code":200,"data":{"status":"ready"}}` |
| `znmp.sopvip.com` HTTP | 通过 | Host 访问 HTTP 200，返回真实站点默认页 |
| `znmp.sopvip.com` HTTPS | 通过 | `--resolve znmp.sopvip.com:443:127.0.0.1` TLS/HTTP 200 |
| OpenResty `nginx -t` | 通过 | syntax successful；仍有 `worker_connections` 超过 `nofile=1024` 的警告 |
| 禁止的空代理残留 | 通过 | `/opt/workmesh-server/openresty-waf` 和 `/www/wwwroot` 未发现 `proxy_pass http://;` 或 `proxy_pass http://127.0.0.1:9` |
| SQLite 只读完整性 | 通过 | `quick_check=ok`、`integrity_check=ok`、外键检查 0 条违规、63 张表 |

残留扫描中出现的 `127.0.0.1:9101`、`9501`、`9912`、`9913` 均位于应用/vendor 文档或 addon 配置，不是 `proxy_pass` 空目标或旧 stream `127.0.0.1:9` 占位；未修改这些文件。

## 自动化门禁（最新网站改动当前工作区）

| 命令 | 结果 | 证据摘要 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | 通过 | 所有 Go 包通过；`node/api` 36.528s，`node/service` 3.690s |
| `GOWORK=off go test -race ./...` | 通过 | 所有可测试 Go 包通过；`node/api` 61.020s；未发现 race |
| `GOWORK=off go vet ./...` | 通过 | 退出码 0，无输出 |
| `git diff --check` | 通过 | 退出码 0 |
| `node --test test/contract/*.test.mjs` | 通过 | 7/7 通过，0 失败 |
| `node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest docs/inventory/route-inventory-1panel.json` | 通过 | 759 条路由通过；191 条扩展路由兼容允许 |
| `node test/contract/http-smoke.mjs --out /tmp/workmesh-http-smoke-integration-20260906.json` | 通过 | 8/8 通过；真实无凭据公开/鉴权边界请求 |
| `WORKMESH_SCOPE=boundary node test/contract/frontend-contract-executor.mjs --out /tmp/workmesh-contract-boundary-integration-20260906.json` | 通过 | 7/7 通过，包含 HTTP 与本地终端 WS 未授权边界 |

### 网站定向测试

| 命令 | 结果 | 证据摘要 |
| --- | --- | --- |
| `GOWORK=off go test ./node/service -run 'Test(Website|SSL|ProbeOpenResty|ParseOpenResty|ContainerImage)' -count=1 -v` | 已由全量测试覆盖 | `node/service` 全量测试通过；未使用模拟证书或模拟 ACME 响应 |
| 网站 API 定向测试 | 已由全量测试覆盖 | `node/api` 全量测试通过，网站路由编译和行为检查通过 |
| SSL / HTTP-01 定向测试 | 部分通过 | certbot 2.9.0 staging `renew --dry-run --no-random-sleep-on-renew` 成功；正式证书未被 dry-run 修改 |
| stream TCP/UDP / OpenResty 恢复定向测试 | 通过 | 启动后空 `site.conf` 恢复；历史 `127.0.0.1:9` 已替换为注释；`nginx -t` 成功 |

### 生产站点与 stream 只读探测

| 检查 | 结果 | 证据摘要 |
| --- | --- | --- |
| 已加载站点 HTTP Host 探测 | 通过 | `znmp.sopvip.com` 发布后 HTTP 200；其他未登记 legacy 目录不作为当前 SQLite 业务站点通过依据 |
| 旧 stream 占位配置清理 | 通过 | 已备份并将 `/www/wwwroot/types-stream.example/nginx/stream.conf` 中 `proxy_pass 127.0.0.1:9` 替换为注释，未删除目录 |
| `znmp.sopvip.com` 站点配置恢复 | 通过 | 重启后空 `site.conf` 自动恢复为真实 server/root/ACME challenge 配置 |
| OpenResty 全量配置残留扫描 | 通过（运行配置） | `openresty -T` 未发现 `proxy_pass http://;`；历史备份目录仍有无效样例配置，未被当前 include 加载 |

以上网站基础检查均使用真实文件、SQLite、OpenResty 和 certbot 结果；集成测试负责人未修改业务代码。

## 生产只读健康检查

| 检查 | 结果 | 证据摘要 |
| --- | --- | --- |
| `systemctl is-active workmesh-server.service` | 通过 | `active` |
| 当前二进制 SHA-256 | 通过 | `93cab71c9ee4bed820eb8aee8ea08f88b65fd8110191c28ffb2ef35bcb86ccb2`；已由主控重新构建并原子部署 |
| `systemctl show ... -p NRestarts` | 通过 | `NRestarts=0`；主 PID `2318545`（本轮 12:51 启动） |
| `GET http://127.0.0.1:9999/health` | 通过 | HTTP 200，`{"code":200,"data":{"status":"ok"}}` |
| `GET http://127.0.0.1:9999/ready` | 通过 | HTTP 200，`{"code":200,"data":{"status":"ready"}}` |
| `docker exec workmesh-openresty-waf nginx -t` | 通过 | syntax successful；存在 `worker_connections` 资源限制警告，但不是配置语法错误 |
| `Host: znmp.sopvip.com` HTTP 入口 | 通过 | HTTP 200，返回真实站点创建成功页 |
| `https://znmp.sopvip.com` 本地/公网解析入口 | 通过 | TLS 1.3 握手成功，证书 CN=`znmp.sopvip.com`，HTTP 200 |

## SQLite 只读完整性

使用 Python `sqlite3` 只读 URI 连接 `/opt/workmesh-server/data/workmesh.db`，未写入数据库：

| 检查 | 结果 |
| --- | --- |
| `PRAGMA quick_check` | `ok` |
| `PRAGMA integrity_check` | `ok` |
| 外键检查 | `0` 条违规 |
| 用户表数量 | `63` |
| `operation_logs` | `2099` 条 |
| `login_logs` | `175` 条 |
| `task_logs` | 不存在（任务日志使用 `runtime_task_logs` 等真实表） |
| `runtime_task_logs` | `18145` 条 |
| `runtime_records` | `6` 条 |
| `runtime_tasks` | `31` 条 |
| `app_installs` | `3` 条 |
| `websites` / `website_domains` / `website_settings` | `1` / `1` / `14` 条 |
| `gateway_binding` | `1` 条 |

## 2026-09-08 并行整改批次门禁

本批次由主控在全部专项智能体停止写入后统一执行，未修改 `/www/apps/1Panel`，未执行生产重启、ACME 签发、真实 Docker 生命周期或跨节点写操作。

| 命令 | 结果 | 证据摘要 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | 通过 | 所有 Go 包通过 |
| `GOWORK=off go test -race ./...` | 通过 | 所有 Go 包 race 通过 |
| `GOWORK=off go vet ./...` | 通过 | 无 vet 报告 |
| `node --test test/contract/*.test.mjs` | 通过 | 7/7 通过 |
| `node test/contract/route-scan.mjs check test/contract/routes.json` | 通过 | 759 条路由；193 条扩展路由兼容允许 |
| `node test/contract/security-contract-scan.mjs --strict` | 通过 | fallback 735 条被明确记录；WS/SSE 白名单一致；link status secret 模式已签名 |
| `git diff --check` | 通过 | 无空白错误 |
| `GOWORK=off go run ./test/quality -root . -out .tmp/quality-current.json` | 未通过规范门禁 | 376 个 Go 文件、3056 个函数、848 项违规；中文文档 773、超长函数 75；不影响编译门禁 |

### 2026-09-08 后续轮换批次复核

本轮新增 PHP 模板、Toolbox 设备密码和 Fail2Ban Handler 拆分，以及按前端菜单生成的路由缺口矩阵和 v2 写请求参数审计。源码冻结后主控再次执行：

| 命令 | 结果 | 证据摘要 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | 通过 | 所有 Go 包通过，包含新增 PHP/Toolbox 代码 |
| `GOWORK=off go test -race ./...` | 通过 | 所有 Go 包 race 通过，包含新增 PHP/Toolbox 代码 |
| `GOWORK=off go vet ./...` | 通过 | 无 vet 报告 |
| `node --test test/contract/*.test.mjs` | 通过 | 7/7 通过 |
| `node test/contract/route-scan.mjs check test/contract/routes.json` | 通过 | 759 条路由；193 条扩展路由兼容允许 |
| `node test/contract/security-contract-scan.mjs --strict` | 通过 | fallback 735；link status secret 模式已签名 |
| `git diff --check` | 通过 | 无空白错误 |
| 质量扫描 | 未通过规范门禁 | 376 个 Go 文件、3065 个函数、843 项违规；中文文档 773、超长函数 70 |

### V2-PARAM-APP 与路由明细批次复核

应用写请求和 SQLite-only 状态专项完成后再次冻结验证：

| 命令 | 结果 | 证据摘要 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | 通过 | 应用拆分、严格请求体和 SQLite 状态变更全部通过 |
| `GOWORK=off go test -race ./...` | 通过 | 全部 Go 包 race 通过 |
| `GOWORK=off go vet ./...` | 通过 | 无 vet 报告 |
| `node --test test/contract/*.test.mjs` | 通过 | 7/7 通过 |
| `node test/contract/route-scan.mjs check test/contract/routes.json` | 通过 | 759 条路由 |
| `node test/contract/security-contract-scan.mjs --strict` | 通过 | fallback 735；link status 已签名 |
| `node test/contract/route-gap-detail.mjs --write --summary` | 通过 | 逐条生成网站/运行时/数据库/容器/主机等缺口明细 |
| `git diff --check` | 通过 | 无空白错误 |
| 质量扫描 | 未通过规范门禁 | 376 个 Go 文件、3075 个函数、842 项违规；中文文档 773、超长函数 69 |

### V2-PARAM-SETTINGS-01 通用 JSON 边界复核

`requestMap` 与 `decodeJSON` 已接入 `decodeSingleJSON`，拒绝同一请求中的第二个 JSON 值；未知字段仍保留兼容。主控随后完成唯一全量门禁：

| 命令 | 结果 | 证据摘要 |
| --- | --- | --- |
| `GOWORK=off go test ./...` | 通过 | 所有 Go 包通过 |
| `GOWORK=off go test -race ./...` | 通过 | 所有 Go 包 race 通过 |
| `GOWORK=off go vet ./...` | 通过 | 无 vet 报告 |
| `node --test test/contract/*.test.mjs` | 通过 | 7/7 通过 |
| `node test/contract/route-scan.mjs check test/contract/routes.json` | 通过 | 759 条路由 |
| `node test/contract/security-contract-scan.mjs --strict` | 通过 | fallback 735；link status 已签名 |
| `node test/contract/route-gap-detail.mjs --write --summary` | 通过 | 逐路由缺口矩阵刷新成功 |
| `git diff --check` | 通过 | 无空白错误 |
| 质量扫描 | 未通过规范门禁 | 378 个 Go 文件、3081 个函数、842 项违规；中文文档 773、超长函数 69 |

本批次后续补充：`node/api/json_request.go` 将 `requestMap` 与 `decodeJSON` 统一到 `decodeSingleJSON`，新增尾随 JSON 回归测试；定向 `Test(Decode|App|Website|Container|Runtime|HostMonitorSettings|Compose)` 通过。字段未知值仍按 1Panel 兼容要求保留，后续再按路由逐项收紧。

本批次专项交付包括：运行时方法拆分、Gateway 本地 SQLite/TCP 与 `link/status` HMAC/nonce/epoch 校验、SQLite 启动迁移幂等证据、安全契约扫描。生产系统只读健康状态未因本批次改变；登录后业务闭环仍按下表保持 `not-run`/`blocked`。

## 尚未宣称完成的验收

静态路由契约和无凭据边界测试不等于登录后的业务闭环。本轮没有有效管理员会话、次节点凭据或 Docker 外部资源，因此以下保持 `not-run`/`blocked`，没有使用模拟 JSON、固定成功响应或空数组：

- 前端清单中的 385 条 HTTP 业务接口和 4 条 WS 业务消息序列；
- Go、Node、Python、Java、.NET、PHP 六类运行环境完整生命周期及 PHP FPM/扩展验证；
- 六类网站、网站设置、主/次节点 Gateway/心跳/跨节点任务；
- Let's Encrypt HTTP-01 真实签发与续期。

本轮额外检查使用 `--resolve znmp.sopvip.com:443:127.0.0.1` 及公网域名访问，TLS 握手和 HTTP 200 均通过。`nginx -t` 同时报告 `worker_connections` 超过当前 `nofile=1024` 的警告，未报告语法错误。

## 阻塞与下一步

1. 本轮发布后只读核对已完成；源码未继续变化时不重复全量 Go/Node 门禁。若主控再次修改后端源码，先由本负责人重新执行网站定向测试，再执行完整 Go、race、vet。
2. 当前生产制品已运行并通过健康、HTTPS、OpenResty 和 SQLite 只读核对；任何后续部署仍需保留二进制、配置和 SQLite 备份并由本负责人复核。
3. `aliases.example`、三类运行时/部署代理站点的真实上游需要由网站环境负责人启动或通过正式 API 创建后复测；不能以 HTTP 200 或模拟 JSON 代替。
4. 前端本轮未参与测试，且无须重新编译；只有前端源码发生变更时才执行 type-check/build。
5. 当前没有有效管理员会话、次节点凭据或外部 ACME 测试资源，登录后的业务接口、WS 业务消息、六类运行环境、网站设置、主次节点和 Let's Encrypt HTTP-01 仍是 `not-run`/`blocked`，禁止以模拟数据替代。

## 发布后基础设施补充核对（2026-09-06）

主控在完整门禁通过后更新了 OpenResty 容器资源限制并重建容器；这不是源码测试结果，也未修改 `apps/1Panel`。当前只读核对结果：容器 `ulimit -n=65536`，`nginx -t` 成功且不再报告 `worker_connections` 超限，`znmp.sopvip.com` HTTP/HTTPS 均返回 200。该配置变更已在发布备份中保存。
