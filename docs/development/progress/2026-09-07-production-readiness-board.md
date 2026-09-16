<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 正式使用并行执行看板（2026-09-07）

## 目标与边界

本看板以“网站可用、运行环境可用、可回滚发布”为上线顺序。`apps/1Panel` 只读参考，`apps/workmesh-node` 不作为实现来源；所有运行态数据必须来自 SQLite，禁止 JSON fixture、固定成功或固定空列表代替真实结果。开发智能体只修改后端代码或文档，唯一集成负责人统一执行全量测试；生产写操作、真实 Docker/ACME/Gateway 联调必须在获得对应人工确认后执行。

## 并行席位

| 席位 | 当前任务 | 输入 | 交付物 | 验收证据 | 禁止操作 |
| --- | --- | --- | --- | --- | --- |
| 主控 `/root` | 集成、冲突处理、门禁、发布审批 | 各队列报告、冻结哈希 | 集成记录、发布/回滚记录 | 测试前后哈希一致；全量门禁结果 | 未获确认不得重启生产或改外部资源 |
| 网站/OpenResty 智能体 | Q1 网站类型、域名、配置文件和 HTTPS 只读盘点 | `node/service/website*`、真实 nginx 配置、1Panel 前端契约 | 网站设置到 API/SQLite/配置文件映射；待测域名清单 | 每项有路由、配置路径、访问命令和状态 | 不改 `apps/1Panel`；不执行签发、删除或 reload |
| 运行时队列 | Q2 Go/Node/Python/Java/.NET/PHP 生命周期准备 | `node/api/runtime*`、Compose 模板、真实镜像清单 | [`2026-09-07-runtime-production-readiness.md`](2026-09-07-runtime-production-readiness.md) | 六类运行时及 PHP 专项请求、字段、命令和阻塞证据已整理 | 不启动或删除生产容器；不跑全量门禁 |
| 升级迁移队列（已完成只读盘点） | Q3 二进制升级、SQLite 迁移、回滚准备 | `internal/storage/migrations.go`、发布脚本、备份目录 | [升级迁移就绪报告](2026-09-07-upgrade-migration-readiness.md)；含迁移前检查、备份、升级、回滚命令和校验项 | 当前库 `quick_check`、`integrity_check`、外键检查通过；隔离双启动和生产替换仍待授权 | 不替换生产二进制；不写真实数据库 |

平台当前最多 4 个席位；子智能体因 `429` 退出时不重复创建相同任务。席位释放后按 Q2、Q3 顺序补位，仍由主控统一集成。

## 当前执行批次（2026-09-07）

本批次将剩余后端工作拆成互不修改同一业务文件的专项任务。专项智能体只能运行本领域定向测试；主控在所有专项交付完成后才执行一次完整门禁，避免重复占用编译和测试资源。

| 任务 ID | 负责人 | 代码边界 | 明确交付 | 完成判定 | 不在本任务内 |
| --- | --- | --- | --- | --- | --- |
| LOG-02 | 日志 SQL 分页队列 | `node/api/logs_queries.go` 与其定向测试 | 操作、登录、任务日志改为 SQLite 条件查询和有界分页，保留 v2 响应字段 | 大表查询不全量读入内存；过滤、排序、总数、分页和空结果均有测试 | 系统日志和 SSH 日志文件读取；生产日志清理 |
| REL-01 | 发布演练队列 | `cmd/workmesh-server`、`internal/storage` 与独立演练文档 | 临时目录执行候选制品验签、SQLite 两次启动迁移、失败回滚证据 | 不触及 `/opt`、systemd 或真实数据库；第二次迁移为 `noop` | 生产二进制替换、服务重启、数据库备份写入 |
| WS-01 | 终端流队列 | `node/api/terminal_stream.go`、`node/api/websocket_stream.go` 与定向测试 | 本地 TCP WebSocket 握手、鉴权、命令输出、关闭码和清理的可重复测试 | 不使用模拟成功；连接、子进程和临时密钥均有释放证据 | 生产终端执行、Docker 容器、真实 SSH 主机 |

主控 INT-00 同步审阅 P0 网站与 P1 运行时的现有真实实现，收集可执行的黑盒验收输入。待管理员会话、CSRF、隔离域名、可清理 Docker 资源和维护窗口明确后，由主控顺序执行真实写操作，保证发生失败时能够立即回滚。

## 实时并行席位与原子任务（2026-09-07）

当前 4 个席位全部工作中。代码智能体只运行本领域定向测试；主控等待源码冻结后统一执行一次全量门禁。任何任务完成后，原席位立即领取表中的下一项，不重新扫描已经确认的范围。

| 席位 | 当前原子任务 | 文件边界 | 定向验证 | 完成后立即轮换 |
| --- | --- | --- | --- | --- |
| 主控 `/root` | INT-01 汇总任务边界、检查共享工作树冲突、维护正式使用阻塞项 | 本看板、`STATUS.md`、最终集成记录 | `git diff --check`；专项冻结后才运行全量 Go/race/vet/契约门禁 | INT-02 生成源码集合哈希并执行最终门禁；INT-03 汇总 HTTP/WS 黑盒未覆盖项 |
| `quality_runtime` | QUALITY-03I 继续处理 `runtime_toolbox_*` 剩余质量方法，保持命令、SQLite 和 v2 响应不变 | 仅 `node/api/runtime_toolbox_*`，每次只拆一个注册方法 | `GOWORK=off go test ./node/api -run 'Test(Runtime|Toolbox)' -count=1` | QUALITY-03J 处理下一组 PHP/Toolbox 文件，直到无超长注册方法 |
| `audit_policy` | SEC-03 已交付逐路由 501/partial/not-run 明细；下一轮将网站 P0 路由转成可执行验收样例 | 仅审计文档、测试和 `test/contract` 清单，不改业务实现 | `node test/contract/route-gap-detail.mjs --write --summary` | WEB-02B 将网站域缺口拆成可执行成功/失败/回滚案例 |
| `release_checklist` | V2-PARAM-APP 与 V2-PARAM-SETTINGS-01 已完成：应用、设置、日志、备份、主机/计划任务共用单对象 JSON 解析 | `node/api/apps.go`、`functional_domains.go`、`host_container_cron.go`、`json_request.go` 及定向测试；不修改生产数据 | 定向 `Test(Decode|App|Website|Container|Runtime|HostMonitorSettings|Compose)` 已通过 | V2-PARAM-SETTINGS-02 继续补设置字段白名单和类型/范围测试 |

### 主控集成检查点

1. **边界检查**：专项交付时先读取各自改动文件，确认未修改 `apps/1Panel`、前端、生产配置或其他席位文件。
2. **行为检查**：逐项核对 API 字段、错误 envelope、SQLite 真实持久化、外部命令超时及资源释放，不接受固定成功或固定空列表。
3. **专项检查**：只复核智能体报告的定向命令与失败输出；发现失败时退回原席位修复，不由多个席位同时修改同一文件。
4. **冻结检查**：记录源码集合哈希，确认没有仍在写入的智能体，再执行一次 Go 全量、race、vet、Node 契约、759 路由和空白检查。
5. **生产检查**：管理员会话、CSRF、隔离域名、Docker/ACME/Gateway 资源齐备且人工确认后，才执行真实写操作；否则保持 `blocked/not-run`。

### 后续轮换队列

| 顺序 | 任务 ID | 工作内容 | 完成证据 |
| --- | --- | --- | --- |
| 1 | WEB-02 | 静态、反代、运行时、子站点、TCP/UDP 的 API 创建到删除回滚场景；补齐可在隔离环境执行的测试 | 每种网站类型至少覆盖成功、参数错误、外部命令失败和 SQLite 回滚 |
| 2 | GATEWAY-02B | Gateway v2 HTTP handler 的认证、CSRF、签名、重放、旧 epoch 和错误 envelope | HTTP 定向测试与路由/方法/状态码矩阵 |
| 3 | QUALITY-03B | 运行时任务的锁、取消、超时、stderr 上限、SQLite 状态迁移注释和小方法拆分 | 质量扫描违规下降，运行时定向测试不变 |
| 4 | BLACKBOX-PREP | 从现有前端调用提取 P0/P1 请求样例，建立不含凭据的执行模板 | 每个请求含方法、路径、必要字段、预期响应、SQLite/配置/域名校验点 |
| 5 | INT-02 | 所有代码席位冻结后执行唯一一次全量门禁 | Go、race、vet、Node 契约、路由扫描和 `git diff --check` 全部有时间戳结果 |

## 阶段一：网站/OpenResty 正式使用

### Q1-A 基础入口

- [x] systemd 服务处于 `active`。
- [x] `/health` 返回 HTTP 200。
- [x] `/ready` 返回 HTTP 200。
- [x] `docker exec workmesh-openresty-waf nginx -t` 通过。
- [x] 现有 `znmp.sopvip.com` HTTP/HTTPS 入口可访问。
- [ ] 新建隔离测试前置域名并验证主站点/次站点角色。需要人工确认外部 DNS/站点写操作。

### Q1-B 网站类型

按每个独立 `*.cs.sopvip.com` 前置域名逐项执行：

1. 静态站点：创建、默认文档、目录权限、HTTP 访问、删除后配置清理。
2. 反向代理：upstream、请求头、WebSocket、超时、错误页和实际访问。
3. 运行环境站点：绑定运行时、端口、重启后访问和删除隔离。
4. 子站点：父子域名、目录隔离、配置生成和访问。
5. TCP/UDP：stream 配置、监听端口、连通性、删除清理。
6. 一键部署：模板下载、SQLite 安装记录、任务日志、失败回滚和重建恢复。

每项必须记录：请求方法/路径、脱敏参数摘要、状态码、响应 envelope、SQLite 变化、生成配置文件、真实域名访问、失败回滚结果。

### Q1-C HTTPS/WAF 设置

- [ ] HTTP-01 为具体前置域名签发 Let's Encrypt 证书；不使用通配符证书假设。
- [ ] 证书安装后 443 实际访问，证书记录与站点 SQLite 外键一致。
- [ ] 续期 hook 生成配置、校验 `nginx -t`、reload；失败保留旧证书。
- [ ] 验证 CORS、伪静态、重定向、真实 IP、防盗链、限流、密码访问和 WAF 规则。
- [ ] 验证访问日志、错误日志和操作日志均写入真实 SQLite/日志文件，敏感字段脱敏。

## 阶段二：运行环境正式使用

每种运行时均执行同一生命周期：创建 -> 详情 -> 启动 -> 停止 -> 重启 -> 日志/任务 -> 删除 -> 重建 -> SQLite 恢复 -> 网站访问。

| 运行时 | 额外验收 |
| --- | --- |
| Go | 包内 Compose、端口、脚本、容器重建 |
| Node.js | scripts、模块扫描、npm/yarn/pnpm 操作 |
| Python | 依赖目录、启动命令、日志 |
| Java | JDK/JAR 参数、健康检查、日志 |
| .NET | 运行时版本、端口、发布目录 |
| PHP | PHP-FPM、FastCGI `/status`、扩展安装/卸载、php.ini、Supervisor、慢日志和 PHP 站点 |

任何一步失败必须保存真实 stderr、任务日志和 SQLite 状态，并执行删除或回滚；不能改写为成功。

## 阶段三：升级、迁移与发布

1. 记录当前二进制、配置、SQLite 主库及 WAL/SHM 的 SHA-256。
2. 复制到独立备份目录并执行 SQLite 只读 `quick_check`、`integrity_check`、外键检查。
3. 在隔离库运行全部 migration 两次，确认第二次为幂等 no-op，新字段存在且旧数据保留。
4. 构建后端二进制并执行原子替换；前端无改动时不重新编译前端。
5. 重启单进程服务，验证 `/health`、`/ready`、OpenResty、SQLite 和关键网站。
6. 注入可控失败或使用备份制品演练回滚，确认旧二进制和数据库均可恢复。
7. 由唯一集成负责人重跑 `go test ./...`、race、vet、Node 契约和 759 路由扫描。

## 当前真实状态

### 并行状态快照（2026-09-08）

| 智能体 | 状态 | 当前/下一任务 | 代码边界 | 验收方式 |
| --- | --- | --- | --- | --- |
| `/root` | 工作中 | 集成审阅、冲突检查、最终门禁 | 仅文档、集成验证；不与专项队列改同一业务文件 | 专项冻结后唯一执行全量门禁 |
| `audit_policy` | 已交付，待轮换 | SEC-06：把网站 501/partial 结果按真实 ServeMux 路由复核 | `test/contract`、审计文档、网站路由测试 | `go test ./node/api -run 'TestWebsiteP0StaticDomainHTTPSAndOpenRestyRoutes'` |
| `website_p0` | 已交付，待轮换 | WEB-03：补网站配置失败回滚和 SQLite/OpenResty 一致性测试 | `node/service/website*`、网站定向测试 | 网站服务层回滚定向用例 |
| `quality_runtime` | 已交付，待轮换 | QUALITY-03L：补 `runtime_php_supervisor.go` 中文边界注释并检查剩余长方法 | 仅 `node/api/runtime_*` 及定向测试 | `go test ./node/api -run 'Test(Runtime|PHP)'` |
| `release_checklist` | 已交付，待轮换 | V2-PARAM-SETTINGS-03：设置路由字段类型/范围契约 | 仅设置参数测试和审计文档，不改业务实现 | 设置相关定向测试和契约扫描 |

轮换规则：专项智能体只运行本领域定向测试，不访问生产，不修改 `/www/apps/1Panel`、前端和生产配置；席位释放后立即领取下一项。所有专项结束后由主控统一运行 Go、race、vet、Node 契约、路由扫描和 `git diff --check`。

### 2026-09-08 细粒度执行批次

当前批次按文件所有权分成四条流水线。一个智能体只能领取一条流水线中的一个原子项；完成后提交“改动文件、定向命令、结果、未覆盖项”，再领取下一项。

| 原子项 | 负责人 | 唯一修改边界 | 交付判定 | 依赖/阻塞 |
| --- | --- | --- | --- | --- |
| WEB-ROLLBACK-01 | 网站队列 | `node/service/website_lifecycle*`、对应测试 | 外部命令失败时 SQLite、目录、OpenResty 配置全部回滚 | 不接触生产 OpenResty |
| SETTINGS-CONTRACT-01 | 参数队列 | `node/api/functional_settings*`、设置契约测试 | key/value、批量更新、memo、SSL、upgrade 的类型/必填/错误码矩阵 | 不改变未知字段兼容 |
| GATEWAY-FENCE-01 | Gateway 队列 | `runtime/link/*` 定向测试 | 注册、心跳、nonce 重放、旧 epoch、断线重连均有真实 SQLite/TCP 证据 | 无生产节点凭据 |
| TERMINAL-WS-01 | WS 队列 | `node/api/terminal_*`、`websocket_stream.go` 测试 | 握手、鉴权、Origin、命令输出、关闭帧、子进程释放 | 只使用本地 TCP |
| LOG-PAGE-01 | 日志队列 | `node/api/logs_queries*`、SQL 测试 | 过滤、排序、总数、有界分页和空结果来自 SQLite | 不清理生产日志 |
| RELEASE-READONLY-01 | 发布队列 | `cmd/workmesh-server`、`internal/storage` 文档/演练 | 制品哈希、隔离迁移二次 noop、失败回滚命令可重放 | 不替换生产二进制 |

执行顺序：WEB/SETTINGS/GATEWAY/TERMINAL/LOG/RELEASE 可并行；所有专项冻结后才执行一次集成门禁。集成门禁固定为 `GOWORK=off go test ./...`、`GOWORK=off go test -race ./...`、`GOWORK=off go vet ./...`、Node 契约、路由扫描、安全扫描和 `git diff --check`。前端没有变更时不运行前端构建。

- 自动化门禁：Go 全量/race/vet、Node 7/7、759 条路由、OpenResty 和健康检查已通过。
- 网站准备：[`2026-09-07-website-production-readiness.md`](2026-09-07-website-production-readiness.md) 已完成只读盘点；现有静态站点可访问，反代/运行时/PHP/stream 测试资源未监听。
- 运行时准备：[`2026-09-07-runtime-production-readiness.md`](2026-09-07-runtime-production-readiness.md) 已完成六类运行时和 PHP 专项验收矩阵；真实容器生命周期待隔离资源。
- 代码规范：当前扫描为 372 个 Go 文件、2986 个函数、854 项违规（中文注释 768、超长函数 86；超长文件 0），仍未通过规范门禁。容器请求分派和应用目录注释已补齐，剩余超长函数和注释继续并行处理。
- 真实业务：管理员登录后 HTTP/WS、六类运行时、网站全类型、高级设置、主次 Gateway、ACME 签发/续期、升级回滚尚未全部执行，保持 `blocked/not-run`。
- 当前生产服务只读状态：`workmesh-server` active，`/health=200`，`/ready=200`，OpenResty `nginx -t` successful。

## 下一次执行入口

取得管理员会话、隔离 Docker 镜像、可解析的 `*.cs.sopvip.com` 前置域名和 ACME 测试许可后，先执行 Q1-B 静态站点与反向代理，再执行 Q1-C HTTPS，最后进入 Q2 六类运行时。所有真实请求由唯一集成负责人记录到 [`integration-test-latest.md`](integration-test-latest.md)。

## P3 后续细分队列（席位释放后）

当前三个专项已交付；席位释放后按以下边界重新分配，仍由主控统一验收：

| 队列 | 独立边界 | 交付物 | 前置条件 |
| --- | --- | --- | --- |
| AUDIT-03 | 日志策略与定向测试 | 敏感字段脱敏、保留/清理策略、七类日志矩阵 | 已完成；生产清理仍需维护窗口 |
| REL-02 | 发布文档与脚本只读审阅 | 正式发布检查单、失败回滚步骤、制品/WAL/SHM 校验模板 | 已完成；生产演练仍需维护窗口 |
| GATEWAY-02 | `runtime/link` 测试与契约 | 注册、心跳、epoch/fencing、任务透传和旧 epoch 拒绝测试 | 次节点凭据与隔离网络 |
| BLACKBOX-01（主控） | 真实 `/api/v2` HTTP/WS | 登录后业务、网站和运行时生命周期证据 | 管理员会话、CSRF、域名及 Docker 资源 |

未取得外部凭据前不执行生产写操作；“计划中”不计为正式可用。

REL-02 清单已落地：[`2026-09-07-release-rollback-checklist.md`](2026-09-07-release-rollback-checklist.md)。

Gateway 并行任务已拆分为本地状态机、TCP 协议、v2 handler、任务透传及真实联调六个子任务：[`2026-09-07-gateway-parallel-work.md`](2026-09-07-gateway-parallel-work.md)。

## 代码质量并行队列

质量扫描当前为 370 个 Go 文件、2985 个函数、872 项违规。为避免互相覆盖，后续席位按文件边界拆分：

| 队列 | 文件边界 | 目标 | 验收 |
| --- | --- | --- | --- |
| QUALITY-01 | `node/api/functional_logs.go`、`node/api/terminal_stream.go` | 将超长文件拆为日志/任务、终端协议/命令等职责文件，保持路由不变 | 已完成；文件均不超过 500 行，定向日志/WS 测试通过 |
| QUALITY-02 | `node/api/website_*`、`node/service/website_*` | 拆分超过 80 行的方法，提取纯校验/持久化辅助逻辑 | 网站定向测试和契约扫描 |
| QUALITY-03 | `node/api/runtime_*`、`control/api/*` | 补充缺失的准确中文注释，优先导出符号和并发/迁移边界 | 质量扫描违规下降，行为测试不变 |
| QUALITY-04（主控） | 全仓库 | 冲突审阅、一次全量 Go/race/vet 和文档状态更新 | 测试前后源码集合哈希一致 |

QUALITY 队列只做结构和注释，不改变 API、SQLite schema、OpenResty 路径或业务语义；每个队列完成后先定向测试，主控统一做最终门禁。
