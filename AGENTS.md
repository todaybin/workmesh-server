# WorkMesh Server Agent 规则

本文件适用于 `workmesh-server` 独立 Git 仓库及其全部子目录。执行任务时先遵守本文件；若子目录后续增加更具体的 `AGENTS.md`，再叠加对应规则。

## 仓库边界

- 本项目是独立仓库 `github.com/todaybin/workmesh-server`，不是上层 WorkMesh pnpm workspace、Go workspace 或 Gateway addon。
- Go module、Web 依赖、锁文件、构建产物和发布流程均在本仓库内管理；禁止从代码或发布脚本依赖上层仓库的私有源码、绝对路径或维护者本机环境。
- 与 Gateway、其他节点及外部服务只通过已声明的 HTTP、节点链路、环境变量和部署契约交互，不跨仓库导入实现。
- 不删除、reset、覆盖用户已有改动；`data/`、`.workmesh-data/`、`.cache/`、`.tmp/`、`web/dist/` 和诊断日志不进入提交。

## 架构基线

- `cmd/workmesh-server` 是唯一进程入口；control 与 node 是同一进程中的业务分区，共享配置、认证上下文、SQLite、调度器和生命周期，不拆回两个服务。
- `control/` 负责登录、Session、MFA、Passkey、RBAC、设置、审计和多节点管理。
- `node/` 负责主机、容器、应用、网站、数据库、文件、计划任务、备份、终端、AI、沙箱和部署等执行能力。
- `runtime/` 只承载共享基础设施、角色协议、节点链路、Gateway adapter、日志、缓存和调度；不得反向承载具体业务页面或领域逻辑。
- `internal/storage/` 是统一 SQLite 存储和迁移入口；领域代码通过 service/repository 边界访问数据，不在 HTTP handler 中散落 SQL、文件写入或进程控制。
- `web/` 是同源 Vue 3 + Vite 管理端，生产构建输出为 `web/dist`，由同一 Go 进程托管；浏览器只访问当前 origin 下的 `/api/v2`。
- 启动、就绪、迁移、更新和回滚流程以 `docs/architecture/unified-server.md`、`docs/architecture/runtime.md` 为准，公开路由兼容以 `docs/migration/route-contract-policy.md` 和 `test/contract/routes.json` 为准。

## API、数据与安全

- HTTP API 使用 `/api/v2`。成功 envelope 的 `code` 必须是数字 `200`；错误使用 `code: "ERR"`，业务错误码放在 `details.errCode`。
- `/health` 只反映进程存活，不访问 SQLite、Gateway 或其他外部依赖；`/ready` 使用短超时检查迁移、存储和关键依赖，未就绪返回 `503`。
- 新增或修改公开路由时，同步更新 handler、鉴权、参数校验、API 文档、契约清单和测试。不得用 `MIGRATION_PENDING`、固定空列表、假成功或 compatibility handler 冒充完成。
- 列表接口必须分页或设置明确上限；排序、路径、命令、资源标识和远端目标必须使用服务端白名单，禁止拼接用户输入生成 SQL、Shell 或文件路径。
- 写接口必须校验认证、资源归属、参数范围和必要的幂等/版本冲突；敏感凭据保存后脱敏返回，禁止写入源码、配置示例、日志或前端状态。
- 文件操作必须限制在授权根目录内，防止路径穿越，设置上传/输出上限，并优先采用同目录临时文件加原子 rename。
- 外部命令不得经未审查的 Shell 拼接，必须使用参数数组、允许列表、上下文取消、超时和输出上限；网络请求必须设置连接及整体超时。
- 长耗时操作使用可观测的异步任务；未启用的 AI、MCP、扫描、WAF、沙箱和大任务不得创建常驻 worker。
- 主次节点角色变更必须保留 `role_epoch`、fencing、签名、时间戳和 nonce 校验；浏览器不得获得或直连节点地址，节点选择只由服务端 relay 解析。
- 数据库结构只能通过幂等、带版本的 migration 演进。迁移不得覆盖或删除旧数据源，失败时必须阻止就绪并保留可回滚状态。

## Go 实现规则

- Go 版本和依赖以 `go.mod`、`go.sum` 为准；在本独立 module 中运行命令时设置 `GOWORK=off`。
- 新增 Go 源文件使用 `SPDX-License-Identifier: GPL-3.0-only` 和现有版权头；新增或修改的代码注释使用中文，导出符号按 lint 要求补充注释。
- handler 只负责协议适配、校验和响应；业务规则放入 service，持久化与外部能力通过明确接口注入，避免全局可变状态。
- 错误必须保留业务上下文并可观测，不静默吞错；日志不得记录密码、Token、私钥、Session、完整凭据或未经脱敏的请求体。
- 平台差异使用带 build tag 或平台后缀的文件隔离；Linux 特有行为不得破坏 Windows 上的编译与单元测试。
- 新查询必须对应明确访问模式和索引；避免 N+1、无界读取、重复扫描以及在请求路径中启动无上限 goroutine。

## 性能、注释与可维护性

- 性能要求必须结合请求延迟、吞吐、内存、CPU、磁盘和数据库锁竞争评估；不得只凭主观判断宣称“更快”。涉及热点路径、批量任务、列表接口、文件传输或并发控制的改动，必须说明复杂度、资源上限和可能的退化场景。
- 请求路径中的数据库查询、网络调用、文件读写和外部命令必须设置合理的超时、取消和结果上限；大结果集使用分页、流式处理或分块处理，禁止一次性加载不受控的数据。
- 并发必须有明确的生命周期、上限和背压策略。新增 goroutine、timer、channel、连接、文件句柄或缓存条目时，必须说明释放路径，并确保错误、超时和服务关闭时不会泄漏。
- 优先通过批量查询、合适索引、缓存复用和减少重复序列化降低开销；缓存必须定义容量、过期、失效和一致性策略，不得用无限增长的全局 map 代替设计。
- 性能敏感改动应补充基准测试、可重复的 profiling 或指标对比；测试数据和结论必须贴近实际访问模式。不得为了追求微小收益牺牲正确性、安全性、可读性或可诊断性。
- 代码必须写清楚必要的中文注释：解释业务不变量、并发/锁顺序、超时与重试原因、缓存失效、协议兼容、平台差异以及不直观的性能取舍。注释应说明“为什么”，不要重复代码表面行为；复杂算法和非显然边界必须在实现附近注明。
- 导出类型、函数、方法、常量和重要配置项必须有准确注释；注释与实现、错误码、API 文档同步更新，禁止保留过期、含糊或与代码相反的说明。
- 保持单一职责和清晰的依赖方向：handler、service、repository、runtime 和前端组件不得越层调用；避免重复逻辑、隐式全局状态、过深嵌套和超大函数。抽象只有在复用边界稳定且能减少复杂度时才引入。
- 修改共享接口、数据结构或公共工具时，必须检查所有调用方、错误处理和兼容行为；优先使用小而可回滚的改动。删除或重命名符号前先完成引用搜索，并同步更新测试和文档。
- 可维护性以可测试、可观测、可审查为验收标准：新增行为应有覆盖关键成功、失败、超时、取消和边界条件的测试；日志、指标和错误上下文应足以定位性能退化，但不得泄露敏感信息。

## Web 实现规则

- 沿用 Vue 3、TypeScript、Vite、Pinia、Vue Router、Element Plus 和现有组件体系，不引入第二套 UI 或状态框架。
- 前端只通过 `web/src/api` 的既有 adapter 访问后端；页面组件不得散落裸 `axios` 调用、硬编码服务端地址或伪造成功响应。
- 节点上下文、权限、流式鉴权、页面状态和国际化优先复用现有 composable、store、directive 和 utility，不在页面内重复实现。
- 所有用户可见文案进入 `web/src/lang` 现有国际化体系；新增功能同步维护实际支持的语言键，至少保证中文和英文键完整。
- 依赖安装与脚本默认使用仓库 README 约定的 npm；`package.json` 与 `package-lock.json` 必须同步，禁止手工编辑锁文件或在同一变更中混用包管理器。现有 `pnpm-lock.yaml` 的处置需单独确认。
- 不手工编辑 `web/dist`、自动生成的类型声明或依赖目录；确需更新生成文件时，记录生成命令并只提交仓库明确跟踪的产物。

## 文档与兼容清单

- 文档默认使用中文并保持 UTF-8；协议字段、代码标识符和业界固定术语可保留英文。
- 修改公开 API、配置项、环境变量、运行时目录、节点协议、部署方式或分发内容时，同步更新相应 `README.md`、`docs/api/`、`docs/architecture/`、`docs/operations/` 或 `docs/migration/` 文档。
- 路由清单、实现状态和隐藏能力扫描是兼容门禁。扫描结果只作为证据，不能代替真实 handler 测试、集成测试或人工部署验收。
- `partial`、`compatibility` 和 `pending` 都是待完成状态；只有真实业务、错误处理、持久化依据和测试齐备时才能标记 `implemented`。

## 主项目与开发会话连续性

- `apps/workmesh-server` 是当前主项目；`apps/1Panel` 只作为接口、行为和迁移差异的参考来源，不直接导入其私有实现，也不在该目录提交修改。
- 开始或恢复开发前，先读取 `docs/development/STATUS.md`，再按当前任务链接读取对应进度文件和代码；不得为了恢复一个任务重复扫描全部迁移文档。
- 每个独立任务使用 `docs/development/progress/` 下的一个 Markdown 文件，并在 `STATUS.md` 保留摘要、状态、证据链接和下一步。并行任务不得共用一个总进度文件。
- 状态必须使用 `[x] 已完成`、`[>] 进行中`、`[ ] 未开始` 或 `[!] 阻塞`。迁移清单中的 `partial`、`compatibility`、`pending` 和缺少验证证据的 `implemented` 均不得标记为已完成。
- 完成一个有意义的里程碑、遇到阻塞、长时间验证结束或准备结束会话时，立即更新任务进度和 `STATUS.md`；记录已改文件、验证结果、未完成事项和可直接执行的下一步。
- 进度记录只保存开发上下文，不保存密码、Token、私钥、完整请求体或其他敏感凭据。已完成项应保留最小可核验依据，未变化的文档无需重复阅读。

## 高风险确认

以下操作必须在实施前取得人工确认：

- 破坏性数据迁移、删除旧数据源、改变数据库兼容或回滚语义。
- 改变认证、RBAC、Session、MFA、节点签名、角色选举、fencing 或凭据保存策略。
- 扩大命令执行、文件访问、终端、容器、网络、沙箱或任务运行权限。
- Breaking API、删除兼容路由，或改变成功/错误 envelope。
- 修改生产 systemd、反向代理、安装/卸载脚本，执行远程部署、服务切换或旧版本清理。
- 访问真实 Gateway、Docker、SSH、ACME、生产节点或其他会改变外部状态的集成测试。

## 验证策略

按影响范围执行最低必要验证，不重复运行已被上层命令覆盖的等价测试。在上层 WorkMesh 工作树内由 Agent 执行时，外层统一使用 `node scripts/with-dev-env.mjs -- <command>`；独立克隆时将 Go 缓存和临时输出固定到本仓库 `.cache/`、`.tmp/`。

Go 定向改动先运行对应包测试；普通后端改动至少运行：

```powershell
$env:GOWORK='off'
go test ./...
go vet ./...
```

前端改动在 `web/` 运行：

```powershell
npm.cmd run type-check
npm.cmd run build:pro
```

公开路由、迁移兼容或发布改动还需运行：

```powershell
node test/contract/route-scan.mjs check --legacy ../workmesh-node --project . --manifest test/contract/routes.json
node test/contract/implementation-scan.mjs --legacy ../workmesh-node --project . --manifest test/contract/routes.json --out .tmp/implementation-status.json
node test/contract/hidden-function-scan.mjs --legacy ../workmesh-node --project . --out .tmp/hidden-function-status.json
```

依赖真实环境的测试默认禁用，只能通过显式环境变量启用；验证不得自动启动开发服务器、连接生产系统或修改宿主机服务。交付时说明实际运行的命令、结果、未覆盖项和安全结论。
