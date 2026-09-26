<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Sandbox 任务接口

任务接口只接受已登记的节点任务令牌 `X-WorkMesh-Token`。Server 不执行宿主 Shell；任务必须由固定摘要的 Sandbox CLI 处理。

## 启用条件

生产环境同时配置：

```text
WORKMESH_TASK_CLI=/absolute/path/to/sandbox-cli
WORKMESH_TASK_CLI_SHA256=<sha256>
WORKMESH_AGENT_WORKSPACE_ROOT=/opt/workmesh-server/projects
```

CLI 除生命周期操作外必须支持能力探测：

```text
sandbox-cli --json task capabilities '{}'
```

成功响应的 `data` 至少包含：

```json
{
  "protocolVersion": "workmesh.sandbox.v1",
  "sandboxType": "forgevm",
  "backend": "gvisor",
  "isolation": "container",
  "workspaceIsolation": true,
  "networkIsolation": true,
  "networkAllowlist": true,
  "preStartEnforcement": true,
  "processTreeContainment": true,
  "hardCpu": true,
  "hardMemory": true,
  "hardPids": true,
  "hardDisk": true,
  "maxResourceLimits": {
    "cpuQuotaMicros": 100000,
    "memoryBytes": 1073741824,
    "pidsMax": 256,
    "diskBytes": 4294967296
  }
}
```

`protocolVersion` 必须为 `workmesh.sandbox.v1`。`networkAllowlist` 单独表示后端支持按主机 allowlist 限制出站网络；
只有 `networkIsolation` 不代表支持 allowlist。`preStartEnforcement` 表示限制在
Agent 进程启动前已应用，`processTreeContainment` 表示 Agent 派生进程不会逃出任务边界。
创建任务时，CLI 还必须在 `data.enforcement` 返回逐任务回执，包含相同的协议版本、隔离类型、
`workspaceRef`、完整 `resourceLimits`、`allowedHosts` 和上述布尔字段；存在 allowlist 时还必须返回
`networkAllowlistEnforced: true`。Server 会逐字段比对请求，
缺失或不一致时拒绝创建。只返回 `sandboxId` 的旧 CLI 不再兼容。

`create` 响应示例：

```json
{
  "ok": true,
  "data": {
    "sandboxId": "sandbox-123",
    "enforcement": {
      "protocolVersion": "workmesh.sandbox.v1",
      "sandboxType": "forgevm",
      "backend": "gvisor",
      "isolation": "container",
      "workspaceRef": "project-a",
      "resourceLimits": {
        "cpuQuotaMicros": 100000,
        "memoryBytes": 1073741824,
        "pidsMax": 256,
        "diskBytes": 4294967296
      },
      "workspaceIsolation": true,
      "networkIsolation": true,
      "networkAllowlistEnforced": true,
      "allowedHosts": [],
      "preStartEnforcement": true,
      "processTreeContainment": true,
      "hardCpu": true,
      "hardMemory": true,
      "hardPids": true,
      "hardDisk": true
    }
  }
}
```

缺少 CLI、摘要不匹配、能力探测失败或能力不足时，任务创建接口返回 `503 TASK_PROVIDER_UNAVAILABLE`；不得降级为宿主进程、Docker Socket 或网站目录执行。

## 任务创建

`POST /api/v2/workmesh/tasks/create`

请求至少包含 `projectId`、`taskId`、`imageDigest` 和 `/opt/workmesh/` 下的 `entrypoint`。`resourceProfile` 只允许 `small`、`medium`、`large`，默认 `small`。Server 会把归一化后的 CPU、内存、PID 和磁盘上限传给 Sandbox，并拒绝超出 profile 或后端能力上限的请求。

项目模式下 `projectId` 和 `taskId` 只能使用安全标识（不含路径分隔符，且不能是 `.` 或 `..`）。Server 派生 `WORKMESH_AGENT_WORKSPACE_ROOT/<projectId>/worktrees/<taskId>`，并在任务创建时物化对应的 `worktrees/<taskId>`、`tmp/<taskId>`、共享 `cache` 和 `artifacts/<taskId>` 目录；客户端不能同时提交 `worktree`。敏感宿主目录、越界路径和符号链接路径会被拒绝。未传 `projectId` 的旧调用仍可提交显式 `worktree`，但必须位于配置的 workspace root 内。

## 生命周期

```text
POST /api/v2/workmesh/tasks/start
POST /api/v2/workmesh/tasks/exec
POST /api/v2/workmesh/tasks/collect
POST /api/v2/workmesh/tasks/cancel
POST /api/v2/workmesh/tasks/destroy
POST /api/v2/workmesh/tasks/sync
POST /api/v2/workmesh/tasks/recover
```

每个任务只能按 Provider 状态机推进。`exec` 只接受 argv 数组，禁止 Shell 字符串；销毁时 Server 只回收当前任务 `tmp/<taskId>`，保留 worktree、共享 cache 和 Artifact。无法安全回收时返回 `409 TASK_CLEANUP_REQUIRED` 并进入人工处理，不自动删除不明归属目录。

`awaiting_human` 表示 Sandbox 后端已经完成销毁，但任务临时目录仍需要人工修复或确认。修复目录后可以再次调用 `destroy`，Server 只重试目录清理，不会重复调用已完成的 Sandbox 销毁。启动或取消等后端操作暂时失败时，Provider 保留原任务状态，调用方可以在修复后重试。

如果 Sandbox 动作已经完成但 `ai_state` 写盘失败，接口仍返回 `TASK_STATE_SAVE_FAILED`，不会伪造动作未发生：

- `start` 会先尝试取消刚启动的 Sandbox；补偿成功后释放 `aiJobs` 槽位，补偿失败则保留 `running` 和资源租约，等待人工或对账处理。
- `collect`、`cancel`、`destroy` 会按 Provider 的真实终态释放 `aiJobs` 槽位，并在内存任务记录中标记 `stateSyncRequired=true` 及错误原因，供后续重试写盘。
- 状态写盘重试成功后清除同步失败标记；在此之前不能把 HTTP 错误解释为 Sandbox 已回滚或重新执行成功。

`sync` 只读取当前进程 Provider 状态并重写 `ai_state`，用于恢复同一进程内记录的控制面写盘失败；它不会再次启动、取消、收集或销毁 Sandbox。进程重启后如果后端没有独立 inspect 能力，Server 不会用旧缓存冒充真实对账，必须走后端恢复流程。

`recover` 是重启恢复或人工复核入口，只调用 Sandbox CLI 的只读 `inspect` 操作。后端返回的 `found=false` 会把任务标记为 `awaiting_human`；inspect 失败或后端不支持 inspect 时保持 `unknown`，不会执行启动、取消、收集或销毁。旧的 `running`、`completed` 等缓存状态在恢复时都必须重新核验，`destroyed` 记录不再访问已销毁句柄。

Sandbox CLI 必须支持：

```text
sandbox-cli --json task inspect '{"sandboxId":"..."}'
```

成功响应的 `data` 为 `{ "found": true, "state": "created|running|cancelled|completed|failed|awaiting_human|destroyed" }`；句柄不存在返回 `{ "found": false }`。未知状态、尾随 JSON 或无效 envelope 都会被拒绝。

当前文档描述的是 Server 与 CLI 的能力契约；真实 ForgeVM/gVisor、cgroup v2、Windows Job Object 和网络隔离后端仍需在隔离环境完成黑盒验收。
