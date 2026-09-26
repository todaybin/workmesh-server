<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 项目 Agent 团队 API

WorkMesh Server 的项目 Agent 控制面使用 `/api/v2`。Server 独占保存 `WORKMESH_AGENT_RUNTIME_TOKEN` 主密钥，Agent Service 只领取其绑定项目的派生令牌，并在所有 `/agent-runtime/` 请求的 `X-WorkMesh-Agent-Token` 请求头中发送。Desktop 任务和 SSE 仍使用 Server 的本地登录或 API 凭据。

JSON 控制请求体统一限制为 2 MiB，并拒绝同一请求中的尾随 JSON 值；模型输出、日志和源码正文应通过有界字段或 Artifact 引用传递。

项目令牌为小写十六进制 `HMAC-SHA256(master, "workmesh.agent.v1:" + projectId)`。`projectId` 必须与请求正文或查询参数一致；主密钥和其他项目的令牌均不能直接使用。分发与轮换需要由受控的 Agent secret 装配流程完成，当前切片没有令牌发放接口；启用前需提供密钥轮换和撤销操作路径。

`apps/workmesh` fork 提供可选 `AgentRuntimeRelay` 适配层，负责上述注册、心跳、长轮询命令、幂等 ack 和 sequence 事件上报。另有 `createAgentSessionCommandHandler` 将 `task.start` 接到当前项目服务的 Session API：创建或复用 Session、提交 `prompt_async`、监听 `/event` SSE，并把增量、idle、error、cancel 映射为 Agent 事件。它只接受项目服务注册文件发现的回环地址和本机服务凭据；只有 `service foreground` 获得完整显式环境配置时才连接 Server，不会绕过 WorkMesh Server 直接操作宿主机。

## 显式启用 Agent Relay

项目服务只有在前台进程启动时同时提供以下环境变量才会连接 Server：

```text
WORKMESH_AGENT_SERVER_URL=https://server.example.com:9999
WORKMESH_AGENT_PROJECT_ID=project-a
WORKMESH_AGENT_TOKEN=<项目派生令牌>
WORKMESH_AGENT_MEMBER_ID=leader-a
```

可选变量包括 `WORKMESH_AGENT_INSTANCE_ID`、`WORKMESH_AGENT_RUNTIME_ID`、`WORKMESH_AGENT_DEVICE_ID`、`WORKMESH_AGENT_NAME`、`WORKMESH_AGENT_PROVIDER_ID`、`WORKMESH_AGENT_MODEL_ID`、`WORKMESH_AGENT_HEARTBEAT_MS`、`WORKMESH_AGENT_MAX_SESSIONS`（默认 4，范围 1-128）、`WORKMESH_AGENT_MAX_RESOURCE_UNITS`（默认 4，范围 1-512）和 `WORKMESH_AGENT_TASK_TIMEOUT_MS`（默认 30 分钟，范围 1 秒至 24 小时）。资源单位为 `small=1`、`medium=2`、`large=4`；Relay 达到 Session 数或资源单位上限的新任务会收到 `task.blocked` 和 `resource_limit` 原因，不会继续创建 Session，Server 侧则将超预算任务置为 `waiting_resource` 并在预算释放后自动派发。Session 超时会调用本地 abort，并上报 `task.failed/reason=timeout`；本地事件流意外关闭也会失败任务并释放预算。只提供部分必填变量会阻止服务启动；全部未提供时保持离线模式。令牌只能使用 Server 发放的项目派生令牌，不能填写 Server 主密钥。

## Agent runtime

```text
POST /api/v2/agent-runtime/register
POST /api/v2/agent-runtime/heartbeat
POST /api/v2/agent-runtime/events
POST /api/v2/agent-runtime/artifacts/init
PUT  /api/v2/agent-runtime/artifacts/{artifactId}/chunks?projectId=...&runtimeId=...&taskId=...&offset=...
POST /api/v2/agent-runtime/unregister
GET  /api/v2/agent-runtime/commands?projectId=...&runtimeId=...&waitSeconds=0..10（另带 `X-WorkMesh-Fencing-Token` 请求头）
POST /api/v2/agent-runtime/commands/ack
GET  /api/v2/projects/{projectId}/agent-runtime
```

注册请求至少包含 `projectId`、`runtimeId`、`instanceId` 和 `memberId`。Server 为项目 runtime 分配 `leaseId` 与 `fencingToken`，15 秒没有心跳后状态视为 offline。同一项目的活动 runtime 不能重复注册；旧 runtime 被 fencing 后不能继续发送事件。

`GET /api/v2/projects/{projectId}/agent-runtime` 是项目在线状态的权威读取入口。状态查询和项目 SSE 建连/轮询都会发现租约过期，并在同一持久化操作中将 runtime 标为 `offline`、清空过期租约并追加一次 `runtime.offline` 事件；重复读取不会重复广播。已连接的项目 SSE 客户端会收到离线事件。桌面端应使用该接口或项目事件流显示在线状态，不能仅依据本地心跳计时推断。

上行事件格式为：

```json
{
  "projectId": "project-a",
  "runtimeId": "runtime-a",
  "fencingToken": "fence-...",
  "events": [
    {"sequence": 1, "type": "task.started", "taskId": "task-...", "attempt": 1, "payload": {}}
  ]
}
```

`sequence` 必须递增；重复事件按 `acceptedThrough` 幂等确认。事件和任务状态保存在 WorkMesh Server 的 `ai_state` 数据域，事件按项目最多保留 2000 条。事件与下行命令 payload 会按字段名递归脱敏 `token`、`password`、`secret`、`apiKey`、`credential`、`privateKey` 和 `authorization` 等凭据字段，再写入事件存储或 SSE；普通文本中的代码、日志和模型输出不会做模糊替换。

Artifact 文件上传必须先发送 `artifacts/init`，请求中提供 `projectId`、`runtimeId`、`fencingToken`、`taskId` 和已登记的 `artifactId`。随后按顺序以二进制 PUT 分块，单块最大 8 MiB，`offset` 必须等于服务端当前已写入大小。最后一块写入后 Server 重新计算 SHA-256，只有摘要和登记大小都匹配时才原子提交为完成文件；临时文件位于 `WORKMESH_DATA_DIR/workmesh-artifacts/<projectId>/<taskId>/`，不接受客户端路径。桌面端可通过 `GET /api/v2/dev/tasks/{taskId}/artifacts/{artifactId}` 使用 HTTP Range 下载已完成文件。

Agent 可以用同一事件通道提交团队交接和结构化完成报告：

```json
{
  "sequence": 8,
  "type": "task.handoff",
  "taskId": "task-123",
  "payload": {
    "fromSessionId": "planner",
    "toSessionId": "coder",
    "toMemberId": "member-coder",
    "summary": "分析完成，开始实现"
  }
}
```

```json
{
  "sequence": 9,
  "type": "task.completion_report",
  "taskId": "task-123",
  "payload": {
    "status": "completed",
    "sessionId": "coder",
    "summary": "实现和测试已完成",
    "verification": "go test ./node/api",
    "artifactIds": ["diff-1", "test-1"]
  }
}
```

`task.handoff` 至少提供 `toMemberId` 或 `toSessionId`，Server 保存最近 20 条交接记录并更新任务负责人字段。`task.completion_report.status` 只能是 `in_progress`、`completed`、`failed`、`blocked` 或 `needs_input`；其中 `completed`、`failed` 和 `blocked` 会更新任务状态并触发后置依赖判断，`needs_input` 映射为 `waiting_input`。`artifactIds` 最多 64 个且只保存标识，不把 Artifact 正文写进事件。

任务创建后，如果项目有有效租约，Server 会把 `task.start` 命令放入该 runtime 的有界 command queue。Agent 通过 `commands` 读取，完成接收后用 `commands/ack` 确认；没有在线 runtime 时任务保持 `awaiting_runtime`，不会伪造已执行。

`waitSeconds` 默认为 `0`，表示立即返回；设置为 `1..10` 时，Server 最多等待指定秒数，并在同项目新命令持久化成功后立即唤醒。长轮询不续租，Agent 仍必须按心跳周期发送 `heartbeat`；请求取消或超时不会创建后台任务。

命令首次返回时状态变为 `delivered`，带有 `deliveryAttempt` 和 30 秒 `deliveryExpiresAt`。Agent 在处理完成后必须 ack；只有当前 runtime 已实际收到的 `delivered` 命令可确认，重复 ack 返回 `acked=0`。可见期内重复读取不会重复投递，Agent 崩溃或网络中断导致租约过期后，下一次读取会重新投递同一个 `commandId`。Agent 必须按 `commandId` 幂等执行。

Server 按项目保留最多 2000 条 `acked`/`superseded` 终态命令历史；压缩只删除最旧终态记录，永不删除 `queued`/`delivered` 活动命令，也不影响其他项目。该上限用于控制长期运行的状态文件增长，不改变命令重投和幂等语义。

任务查询、命令列表和 SSE 回放会再次递归脱敏历史 payload 中的凭据字段，因此旧版本写入的事件即使未在入库时脱敏，也不会原样返回；runtime 注册响应中的 `fencingToken` 是协议字段，仍按 Agent 心跳协议返回。

## Desktop 任务和事件

```text
POST /api/v2/projects/{projectId}/tasks
GET  /api/v2/projects/{projectId}/tasks
GET  /api/v2/dev/tasks/{taskId}
POST /api/v2/dev/tasks/{taskId}/input
POST /api/v2/dev/tasks/{taskId}/cancel
POST /api/v2/dev/tasks/{taskId}/retry
POST /api/v2/dev/tasks/{taskId}/complete
POST /api/v2/dev/tasks/{taskId}/approve
POST /api/v2/dev/tasks/{taskId}/reject
GET  /api/v2/projects/{projectId}/events/stream
GET  /api/v2/dev/tasks/{taskId}/events
```

任务创建至少需要 `title` 和 `instruction`，可以提供 `idempotencyKey` 与最多 32 个同项目 `dependsOn` 任务 ID。依赖未完成时任务状态为 `waiting_dependency`，失败、取消或阻塞依赖会使任务进入 `blocked`；所有依赖完成后 Server 写入 `task.ready` 并将任务加入 command queue。若项目资源预算已满，无依赖任务进入 `waiting_resource`，预算释放后再写入 `task.ready` 并入队。无依赖且有资源的任务状态为 `accepted`，由项目组长 Agent Relay 后续领取。Server 先写入事件再通过 SSE 推送，客户端使用 `Last-Event-ID` 或 `cursor` 断点恢复；事件 payload 有大小上限，大日志、截图和视频应使用 Artifact 引用。

任务创建时 `attempt=1`。调用 `retry` 后 Server 将旧任务命令标记为 `superseded`，先下发旧轮次的 `task.cancel`（本地没有活动 Session 时为空操作），随后新轮次收到带原任务说明和执行上下文的 `task.start`；依赖或资源暂不可用时，新轮次先进入等待状态。Relay 对取消命令校验轮次，为新轮次创建新的 Session，并在所有任务事件顶层回传 `attempt`。Server 对旧轮次事件继续按 sequence 确认和保留审计，标记 `staleAttempt=true`，但不更新任务快照或解除依赖。未带 `attempt` 的旧版事件按第 1 轮处理；重试后的任务必须使用支持轮次协议的 Relay。`retry` 的 200 响应表示 Server 已接受调度，实际执行和完成以新轮次的 `task.started`、`task.completed` 及验证结果为准。

当前 Relay 的任务与 Session 对应关系只在进程内保存；Agent 进程重启后的旧 Session 清理和跨进程重试恢复仍需补充持久化对账，不能以本切片宣称生产重试闭环。

任务可选 `resourceProfile`，只能是 `small`、`medium` 或 `large`，省略时默认为 `small`。Server 会把该 profile 原样写入任务和 `task.start` 命令，Relay 会再次白名单校验；未知 profile 在创建阶段拒绝，不能借任务载荷注入任意资源参数。Server 还按 `small=1`、`medium=2`、`large=4` 统计项目内 `accepted/running/waiting_input` 任务，预算由 `WORKMESH_AGENT_MAX_RESOURCE_UNITS` 控制（默认 4，允许 1-512）；超出预算的无依赖任务进入 `waiting_resource`，预算释放后自动入队。该调度护栏不替代 Sandbox 的 CPU、内存和 PID 硬限制。

任务还可以提供执行上下文 `workstreamId`、`worktreeId`、`environmentId`、`verificationOwner` 和 `baseRevision`。前四项只能是安全标识，`baseRevision` 最长 256 个字符且拒绝控制字符；还可以提供 `timeoutSeconds`（1-1800 的整数），Server 会将它和 `attempt`、资源档位一起放入 `task.start`，Relay 再限制在本地 `WORKMESH_AGENT_TASK_TIMEOUT_MS` 上限内执行。这些字段只作为任务/命令元数据传递，不会被当作宿主机路径、Shell 参数或权限凭据。真正的 worktree 创建、环境挂载和资源硬限制仍由后续 Sandbox/RuntimeProvider 切片负责。

事件仅保留项目最近 2000 条。若 `Last-Event-ID`/`cursor` 早于保留窗口，SSE 返回 `EVENT_CURSOR_EXPIRED`；客户端必须先读取任务列表/详情快照，再从当前游标重新订阅，不能假设服务端仍保留全部历史。

用户调用 `POST /dev/tasks/{taskId}/complete` 可传 `{"reason":"..."}`；未提供时记录“用户直接完成”。Server 将 `completionSource=manual`、`completionReason` 和 `completedAt` 写入任务，重复请求返回 `replayed=true`。尚未确认的旧任务指令标记为 `superseded`，在线 runtime 收到新的 `task.cancel`；晚到的 Agent 事件继续留痕，但不能覆盖人工完成状态。人工完成后旧任务不可重试，继续工作需创建新任务；若任务已由 Agent 完成，接口返回 `TASK_ALREADY_COMPLETED`。这只表示用户结束任务，不表示测试或部署成功。

用户调用 `POST /dev/tasks/{taskId}/cancel` 时，Server 记录 `cancellationSource=manual`、可选 `cancellationReason` 和 `cancelledAt`，先将该任务尚未确认的旧命令标记为 `superseded`，再排入单独的 `task.cancel`。即使项目待处理队列已满，只要该任务有可替代的旧命令，也允许取消。重复取消返回 `replayed=true`，不重复排命令。Agent 重连后不会重新领取已取消任务的 `task.start`；取消后晚到的 Agent 状态事件和完成报告继续留审计，但不覆盖人工取消的任务快照。取消任务释放项目资源预算并尝试派发等待任务。

任务列表支持 `page`（默认 1，最大 10000）和 `pageSize`（默认 50，最大 100）；仅返回当前 `projectId` 的任务，按创建顺序倒序排列，并返回项目内的 `total`。

Agent 还可以上报 `artifact.created` 事件登记 Artifact 元数据。Server 只保存 `artifactId`、白名单 `kind`、`sha256`、大小、可选 `storageRef` 和证据类型；单个任务最多保留 64 条。`GET /api/v2/dev/tasks/{taskId}/artifacts` 与 `/evidence` 支持有界分页查询，单独的 Artifact 下载只允许读取已完成且属于当前任务的文件。

任务 Artifact 还提供三个只读运维接口：`GET /api/v2/dev/tasks/{taskId}/artifacts/verify` 逐条检查文件存在、大小和 SHA-256；`GET /api/v2/dev/tasks/{taskId}/artifacts/reconcile` 扫描任务目录中的 `.part`、`.bin` 文件，报告未登记、未完成或元数据不一致的文件；`GET /api/v2/projects/{projectId}/artifacts/reconcile` 按项目扫描所有任务目录，生成跨任务 dry-run 摘要和回收候选。项目级结果额外返回 `references`/`referenceCount`：引用来源区分任务 Artifact 元数据和 `completionReport`/`completionReports.artifactIds`，并标记 `crossTask`、未知引用、文件缺失和重复引用次数。文件条目返回有界 `referenceCount`、`retentionUntil`、`retentionState` 和 `reclaimCandidate`；默认保留期为 30 天，过期且引用数为 0 的文件只进入 dry-run 候选，必须由后续人工批准回收接口处理。任务级对账目录读取最多取 513 个条目来判定截断，最多返回其中前 512 个条目的结果；项目级对账最多扫描 512 个任务、2048 个文件和 4096 条引用，任一上限超出时返回 `truncated=true`。两类对账都明确返回 `destructive=false`，当前不会自动删除任何文件，`total` 只代表本次返回条目数。项目级摘要状态包括 `registered_complete`、`partial`、`partial_after_complete`、`unregistered_partial`、`unregistered_complete`、`completed_file_without_metadata`、`symlink_rejected` 和 `unrecognized_file`，并按状态返回条数和字节数。常见任务级状态包括 `partial`（残留分块）、`unregistered_partial`（未登记残留分块）和 `completed_file_without_metadata`（文件已完成但元数据仍未完成）。核验、下载和项目/任务对账均使用只读路径，不会因为目标缺失而创建目录。

`POST /api/v2/projects/{projectId}/artifacts/reclaim-plan` 基于一次完整项目对账生成只读回收计划。计划只纳入过保留期、文件完整、状态为 `registered_complete`、路径安全且没有完成报告或跨任务外部引用的 `.bin` 文件；当前任务自身的元数据登记只作为所有权记录，不视为外部引用。计划状态固定为 `awaiting_approval`，返回 `approvalRequired=true`、`destructive=false`、文件大小、已登记的 SHA-256、`storageRef` 和总字节数，绝不删除文件。对账结果被截断时拒绝生成计划；请求可提供 `idempotencyKey`，同一项目重复请求返回 `replayed=true`。项目最多保留 100 份计划。`GET /api/v2/projects/{projectId}/artifacts/reclaim-plans` 分页返回当前项目计划，`GET /api/v2/projects/{projectId}/artifacts/reclaim-plans/{planId}` 返回当前项目单份计划；跨项目详情统一返回 `ARTIFACT_RECLAIM_PLAN_NOT_FOUND`。人工批准、实际删除和失败审计仍是后续高风险切片，不能把本接口响应解释为已回收。

当前切片已经实现控制面状态、租约、事件回放、任务交接、完成报告、Artifact 元数据索引与受控分块上传/下载，并已完成 `apps/workmesh` Relay 到本地 Session 的协议联调；Sandbox 执行器和部署 Broker 仍未接入，不能把 `dispatchStatus=queued` 解释为代码已经执行。

## 非生产 Deployment Broker dry-run

```text
POST /api/v2/projects/{projectId}/deployments/dry-run
GET  /api/v2/projects/{projectId}/deployments/plans
GET  /api/v2/projects/{projectId}/deployments/plans/{planId}
```

dry-run 必须提供非生产 `environmentId`、候选 `version` 和至少一个当前项目的已完成 Artifact 引用，最多 64 个；Artifact 只保存任务/Artifact 标识、大小、SHA-256 和受控 `storageRef`，不返回宿主路径。健康检查最多 16 个，只允许 `GET`/`HEAD`、站内相对路径、状态码 100-599 和 1-30 秒超时；省略时使用 `/health` 默认检查。`production`、`prod` 和 `live` 环境直接拒绝。

计划固定为 `awaiting_approval`、`dryRun=true`、`approvalRequired=true`、`destructive=false`、`deploymentStarted=false`。同一项目 `idempotencyKey` 幂等，项目最多保留 100 份计划，列表按项目隔离并分页。该接口不执行健康检查，不调用 `deployment_runtime`，不访问 systemd、Docker、Nginx、SSH、数据库或生产 `9999`；人工批准、真实部署、失败审计和回滚属于后续高风险切片。

详情接口只返回当前项目的计划；使用其他项目的 `planId` 统一返回 `DEPLOYMENT_PLAN_NOT_FOUND`，避免通过响应区分计划是否存在。
