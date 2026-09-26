<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-09-25 项目级多 Agent 并行开发实施

## 2026-09-26 Sandbox CLI 执行回执协议

用户已批准继续执行 Q1 Sandbox 高风险最小闭环。本轮新增固定版本 `workmesh.sandbox.v1`：能力响应必须声明工作区/网络隔离、独立网络 allowlist 能力、启动前资源限制、进程树约束及 CPU/内存/PID/磁盘硬限制；创建响应必须提供逐任务回执，Server 将隔离类型、工作区引用、全部资源上限和网络 allowlist 与请求逐项比较。旧 CLI、缺字段、资源降级或工作区/allowlist 不符均拒绝创建，不会降级到宿主执行。同步更新 `docs/api/ai-tasks.md`。

这份回执由固定摘要 CLI 自行提供，只是 Server 接口门禁，不能证明 CLI 内部确实在 Agent 启动前安装限制。仓库内未找到可接线的 ForgeVM/gVisor WorkMesh CLI，因此本轮没有把 cgroup 控制器接到 CLI helper PID，也没有宣称完成真实隔离验收；Q1 仍待 CLI 实现和隔离环境黑盒验证。

验证：

```text
node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server test ./node/service/taskruntime -run 'TestCLITaskBackend|TestCapabilityCheckedProvider' -count=1 -timeout 180s
node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server vet ./node/service/taskruntime
git -C apps/workmesh-server diff --check
```

未修改 `apps/workmesh` Agent fork，未触碰生产 `9999`、网站进程或 Docker Socket。

## 2026-09-26 在线租约事件闭环

项目 runtime 状态原先在 `GET /projects/{projectId}/agent-runtime` 读取时才将过期租约持久化为 offline；保持打开的项目 SSE 连接因而可能看不到离线事件。本轮将同一租约核验加入 SSE 建连和事件轮询：发现过期租约后，先持久化 runtime offline 和一次 `runtime.offline`，再按正常 sequence 回放；重复 SSE/状态读取不会重复追加。轮询常态只扫描 runtime 列表，只有发现到期项时才复制团队状态用于失败回滚。

验证通过：

```text
node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server test ./node/api -count=1 -timeout 180s -run 'TestAgentTeam(RuntimeStatusPersistsExpiredLeaseAndEmitsEvent|SSEEmitsExpiredRuntimeOfflineEvent|RuntimeTaskEventsAndSSE)'
node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server vet ./node/api
git -C apps/workmesh-server diff --check
```

这只验证 Server 控制面状态和 SSE 通知，不代表真实 Agent、Gateway relay 或 Sandbox 联调通过；未启动或切换生产 `9999`，未触碰网站进程。

## 状态

- [>] 进行中：已完成实施计划、架构图和使用说明文档；已落地 Agent runtime 注册/心跳/租约、项目任务控制面、Agent 事件上行和 Desktop SSE 回放；Relay 已在完整显式配置下接入项目服务并完成协议联调，项目级多 Session 调度、Sandbox、密钥生命周期仍未完成，进程会话回收基础强化已落地并继续补齐平台级 Supervisor。

## 2026-09-26 任务恢复与文档状态收敛

2026-09-26 Sandbox inspect 与重启恢复：新增只读 `TaskBackendInspector` 和固定 CLI `inspect` 操作。Server 初始化恢复时不再直接信任 `ai_state` 中旧的 `running/completed` 状态：支持 inspect 的后端必须重新返回真实状态，句柄不存在进入 `awaiting_human`，探测失败或后端缺少 inspect 能力保持 `unknown`；`unknown` 任务禁止 exec/destroy，`recover` 仅执行只读核验并写回状态。补充 API 文档和 taskruntime 回归测试。定向 `go test ./node/service/taskruntime ./node/api`、`go vet`、`git diff --check` 已通过。真实 Sandbox CLI、ForgeVM/gVisor、cgroup/网络/磁盘隔离和生产 `9999` 仍未验收。

### 计划恢复补充

本次用户反馈指出前序任务内容出现断档。经核对，控制面、Relay、SSE、Artifact、工作区和进程治理实现仍在工作树中；缺口主要是 Sandbox 后端、凭据生命周期、部署 Broker 和 `9999` 隔离演练。主计划已新增“完整需求矩阵”和 Q1–Q6 可执行队列，逐项记录责任层、接口、状态、验收证据和回滚边界。

本次变更仅涉及计划恢复与状态同步，没有改变 RuntimeProvider 权限、Sandbox 网络策略、凭据模型、生产部署配置或 `9999` 运行服务。

验证：主计划、状态文件和本进度文件执行 `git diff --check` 通过；真实 Sandbox、项目令牌轮换、非生产部署和生产 `9999` 演练仍未执行。

- [x] 恢复完整方案索引：将源码/runtime 安装位置、项目目录隔离、A/B/C Session 身份、在线状态和心跳归属、Desktop REST/SSE、Relay 长轮询、断点恢复、Artifact/实测反馈、部署审批、缓存回收、资源限制和 `9999` 隔离升级串成一条 Mermaid 交互闭环，写入 `docs/internal/agent-team-realtime-implementation-plan.md`。
- [x] 修正状态冲突：`STATUS.md` 和公开编排文档不再把已完成的 Desktop Server adapter 描述为“未接线”；当前准确状态是 Server adapter、AgentMonitor Server 区域和类型检查已完成，真实登录会话、Gateway→Server relay、Sandbox、凭据轮换、Deployment Broker 和生产页面/`9999` 验收仍未完成。
- [x] 文档校验：`node scripts/with-dev-env.mjs -- node test/scripts/project-rules-check.mjs --workflow` 通过；Server 相关文档 `git diff --check` 通过。VitePress 构建完成渲染后因既有 `dist/workmesh/app` 路径冲突在收集产物阶段失败，不能记录为文档站构建通过。
- [x] Desktop 控制面补强：`server-client.ts` 解析 SSE 非 2xx envelope，正确识别 `EVENT_CURSOR_EXPIRED`，支持标准多行 `data`；新增任务 `input/cancel/retry/complete/approve/reject` adapter、项目 runtime 状态查询；`AgentMonitor.vue` 接入取消/重试/人工完成按钮、Server 权威 runtime 在线状态和刷新请求合并，避免并发建立多条项目 SSE。`pnpm --dir packages/frontend typecheck`、Server Agent Team 定向测试和 `go vet ./node/api` 通过。

### Artifact 项目级只读回收核查

- [x] 新增 `GET /api/v2/projects/{projectId}/artifacts/reconcile`。接口只在 `WORKMESH_DATA_DIR/workmesh-artifacts/<projectId>/` 内读取，最多扫描 512 个任务目录和 2048 个文件；项目越界、任务目录或 Artifact 文件的符号链接会被拒绝并报告，不跟随逃逸路径。
- [x] 项目级结果按 `registered_complete`、`partial`、`partial_after_complete`、`unregistered_partial`、`unregistered_complete`、`completed_file_without_metadata`、`symlink_rejected` 和 `unrecognized_file` 分类，返回每类数量/字节数、扫描计数、截断标记和 `destructive=false`。实现不创建目录、不计算 SHA-256、不删除文件，后续人工回收可据此生成明确候选。
- [x] 项目级引用核查：对账同时收集任务 `artifacts`、当前 `completionReport.artifactIds` 和历史 `completionReports.artifactIds`，按任务/Artifact 返回引用次数、来源、跨任务标记、未知引用和文件缺失状态；默认保留期为 30 天，过期且无引用的残留文件只标记 `reclaimCandidate`，不执行删除。引用最多返回 4096 条，避免历史报告造成无界响应。
- [x] 回归命令：`node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server test ./node/api -run 'TestAgentTeam|TestTeamArtifact|TestProjectArtifact' -count=1 -timeout 180s`；`node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server vet ./node/api`。

### Artifact 人工回收计划控制面

- [x] 新增 `POST /api/v2/projects/{projectId}/artifacts/reclaim-plan` 和 `GET /api/v2/projects/{projectId}/artifacts/reclaim-plans`。计划复用一次完整项目对账，只纳入过保留期、`registered_complete`、路径安全且没有完成报告或跨任务外部引用的 `.bin` 文件；任务自身元数据登记只作为所有权，不视为外部引用。
- [x] 计划写入 `ai_state`，包含 `planId`、项目、候选 Artifact、大小、已登记 SHA-256、`storageRef`、检查时间、`awaiting_approval`、`approvalRequired=true` 和 `destructive=false`；支持项目内 `idempotencyKey`，每个项目最多 100 份，重试返回 `replayed=true`。
- [x] 生成计划不删除文件、不创建生产目录；对账截断时拒绝生成。新增 `TestProjectArtifactReclaimPlanIsApprovalOnlyAndIdempotent`，验证计划、幂等、项目隔离和文件仍存在。
- [!] 人工审批、实际删除、删除失败审计和 Sandbox 生命周期清理仍未实现，不能把该接口当作 GC 执行接口。

## 2026-09-26 Sandbox 准入回归

### 控制面写盘失败后的外部动作一致性

- [x] `start` 的 Sandbox 启动完成但 `ai_state` 写盘失败时，先调用 `Cancel` 补偿；补偿成功释放 `aiJobs`，补偿失败保留运行状态和资源租约，并返回需要对账的 `TASK_STATE_SAVE_FAILED`。
- [x] `collect`、`cancel`、`destroy` 的外部动作完成后，即使状态写盘失败，也按 Provider 真实终态释放 `aiJobs`；内存任务标记 `stateSyncRequired=true` 与错误原因，避免把失败响应误判为动作回滚。
- [x] 新增受保护 `POST /api/v2/workmesh/tasks/sync`：只读取 Provider 当前状态并重写 `ai_state`，不重复触发 Sandbox 动作；写盘恢复后清除同步失败标记。新增 `TestWorkMeshTaskStartSaveFailureCompensatesExternalRuntime`、`TestWorkMeshTaskTerminalActionSaveFailureKeepsStateAndReleasesSlot`，覆盖后续 sync 恢复。
- [!] 该证据只覆盖 Server 控制面和测试后端，不代表真实 ForgeVM/gVisor、cgroup v2、磁盘 quota、网络 allowlist、Windows Job Object 或生产 `9999` 部署已经验收。

- [x] `TaskSpec` 已将 `resourceProfile` 和 `resourceLimits` 传入隔离后端；Server 只接受 `small/medium/large` profile，并拒绝超过 profile 默认上限的 CPU、内存、PID 和磁盘配置。
- [x] `validateWorktree` 现在要求显式 `WORKMESH_AGENT_WORKSPACE_ROOT`，拒绝 `/www`、`/etc`、`/root`、`/proc`、`/sys`、`/dev`、`/var/run`、`/var/lib/docker`、`/home` 等宿主敏感目录，以及 workspace root 外路径和已存在路径组件的符号链接逃逸。测试专用根目录固定在 `/opt` 下并自动清理，避免 `with-dev-env` 的 `/www/.tmp` 被生产规则拒绝。
- [x] 验证通过：`node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server test ./node/service/taskruntime ./node/api -count=1 -timeout 180s -run 'TestProvider|TestWorkMeshTaskRoutesUseIsolatedProvider|TestTaskExecRequiresToken'`；`node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server vet ./node/service/taskruntime ./node/api`。
- [!] 当前实现只完成 Server 侧路径和资源契约、CLI 摘要校验及结构化参数传递；真实 cgroup v2/Windows Job Object/ForgeVM 硬限制、网络隔离和超限回收仍需隔离环境验收。生产 `9999` 和网站目录/进程未触碰。
- [x] Sandbox 能力证明已接入：固定摘要 CLI 必须响应 `--json task capabilities '{}'`，Server 校验 `forgevm/gvisor`、工作区/网络隔离、CPU/内存/PID/磁盘硬限制和最大资源上限；能力不足在任务创建时返回 `503 TASK_PROVIDER_UNAVAILABLE`。新增 `TestCLITaskBackendCapabilities*`、能力不足路由测试，定向 `go test` 与 `go vet` 通过。
- [!] 能力证明仍是外部后端声明的契约，当前仓库没有 ForgeVM/gVisor 启动器或 cgroup v2/Windows Job Object 实现；必须在隔离环境用超限、网络、符号链接和回收黑盒测试验证后，才能推进 P3 完成。
- [x] 新增 Linux `CgroupV2Controller` 适配层：受控 root 下启用 `cpu/memory/pids` controller，写入 `cpu.max`、`memory.max`、`memory.swap.max`、`pids.max`，支持 attach、残留 PID 拒绝 destroy、路径/符号链接校验；定向 `TestCgroupV2Controller*` 通过。该层不虚构磁盘容量限制，也尚未由 ForgeVM/gVisor CLI 调用。
- [!] Windows Job Object、文件系统 disk quota、网络白名单和 CLI 到 cgroup 的真实接线仍未完成；生产 `9999` 不受本轮影响。
- [x] 平台编译门禁：`GOOS=windows GOARCH=amd64 go test ./node/service/taskruntime -c` 通过，Windows 非 Linux 分支可编译；交叉编译产物已从 `.tmp` 清理。
- [x] 项目工作区派生已接入任务创建路由：`projectId` 模式生成 `<workspace-root>/<projectId>/worktrees/<taskId>`，拒绝混用客户端绝对 `worktree`，并将项目引用写入 runtime policy；legacy 请求保留但只允许受控 root。`TestWorkMeshTaskRouteDerivesProjectWorkspace` 通过。
- [x] 项目任务目录物化：创建任务时由 `WorkspaceLayout.Ensure` 在受控 root 内建立 `worktrees/<taskId>`、`tmp/<taskId>`、共享 `cache` 和 `artifacts/<taskId>`，逐级检查符号链接；目录创建失败拒绝任务，不触碰其他项目或网站路径。
- [x] 任务清理闭环：新增 `WorkspaceLayout`，`Destroy` 后只删除 `tmp/<taskId>`，不删除 worktree、项目共享 cache 或 Artifact；符号链接/回收失败会保留现场并返回 `TASK_CLEANUP_REQUIRED`，定向 `TestWorkspace*` 和 Provider 测试通过。
- [x] 项目目录物化和跨平台门禁：`WorkspaceLayout.Ensure` 在项目任务创建时逐级 `Lstat/Mkdir/Lstat` 建立四类目录并拒绝创建阶段符号链接；通用 `contextError` 移出 Linux 文件，Windows 交叉编译重新通过。完整 `node/api` 回归只保留既有网站代理测试夹具缺失 `nginx/proxy/root.conf` 的失败。
- [x] CLI 进程组回收：`CLITaskBackend` 不再依赖 `exec.CommandContext` 的单进程终止语义；Unix 将每次固定摘要 CLI 放入独立进程组，取消/超时先发送 `SIGTERM`，250ms 后发送 `SIGKILL`，Windows 使用固定参数 `taskkill.exe /PID /T /F`。新增 `TestRunCLIKillsUnixProcessGroupOnTimeout`，验证 CLI 派生的 `sleep` 在超时后退出；`go test ./node/service/taskruntime`、`go vet` 和 Windows 交叉编译均通过。该改动只治理 CLI helper 生命周期，不等同于真实 ForgeVM/cgroup/Job Object 隔离验收。

## 目标

- 一个项目一个组长 Agent Service，多 Session 协作，不重复启动同项目主 Agent。
- Agent 自己注册、续租和发送心跳；Server 管理项目成员状态、任务、事件和审批。
- Desktop 使用 REST + SSE 查看流式模型输出、工具事件、测试、Artifact 和团队动态。
- 支持项目绑定、任务 DAG、交接、用户直接完成、资源预算、Sandbox 和部署反馈。
- 并行开发时登记所有临时进程，清理进程组、僵尸、端口和临时目录，避免系统资源耗尽。
- WorkMesh Server 默认管理端口为 `9999`，在线升级不停止网站运行时。

## 输入和输出契约

每个实施任务必须声明：

```text
目标、范围、输入契约、输出契约、触达资源、风险等级、审批要求、验收命令
projectId、workstreamId、taskId、baseRevision、worktreeId
environmentId、resourceProfile、ownerMemberId、verificationOwner
```

输出必须包括：

```text
变更摘要、diff/commit、测试命令和结果、资源摘要、日志/Artifact 引用、风险和未完成项
```

## 实施阶段

1. **P0 文档和契约**：维护 `apps/workmesh-server/docs/architecture/multi-agent-execution.md`、本进度文件和运维手册；同步根台账。
2. **P1 项目绑定和成员中心**：`AgentProfile`、`ProjectAgentBinding`、`AgentInstance`、`PresenceLease`、唯一 active runtime、注册和心跳。
3. **P2 任务和 Session**：组长接收任务，SessionManager 管理 planner/coder/tester/reviewer；限制并发、嵌套深度、超时和资源。
4. **P3 实时事件**：Agent Relay、sequence/ack/outbox、SSE、cursor 恢复、事件去重和 Desktop 团队工作台。
5. **P4 进程治理**：进程组绑定、等待退出、SIGTERM→SIGKILL、zombie/端口/子孙进程检查、清理失败阻塞；补超时、取消、孤儿和人工共享服务测试。
6. **P5 Sandbox 和证据**：worktree、RuntimeProvider、Artifact、VisualEvidence、Preview、日志脱敏和缓存回收。
7. **P6 部署和升级**：`9999` 候选预检、SQLite Expand/Contract、drain、Agent 重连、网站连续探测、回滚。

当前已完成的首个代码切片是 `node/api/agent_team.go` 与 `agent_team_test.go`。它复用既有 `ai_state` 持久化，避免新增数据库迁移；Server 保管 `WORKMESH_AGENT_RUNTIME_TOKEN` 主密钥，Agent runtime 使用按项目派生的令牌，Desktop SSE 由外层 Server 会话保护。写入失败会回滚内存中的团队任务、命令和事件，SSE 只在持久化成功后收到通知。该切片只负责任务、下行命令和事件控制面，不会假装启动模型或执行源码。

用户直接完成任务现在记录 `manual` 来源和原因，旧的待处理任务指令被标记为 `superseded`，在线 runtime 收到 `task.cancel`；晚到 Agent 事件保留审计但不改写人工完成状态。此能力只覆盖 Server 控制面；真实 Agent 是否停止仍需 Agent Relay 接入后联调验证。

命令读取增加 `waitSeconds=1..10` 长轮询：复用项目事件通知唤醒，不创建常驻 goroutine，不续租 runtime；定向测试覆盖任务入队唤醒。

命令投递增加 30 秒可见期和 `deliveryAttempt`；可见期内不重复投递，过期后按相同 `commandId` 重投，要求 Agent 幂等处理。

命令 ack 仅接受当前 runtime 的 `delivered` 状态，重复确认不会重复计数。

项目任务列表增加有界 `page/pageSize` 分页，返回项目内准确 `total`；定向测试覆盖跨项目隔离和第二页。

SSE 订阅发现 cursor 早于 2000 条保留窗口时返回 `EVENT_CURSOR_EXPIRED`，要求客户端先刷新快照；避免静默漏事件。

任务创建支持最多 32 个同项目依赖；依赖未完成时等待，依赖完成事件会自动释放后置任务并入队，定向测试覆盖先行/后置任务。

## 并行任务安排

默认最多四个席位：

```text
主 Agent：任务拆分、资源锁、合并和最终重验证
实现 Agent：生产代码和协议实现
测试 Agent：定向测试、故障复现和回归
Review Agent：只读审查安全、契约、性能和证据
```

同一项目写任务使用独立 worktree；测试、编译和发布由唯一验证负责人按队列执行，`WORKMESH_VERIFY_CONCURRENCY=1`。其他 Agent 不重复启动全量验证或开发服务。

## 进程安全验收

当前 `scripts/process-session.mjs` 已有会话登记、5 秒心跳和超过 120 秒 stale sweep；本轮已增加 Unix `/proc/<pid>/stat` 状态和 starttime/PGID 归属校验、SIGTERM 等待后 SIGKILL 升级、子孙进程复核、端口和临时目录检查，以及失败时保留 `awaiting_human` 会话。仍未实现 cgroup/Windows Job Object 绑定，也不能据此宣称宿主机全局“无僵尸进程”。

验收至少覆盖：

- 正常退出和子进程回收；
- 超时后 SIGTERM 等待和 SIGKILL 升级；
- orphan/zombie 区分和归属校验；
- 端口未释放时保留会话并进入 blocked；
- 人工共享服务只 retire，不误杀；
- stale sweep 不清理不明归属 PID；
- 取消、失败和崩溃后的临时目录、句柄和日志清理。

已通过的定向回归：

- `node scripts/with-dev-env.mjs -- node test/scripts/process-lifecycle-check.mjs`
- `node scripts/with-dev-env.mjs -- node test/scripts/development-paths-check.mjs`

测试只验证 WorkMesh 自有登记进程；主机上发现的外部 `relay-watcher` `[sh] <defunct>` 仍按归属隔离，只读告警，不由 WorkMesh 清理。

## 当前验证

### 本轮 P2 交接与完成报告切片

- [x] Agent sequence 事件支持 `task.handoff`：校验目标 Session/成员，保存最近 20 条交接历史并更新任务负责人字段。
- [x] Agent sequence 事件支持 `task.completion_report`：白名单状态、验证摘要和最多 64 个 Artifact 标识；终态报告更新任务状态并触发后置依赖判断。
- [x] 非法协作 payload 在写入前拒绝，不推进 Agent sequence、不写入 SSE 或任务快照；人工完成终态不会被晚到完成报告覆盖。
- [x] 定向测试：`node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server test ./node/api -count=1 -timeout 180s -run 'TestAgentTeam'` 通过。
- [x] Artifact 元数据索引：`artifact.created` 事件校验 `artifactId/kind/sha256/size`，每个任务最多 64 条；`/artifacts` 和 `/evidence` 提供有界分页查询。
- [x] Artifact 文件闭环：初始化、8MiB 顺序分块、offset 校验、最终 SHA-256/大小校验、原子提交和 HTTP Range 下载；文件限制在 `WORKMESH_DATA_DIR/workmesh-artifacts`，不接受客户端路径。
- [x] Artifact 生命周期核验：`/artifacts/verify` 检查文件存在、大小和 SHA-256；`/artifacts/reconcile` 只读报告未登记或未完成文件，项目级对账同时输出任务元数据/完成报告引用、跨任务/未知引用、30 天保留期和无引用回收候选，明确 `destructive=false`，不自动删除。
- [x] Artifact 只读边界补强：核验、下载和对账路径不创建缺失目录；对账最多读取 513 个目录条目、返回前 512 个结果并返回 `truncated`，区分 `partial`、`unregistered_partial` 和 `completed_file_without_metadata`，定向测试覆盖缺失文件、残留分块、未登记完成文件和截断边界。
- [x] 事件凭据脱敏：团队事件和下行命令的嵌套 payload 按字段递归屏蔽 token/password/secret/apiKey/credential/privateKey/authorization，再持久化和推送 SSE；普通文本不做替换，定向测试覆盖嵌套 `apiKey` 与 `access_token`。
- [x] JSON 输入边界：`aiBody` 统一读取上限为 2MiB，拒绝尾随 JSON 值；新增 Agent Team 超限和拼接 JSON 回归测试。
- [x] Server 资源准入：项目任务按 `small=1`、`medium=2`、`large=4` 计量，默认预算 4，可由 `WORKMESH_AGENT_MAX_RESOURCE_UNITS` 设置到 1-512；超限任务进入 `waiting_resource`，活动任务完成/取消后自动释放并入队。补充多命令读取回归，修复 query 缺省时 `limit` 被解析为 0 的问题。
- [x] Command queue 有界历史：每项目终态 `acked/superseded` 最多保留 2000 条，压缩最旧记录但保留活动命令和其他项目；新增压缩回归测试。
- [x] 历史状态回放脱敏：团队任务/命令查询和 SSE 写出统一递归脱敏嵌套凭据，覆盖旧 `ai_state` 数据；保留 runtime 注册 `fencingToken` 协议字段，新增历史 payload 回归测试。
- [x] 取消竞态治理：用户取消任务时，旧未确认 `task.start` 命令先失效，再派发 `task.cancel`；重复取消幂等，晚到 Agent 状态/完成报告不覆盖人工取消快照，队列满但本任务有可替代旧命令时仍可取消。定向测试覆盖这些路径。
- [>] 重试链路已引入同任务 `attempt`：旧命令失效，旧轮次先取消，新轮次下发 `task.start`；Agent 任务事件带轮次，Server 将晚到旧轮次事件标为 `staleAttempt` 并仅留审计。Server 定向测试覆盖命令顺序、旧完成报告隔离和新轮次完成，已通过。Relay 定向测试 10 pass；跨进程 Server→Relay 重试联调待完成；接口 200 只表示调度被接受。
- [x] 任务执行上下文元数据：任务创建可声明 `workstreamId/baseRevision/worktreeId/environmentId/verificationOwner`，服务端只接受受限标识和无控制字符的 revision，并将元数据原样带入 `task.start`；当前只物化受控目录，不创建 Git worktree、不挂载环境，实际隔离仍由 Sandbox/RuntimeProvider 负责。
- [x] Artifact 定向测试已包含在上述 `TestAgentTeam` 运行中；`node/api` `go vet` 也通过。

已通过只读/临时验证：

```text
node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server test ./node/api ./cmd/workmesh-server -count=1 -timeout 180s -run 'TestAgentTeam|TestAgentRuntimeRegistrationPassesBothSecurityLayers'
node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server vet ./node/api ./cmd/workmesh-server
env GOWORK=off go test ./cmd/workmesh-server -count=1 -timeout 180s -run 'TestAgentRuntimeRegistrationPassesBothSecurityLayers|TestSecurityWrapperProtectsControlAndLeavesHealthPublic'
node test/contract/route-scan.mjs check --legacy /www/apps/1Panel --project . --manifest docs/inventory/route-inventory-1panel.json
git -C apps/workmesh-server diff --check
node scripts/with-dev-env.mjs -- node test/scripts/process-lifecycle-check.mjs
node scripts/with-dev-env.mjs -- node test/scripts/development-paths-check.mjs
```

新增定向测试覆盖跨项目令牌拒绝、主密钥不能冒用项目令牌、持久化失败回滚、人工完成终态、长轮询唤醒，以及命令 delivery lease 的可见期和过期重投。完整 `node/api` 测试仍保留既有 `nginx/proxy/root.conf` 网站代理用例失败，见 `STATUS.md`；本轮没有运行完整发布验收。

本轮又增加任务 DAG 依赖、SSE 过期 cursor 和项目任务分页测试；最新 `node/api` Agent Team、`cmd/workmesh-server` 安全链路、`go vet` 和 759 条路由契约均通过。未修改 `apps/workmesh` fork，未启动或切换 9999 生产服务。

随后在 `apps/workmesh` 已登记补丁路径新增 `AgentRuntimeRelay` 协议适配层和本地 HTTP 测试；本轮又新增受控 `createAgentSessionCommandHandler`：任务开始时创建或复用本地 Session，提交 `prompt_async`，订阅本地 `/event` SSE，将 `message.part.delta`、Session idle/error 映射为 Server 事件，取消走 Session abort。服务发现默认读取项目服务注册文件，只接受 `127.0.0.1`/`localhost`，也支持测试注入；现在已挂入 `service foreground`，但只有完整的显式环境配置才启动 Relay，半配置直接拒绝，未配置保持离线。

该 fork 当前通过仓库隔离环境安装完整依赖后，Relay 定向测试已通过 `5 pass / 0 fail`，覆盖显式配置校验、注册/长轮询/ack/sequence、Server→Session→SSE→Server 真实联调、并发预算阻断和 Session 取消适配。全仓 `bun typecheck` 仍因 `tsgo` 在当前内存上限下被 SIGKILL/OOM，不能记录为全仓类型检查通过；需在资源更充足的验证机重跑。

本轮服务端新增 `resourceProfile` 白名单（`small`/`medium`/`large`，默认 `small`），Relay 对下行命令再次校验未知 profile 并上报 `task.blocked/resource_profile_invalid`；Server Agent Team 定向测试和 `go vet ./node/api` 均通过。

Relay 现在按 profile 计算项目资源单位（`1/2/4`），同时受 `WORKMESH_AGENT_MAX_SESSIONS` 和 `WORKMESH_AGENT_MAX_RESOURCE_UNITS` 双重准入限制；Server 也按同一预算将超限任务置为 `waiting_resource`，资源释放后自动入队；超限 Relay 任务上报 `task.blocked/resource_limit`，定向测试覆盖 large 占满预算、Server 等待和释放后派发。

本轮补充 Session 任务超时和事件流提前关闭处理：超时调用本地 `/abort` 并上报 `task.failed/reason=timeout`，事件流异常结束上报 `event_stream_closed`，两种路径都会释放 Session 资源；Relay 定向测试现为 `7 pass / 0 fail`。

`dev-shared-reuse-check.mjs` 当前因环境缺少 `@clack/prompts` 被阻断，不能当作复用通过证据。当前主机还观察到一个非 WorkMesh 归属的 `[sh] <defunct>`，不得由 WorkMesh Agent 清理；后续验收必须读取 `/proc/<pid>/stat` 或 `ps STAT` 并按归属隔离。

2026-09-25 重试切片验证：Server `TestAgentTeamRetry|TestAgentTeamRuntimeTaskEventsAndSSE|TestAgentTeamCancel` 通过，`go vet ./node/api` 通过；Agent Relay 定向测试 `10 pass / 0 fail`。新用例验证 `attempt=1→2`、先取消后启动、旧完成报告与不带轮次的旧事件只留审计、新轮次完成可更新快照、Agent 收到晚到旧取消不会中止新 Session，取消失败或异常关闭事件流时保留未确认停止的旧 Session，阻止新轮次被误确认。完整 `bun --cwd apps/workmesh/packages/opencode typecheck` 仍因 `tsgo` 首次被 SIGKILL、再次运行在约 3 GiB 内存和满交换空间下主动中止；fork 检查被当前工作树的 `origin` 指向内网镜像且缺少 `upstream` remote 阻断，未修改 Git remote。跨进程 Server→Relay 重试联调、Sandbox 和生产部署尚未验收。

2026-09-26 任务生命周期恢复切片：补齐 `TaskProvider.State` 只读状态接口；取消后的任务即使继续 `collect` 也保持 `cancelled`，清理失败同时保留 Provider 的 `awaiting_human` 状态和持久化状态，避免进程重启后误恢复为 `created`。验证命令：

```text
node scripts/with-dev-env.mjs -- go -C apps/workmesh-server test ./node/service/taskruntime ./node/api -run 'TestProviderCollectPreservesCancelledState|TestProviderDestroyRejectsRunningTask|TestWorkMeshTaskRoutesUseIsolatedProvider|TestWorkMeshTaskDestroyReportsCleanupRequired'
node scripts/with-dev-env.mjs -- go -C apps/workmesh-server vet ./node/service/taskruntime ./node/api
```

本切片未启动、切换或重启生产 `9999`，未触碰网站进程、网站目录或网站容器。

2026-09-26 非生产 Deployment Broker dry-run：新增项目级部署计划控制面。请求校验非生产环境、版本、项目内已完成 Artifact 引用和有界 GET/HEAD 健康检查；计划只保存 Artifact 元数据，不暴露宿主路径。历史 Artifact 的不安全 `storageRef` 会替换为 Server 生成的受控引用。计划写入 `ai_state` 后保持 `awaiting_approval`，幂等键按项目生效，列表和详情按项目隔离分页，超过 100 份拒绝。实现不调用 `deployment_runtime`，不执行健康检查，不改变 `localDeployment`，不触碰生产 `9999`。补充尾随 JSON、路径穿越、浮点状态码、超长幂等键、计划上限和宿主路径替换边界测试。定向验证：`TestProjectDeploymentDryRun*`、Agent Team/Artifact 回归通过；`go vet ./node/api` 已在本轮执行。

2026-09-26 控制面失败回滚与计划重载补强：任务 Sandbox 句柄写入 `ai_state` 失败时恢复内存索引和任务数组，并主动销毁刚创建的隔离句柄，避免出现重启后无法回收的孤儿任务；任务状态更新写盘失败同样恢复旧状态。新增状态写入失败回归测试。Artifact 回收计划详情补充 `awaiting_approval`、`approvalRequired=true`、`destructive=false` 不变量断言；Deployment Plan 增加同样不变量和重启后详情回读测试，证明计划仍按项目隔离且不会被解释为已部署。定向验证：`go test ./node/api ./node/service/taskruntime -run 'TestProjectArtifactReclaimPlan|TestProjectDeploymentDryRun|TestWorkMeshTask|TestCapability|TestCgroup|TestWorkspace'`、`go vet ./node/api ./node/service/taskruntime`、`git diff --check` 均通过。

2026-09-26 任务清理重试与失败状态恢复：`TaskProvider.Destroy` 在 Sandbox 后端销毁成功但任务临时目录清理失败时进入 `awaiting_human`；后续重试只执行受控 `tmp/<taskId>` 清理，不重复调用已经完成的后端销毁。`Start`/`Cancel` 的后端暂时失败保留原状态，避免把可重试故障伪装成不可恢复的 `failed`。新增清理重试和状态保留回归测试；定向验证：

```text
node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server test ./node/service/taskruntime ./node/api -run 'TestProviderOperationFailurePreservesRetryableState|TestProviderDestroyRejectsRunningTask|TestWorkMeshTaskDestroyReportsCleanupRequired|TestWorkMeshTaskStateSaveFailureRollsBackMemory|TestWorkMeshTaskRoutesUseIsolatedProvider' -count=1 -timeout 180s
node scripts/with-dev-env.mjs -- env GOWORK=off go -C apps/workmesh-server vet ./node/service/taskruntime ./node/api
```

本切片仍未接线真实 ForgeVM/gVisor、磁盘 quota、网络隔离或生产 `9999` 发布；人工清理失败继续保持 `awaiting_human`，不自动删除不明归属目录。

## 安全和阻塞

- 触达 `apps/workmesh` 源码需要读取其 `AGENTS.md`，登记 WorkMesh 补丁并执行 fork/Bun 检查。
- RuntimeProvider、网络、凭据、权限、Sandbox、生产部署和破坏性迁移属于高风险边界。
- 真实 Sandbox、mTLS、生产 `9999` 切换和网站零影响必须在隔离或维护窗口验收。
- 项目令牌分发、轮换和撤销尚无自动接口；真实 Agent 接入前必须完成受控 secret 装配，不能把主密钥传给 Agent。
- 进程清理失败、端口未释放、任务临时文件残留或验证负责人未完成时，任务保持 `blocked/awaiting_human`。
