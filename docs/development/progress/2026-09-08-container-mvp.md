# 容器 P0 进度

## 已完成

- 容器列表、详情、镜像查询、Compose 操作均通过参数数组调用真实 Docker CLI。
- 新增 POST `/api/v2/containers/list`、`list/byimage`、`info` 的真实查询适配，返回前端 `name/state` 契约。
- Compose 支持 `create/update/operate/search`，`operate` 支持 `up/down/start/stop/restart/delete`；删除按 `withFile` 清理配置文件，并持久化 Compose 记录。
- Compose 操作支持 `taskID`，通过现有 SQLite 任务日志机制记录执行、失败、完成和结束标记。
- Compose 文件改为同目录临时文件、`fsync`、关闭后 `rename`，避免并发写入产生半文件。
- Docker CLI 缺失或 daemon/socket 不可用时，命令接口返回 HTTP 503 和 `code=ERR`，不返回固定成功值。
- Compose `up` 使用 detached 模式，避免 HTTP 请求被服务日志阻塞；Compose 操作仍通过 `taskID` 记录执行结果。
- 容器列表对停止、退出、创建、暂停、重启、删除和 dead 状态立即显示零资源；stats 依赖故障不会让停止态行保持 loading。
- 单容器 stats 和列表 stats 在 Docker CLI/daemon 不可用时统一返回 HTTP 503 和 `code=ERR`。

## 定向验证

`TestContainerMVPUsesRealDockerForListInspectAndImage`、`TestContainerMVPComposeAndImageOperationsUseDockerAndPersistFiles`、`TestContainerMVPReturns503WhenDockerCLIUnavailable`、`TestContainerStatsReturns503WhenDockerDaemonUnavailable` 覆盖隔离 Docker CLI、真实文件持久化、Compose 删除和不可用错误。

隔离黑盒执行：

```bash
WORKMESH_CONTAINER_EXTERNAL_TEST=1 GOWORK=off \
  go test ./node/api -run '^TestExternalContainerLifecycle$' -count=1 -v
WORKMESH_CONTAINER_EXTERNAL_TEST=1 GOWORK=off \
  go test ./node/api -run '^TestExternalComposeLifecycle$' -count=1 -v
```

结果：两条均通过。临时资源仅使用 `workmesh-acceptance-*` 名称；容器生命周期、停止态 stats、Compose `config/up/stop/start/restart/down/delete` 和文件清理均已验证。

主控集成门禁已确认容器与数据库测试可共同编译通过；容器专项未触碰生产 Docker 资源。

## 未覆盖

- 生产 Docker daemon、复杂 Compose 网络/卷、多服务编排和日志流仍需在隔离维护窗口执行黑盒验收。
- 容器 WebSocket/SSE 日志流仍需单独在带认证的反向代理链路验证。
