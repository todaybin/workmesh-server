<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 多 Agent 并行开发与实时执行手册

## 使用前提

- WorkMesh Server 管理端口默认是 `9999`；`/health` 只检查进程存活，`/ready` 检查短超时依赖。
- 用户任务必须有目标、范围、输入/输出契约、风险、审批要求和验收命令。
- 项目写任务必须使用独立 worktree；同一项目的重型验证由一个负责人排队执行。
- 共享热更新服务由人工启动，Agent 只能检查和复用。
- Server 仅在受限环境变量中保存 `WORKMESH_AGENT_RUNTIME_TOKEN` 主密钥；Agent runtime secret 只保存对应项目的派生令牌：小写十六进制 `HMAC-SHA256(master, "workmesh.agent.v1:" + projectId)`。Agent 在 `X-WorkMesh-Agent-Token` 请求头中发送项目令牌；读取命令时还需通过 `X-WorkMesh-Fencing-Token` 请求头发送当前租约凭据。密钥不得写入任务、事件、URL、日志或 Desktop。当前还没有自动发放/轮换接口，启用真实 Agent 前需完成受控分发、轮换和撤销。

## 启动任务前

```powershell
pnpm.cmd agent:processes:status
pnpm.cmd agent:processes:sweep
pnpm.cmd check:dev-shared [target...]
```

如果共享服务未就绪，由人工启动：

```powershell
pnpm.cmd dev:hot -- gateway frontend
```

Agent/测试脚本不得自行执行 `dev:*` 或重复启动 Gateway、Vite、VitePress、Electron。

## 并行分工

```text
主 Agent：拆任务、建立依赖、分配资源锁、合并、最终重验证
实现 Agent：只修改分配的代码和 worktree
测试 Agent：定向测试、复现失败、保存证据
Review Agent：只读审查安全、接口、性能和测试完整性
```

同一文件、API、数据库表、权限或模型配置发生重叠时必须串行或建立独立 worktree。测试发现生产缺陷时创建新的修复任务，不降低原断言强度。

## 临时进程登记

Agent 自己启动构建、测试、Smoke Test 或 runtime 时，必须使用：

```powershell
node scripts/with-dev-env.mjs -- <command>
```

并登记：

```text
taskId
ownerType
cwd
pid
进程组或 cgroup
子进程
端口
startedAt / heartbeatAt
```

进程会话保存在：

```text
.tmp/dev-shared/processes/
```

每 5 秒发送心跳。任务完成、失败、取消和超时都必须走同一清理流程。

## 清理顺序

1. 先发送任务取消或 Agent interrupt；
2. 停止登记的进程组和子树；
3. 等待进程退出；
4. 超时后按策略升级到 `SIGKILL`；
5. 读取 `/proc/<pid>/stat` 或 `ps`，区分 running、dead 和 zombie；
6. 检查子孙 PID、监听端口和临时目录；
7. 只有全部确认后删除进程会话 JSON；
8. 任一项失败则保留记录并标记 `blocked/awaiting_human`。

检查命令：

```powershell
pnpm.cmd agent:processes:status
pnpm.cmd agent:processes:sweep
pnpm.cmd agent:processes:clean -- --task <taskId>
```

不得使用模糊 `pkill`、无归属 `kill -9` 或按端口误杀。人工共享服务标记为 `human_shared`，只能 retire 会话登记，不能自动停止进程。系统中存在不属于 WorkMesh 的 zombie 时，只读记录并交给其父进程或宿主 supervisor 处理。

## 验证队列

设置唯一重验证负责人：

```powershell
$env:WORKMESH_VERIFY_CONCURRENCY='1'
pnpm.cmd verify:fast -- --files <changed-files>
pnpm.cmd verify:changed -- --files <changed-files>
```

只有合并、发布或用户明确要求时运行一次 `pnpm.cmd verify:commit`。其他 Agent 复用已有结果或执行定向检查，不重复占用 CPU、内存和端口。

## 任务和实时反馈

- 用户把任务分配给项目组长 Agent。
- 组长在一个项目 Agent Service 内创建 planner/coder/tester/reviewer Session。
- Desktop 通过项目或任务 SSE 查看 `message.delta`、工具事件、文件变更、测试、Artifact 和审批。
- SSE 断线使用 `Last-Event-ID` 或 cursor 补取；关闭页面不会取消任务。
- 用户可追加输入、取消、重试、转派、审批或直接完成；手工完成必须记录原因，不能伪造验证通过。
- 任务创建后，在线 runtime 从 command queue 领取 `task.start` 并用 ack 确认；没有有效租约时任务保持 `awaiting_runtime`，恢复心跳后再领取。

## 失败处理

```text
Agent 断线 -> 停止新任务 -> 保留任务租约 -> 尝试恢复 Session
恢复失败 -> agent_disconnected -> awaiting_human/failed
资源超限 -> 停止创建 Session -> 终止低优先级 Worker -> 写 resource.exhausted
清理失败 -> 保留进程记录、端口和日志 -> blocked/awaiting_human
```

## Server 在线升级

1. 候选版本写入独立 `releases/<releaseId>`；
2. 只读副本执行迁移和 `/health`、`/ready` 检查；
3. 旧 Server 在 `9999` 继续服务并进入 drain；
4. 切换 `9999` 到新版本；
5. Agent Relay 重连并补发事件 sequence；
6. 验证管理 API 和网站持续访问；
7. 失败时切回旧版本，保留新版本和失败证据。

发布脚本不得停止或重启网站 PHP-FPM、Node、容器、数据库、Nginx 网站配置、SSL、DNS 或 `/www/wwwroot` 内容。新旧 Server 不能同时写同一个 SQLite。

## 交付前检查

```powershell
pnpm.cmd agent:processes:status
pnpm.cmd check:dev-shared [target...]
node scripts/with-dev-env.mjs -- node test/scripts/process-lifecycle-check.mjs
node scripts/with-dev-env.mjs -- node test/scripts/development-paths-check.mjs
git diff --check
```

交付前必须确认：

- 临时进程和端口已释放；
- `.tmp/dev-shared/processes` 没有本任务残留；
- 没有不明归属的进程被清理；
- 生成的日志、Artifact、diff 和测试结果可追溯；
- 未把 `partial/pending/compatibility` 记录为完成；
- 所有阻塞和人工审批要求已写入任务。
