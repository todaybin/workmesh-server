# SQLITE-BOOT-01 启动与 SQLite 就绪检查

日期：2026-09-08
状态：`[x] 已完成`（本地启动/迁移专项）

## 范围

- 单进程入口：`cmd/workmesh-server/main.go`
- 启动数据目录：`cmd/workmesh-server/cli_storage.go`
- 统一状态初始化：`cmd/workmesh-server/startup_storage.go`
- SQLite 打开与迁移：`internal/storage/storage.go`、`internal/storage/migrations.go`

## 已确认

- 服务只有 `cmd/workmesh-server` 一个进程入口；control、node、runtime 共享同一 SQLite 连接池。
- `initializeServerState` 在迁移、旧数据导入、共享服务注入、部署状态恢复或角色校验失败时关闭已打开的 SQLite，失败不会进入 HTTP 监听阶段。
- `/ready` 在统一存储和后台服务装配完成前返回 `503`，仅在 HTTP 服务启动流程末尾置为 ready；关闭阶段重新置为未就绪。
- 数据目录只创建 `logs`、`releases` 等启动必需目录，应用数据目录按功能首次使用创建，避免启动时无界创建资源。
- SQLite 连接池上限为 4、空闲连接上限为 1；设置 `journal_mode=WAL`、`synchronous=NORMAL`、`foreign_keys=ON`、`busy_timeout=5000`。
- 迁移使用带 checksum 的版本账本；checksum 变化会阻断启动。首次迁移写入 `applied` 审计，重复启动写入 `noop` 审计，不重复执行 DDL。
- 迁移审计记录制品 SHA-256 与备份目录，便于发布回滚追踪。
- 本地隔离迁移演练以独立子进程连续启动两次，确认 13 个统一迁移仅首次应用、第二次为 noop、失败记录为 0，SQLite 文件可关闭并重新打开。

## 验证命令

```text
GOWORK=off go test ./cmd/workmesh-server ./internal/storage -run 'Test(IsolatedMigrationRehearsal|OpenConfiguresSQLiteAndCreatesSchemaIdempotently|ApplyMigrationsRejectsChecksumConflict|Readiness|CLIListenIP)' -count=1
```

结果：`cmd/workmesh-server` 与 `internal/storage` 均通过。

## 未覆盖项

- 未连接真实生产 SQLite、systemd、Docker 或 Gateway；生产部署与真实端口验收仍需主控在授权环境执行。
- 未执行全量 `go test ./...`、`go vet ./...`，由主控在源码冻结后统一执行。
- 单进程约束已由入口结构和启动演练确认，但生产环境仍需检查 systemd `ExecStart` 未启动重复实例，并检查实际 WAL/SHM 文件生命周期。

## 下一步

1. 主控统一执行全量 Go 门禁和路由/契约测试。
2. 在隔离部署机执行一次真实 systemd 启停，记录 `/ready`、SQLite WAL/SHM、日志和资源占用。
3. 取得真实 Gateway 凭据后执行注册、任务 fencing、断线恢复验收。
