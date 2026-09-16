<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 运行环境生产就绪验收清单（2026-09-07）

状态：`[!] 阻塞`。本文件是 Go、Node.js、Python、Java、.NET、PHP 六类运行环境的真实上线验收执行单。当前只完成代码/契约审计和可重复的单元测试，未启动、停止、删除或重建任何生产容器，未修改 `apps/1Panel`，未使用模拟 JSON 代替业务数据，也未执行全量 Go 测试。本次仅新增文档，不改变运行时、数据库、前端或生产服务。

## 1. 范围与通过定义

运行时 API 统一使用 `/api/v2`，成功响应必须是数字 `code: 200`；`operate` 使用 1Panel 兼容值 `up`、`down`、`restart`，并兼容前端发送的 `ID` 字段。所有创建、更新、启停、删除和扩展操作都必须由真实 SQLite 记录、真实 Compose/Docker 结果和真实任务日志共同证明。单元测试中注入的命令执行器只能证明命令顺序和错误处理，不能证明生产镜像、端口、权限或网络可用。

每个运行时必须形成以下闭环：

`创建（可选 install） -> 查询详情 -> 容器/端口核对 -> 停止 -> 启动 -> 重启 -> 日志/任务 -> 更新或备注 -> 删除 -> SQLite 重启恢复 -> 使用相同名称重建 -> 网站实际访问`。

任何一步失败都要记录 HTTP 状态、脱敏响应摘要、任务 ID、`runtime_tasks`/`runtime_task_logs` 行、Docker stderr、Compose 文件和回滚结果；禁止把失败改写为成功或空列表。

## 2. 当前实现和数据边界

| 能力 | 代码入口 | 当前可证明事实 | 生产状态 |
| --- | --- | --- | --- |
| 运行时 CRUD | [`node/api/runtime_routes.go`](../../../node/api/runtime_routes.go) | 已实现 `search`、详情、创建、`sync`、`operate`、`remark`、`update`、`del` | 单元/契约覆盖；真实容器 `not-run` |
| Docker/Compose 执行 | [`node/api/runtime_execution.go`](../../../node/api/runtime_execution.go) | 参数数组、超时、环境变量和 Compose `config/pull/up/build/down` 路径已存在 | 未连接生产 Docker |
| SQLite 持久化 | [`node/api/runtime_persistence.go`](../../../node/api/runtime_persistence.go) | migration `0009` 至 `0012`，运行时、任务、任务日志、属性表 | 只读计数可核对；真实重启闭环 `not-run` |
| 任务和日志 | [`node/api/runtime_tasks.go`](../../../node/api/runtime_tasks.go) | `runtime_tasks`、`runtime_task_logs` 与统一任务日志同步，重启中断任务标记失败 | 真实任务输出 `not-run` |
| Node 模块 | [`node/api/runtime_node.go`](../../../node/api/runtime_node.go) | package/module 查询和异步 install/uninstall/update | 真实 npm/yarn/pnpm `not-run` |
| PHP 配置/FPM/容器 | [`node/api/runtime_php_routes.go`](../../../node/api/runtime_php_routes.go) | FPM 配置、FastCGI status、容器参数、PHP 文件更新 | 真实 PHP-FPM `not-run` |
| PHP 扩展/Supervisor | [`node/api/runtime_php_templates.go`](../../../node/api/runtime_php_templates.go)、[`node/api/runtime_php_supervisor.go`](../../../node/api/runtime_php_supervisor.go) | 扩展安装/卸载事务、模板 CRUD、Supervisor CRUD/回滚 | 真实镜像和进程 `not-run` |

运行时关系数据必须来自 SQLite。旧 `runtime.json` 只允许按 migration/legacy import 归档，不得作为新的运行态数据源；验收报告不得提交 fixture 文件或凭据。

## 3. HTTP 路由和请求契约

下表是生产执行时必须逐条记录的路由族。同一路径的不同 `operate`、`type`、`file` 或 `operation` 是不同案例，不能用一次请求代表整组通过。

| 方法 | 路径 | 关键请求字段/分支 | 必须记录的响应与副作用 |
| --- | --- | --- | --- |
| POST | `/api/v2/runtimes/search` | `page`、`pageSize`、`type`、`name`、`status` | `data.items/total/page/pageSize`；与 SQLite 记录一致 |
| GET | `/api/v2/runtimes/{id}` | 真实运行时 ID | `data` 的类型、版本、状态、端口、路径、容器和任务字段 |
| GET | `/api/v2/runtimes/installed/delete/check/{id}` | 真实运行时 ID | 网站引用列表；有引用时删除必须返回冲突 |
| POST | `/api/v2/runtimes` | `id/name/type/version/install/codeDir/port/params/exposedPorts/environments/volumes/extraHosts` | 创建响应、SQLite 行、Compose 路径、异步 `taskID` |
| POST | `/api/v2/runtimes/sync` | 空对象或前端同步参数 | Docker 状态同步；不得伪造状态 |
| POST | `/api/v2/runtimes/operate` | `ID`/`id`、`operate=up\|down\|restart` | `code:200,message:success,data:null`；容器实际状态和 SQLite 状态 |
| POST | `/api/v2/runtimes/remark` | `id`、`remark` | SQLite `remark/updated_at` 变化和 Success envelope |
| POST | `/api/v2/runtimes/update` | `id`、变化的版本/端口/参数/环境/卷 | Compose 重建、旧值回滚、SQLite 更新 |
| POST | `/api/v2/runtimes/del` | `id`、`forceDelete`、`deleteImage`、`taskID` | 网站引用检查、容器/目录/镜像清理、SQLite 删除 |
| POST | `/api/v2/runtimes/node/package` | `id`、`codeDir` | 真实 `package.json` 依赖扫描，大小和数量上限 |
| POST | `/api/v2/runtimes/node/modules` | `id` | 容器内模块列表和真实安装状态 |
| POST | `/api/v2/runtimes/node/modules/operate` | `id`、`operate=install\|uninstall\|update`、`pkgManager`、`module` | 真实 npm/yarn/pnpm 命令、任务日志和版本变化 |
| GET | `/api/v2/runtimes/node/tasks/{id}` | Node 任务 ID | SQLite 任务状态、进度、错误和日志尾部 |
| GET | `/api/v2/runtimes/php/{id}/extensions` | PHP 运行时 ID | 容器 `php -m`、`extensions`、`supportExtensions`、状态 |
| POST | `/api/v2/runtimes/php/extensions/install` | `id`、`name/extension`、可选 `taskID` | 安装任务、`.so`/INI/.env、重启结果 |
| POST | `/api/v2/runtimes/php/extensions/uninstall` | `id`、`name/extension` | 文件事务、重启失败回滚和 SQLite 扩展列表 |
| POST | `/api/v2/runtimes/php/extensions/search` | `name/search`、`page/pageSize`、`all` | 真实模板/目录筛选和分页 |
| POST | `/api/v2/runtimes/php/extensions` | `name`、`extensions[]` | SQLite 模板创建和返回对象 |
| POST | `/api/v2/runtimes/php/extensions/update` | `id`、`extensions[]` | SQLite 模板更新和 Success |
| POST | `/api/v2/runtimes/php/extensions/del` | `id` | SQLite 模板删除和 Success |
| GET/POST | `/api/v2/runtimes/php/config/{id}`、`/api/v2/runtimes/php/config` | `uploadMaxSize`、`maxExecutionTime`、`disableFunctions[]` | php.ini 变更、容器重启和旧文件回滚 |
| GET/POST | `/api/v2/runtimes/php/fpm/config/{id}`、`/api/v2/runtimes/php/fpm/config` | `params` 对象 | FPM INI 变更、语法/重启结果 |
| GET | `/api/v2/runtimes/php/fpm/status/{id}` | PHP ID | FastCGI `/status` 实际响应，不得以进程存在代替 |
| GET | `/api/v2/runtimes/php/container/{id}` | PHP ID | 容器名、端口、环境、卷、extraHosts 与 Docker inspect 一致 |
| POST | `/api/v2/runtimes/php/container/update` | `ID/id`、`containerName`、端口/环境/卷 | Compose 更新、冲突检测、Success |
| GET | `/api/v2/runtimes/php/{kind}/file/{id}` | `kind=config\|fpm\|...`、ID | 受限文件内容和路径；敏感值脱敏 |
| POST | `/api/v2/runtimes/php/file`、`/api/v2/runtimes/php/update` | ID、`type`、`content` | 原子写入、配置检查、重启和回滚 |
| GET/POST | `/api/v2/runtimes/supervisor/process/{id}`、`/api/v2/runtimes/supervisor/process` | `name`、`operate=create\|update\|start\|stop\|restart\|delete`、配置字段 | `supervisorctl` 结果、配置、out.log/err.log、SQLite/任务日志 |
| POST | `/api/v2/runtimes/supervisor/process/file` | `id`、`name`、`file=config\|out.log\|err.log`、`operate=get\|clear\|update` | 文件大小/语法限制，应用失败恢复旧配置 |

## 4. 六类运行时真实生命周期

以下每行都是独立验收案例。执行时使用真实隔离资源和真实请求；`<id>`、端口和域名必须由本次创建结果取得，禁止填 `test`、`1` 或虚构路径。

| 类型 | 创建输入重点 | Docker/端口证据 | 日志/任务证据 | 删除/重建验收 |
| --- | --- | --- | --- | --- |
| Go | `type=go`、版本、存在的 `codeDir`、唯一端口、`install=true` | `docker compose config`；`docker inspect` 显示镜像、映射端口、挂载和 running | 创建 `taskID` 在 `runtime_tasks`、`runtime_task_logs` 有 pulling/up/stdout/stderr/TASK-END | 停止、删除后 Compose/目录/容器清理；同名重建产生新任务和新时间戳 |
| Node.js | `type=node`、Node 版本、代码目录、`RUN_INSTALL`、端口 | 容器端口可从 OpenResty 前置实际访问；`package` 与容器依赖目录匹配 | modules 查询和 install/update/uninstall 的真实包管理器输出 | 删除不遗留 `node_modules`/Compose；重建后模块状态由容器重新读取 |
| Python | `type=python`、版本、启动参数、代码目录、端口 | 进程监听端口与 `exposedPorts`/`PANEL_APP_PORT_HTTP` 一致 | 任务日志包含启动命令 stderr；失败状态为 Error 而不是空列表 | 目录、卷、容器清理；重建后 SQLite 参数和实际容器一致 |
| Java | `type=java`、JDK/JAR 参数、代码目录、端口 | JDK/JAR 进程健康、端口映射、内存参数和 Compose 一致 | 启动/重启日志含真实进程输出、退出码和任务完成标记 | 删除镜像按 `deleteImage` 选择；同名重建不可复用旧任务 |
| .NET | `type=dotnet`、运行时版本、发布目录、`ASPNETCORE_URLS`/端口 | 容器内监听与宿主映射；真实 HTTP 200/错误码 | 任务日志记录 publish/start 的 stdout/stderr | 删除后无孤儿容器/目录，重建后健康访问恢复 |
| PHP | `type=php`、`PHP_VERSION`、`PHP_EXTENSIONS`、端口/卷 | `1panel-php-fpm:<PHP_VERSION>` build/up；FastCGI socket/端口和容器状态 | build、扩展、FPM 重启输出进入任务日志；慢日志单独读取 | 删除 PHP-FPM、配置、Supervisor 和目录后同名重建；真实 PHP 页面访问 |

每个类型至少执行 `GET /{id}`、`POST /operate` 的 `down/up/restart` 三个变体、`POST /search`、日志/任务查询、`POST /update` 或 `remark`、`POST /del`，并在每一步后查询 SQLite。状态转换必须真实：`Creating/Building/Running/Stopped/Error`，不得依赖缓存字段。

## 5. Docker、镜像、端口、日志和任务证据

以下命令均为只读，默认在已批准的测试主机执行；本轮未执行 Docker 连接，避免触碰生产容器。命令输出应保存到受保护的测试证据目录，脱敏容器环境中的密码、Token 和私钥。

```bash
# 版本和容器状态（只读）
docker version --format '{{json .Server}}'
docker ps -a --no-trunc --format '{{json .}}'
docker inspect <container> --format '{{json .}}'
docker image inspect <image> --format '{{json .}}'

# Compose 解析、服务和日志（不执行 up/down）
docker compose -f <compose-path> config
docker compose -f <compose-path> ps --all
docker compose -f <compose-path> images
docker compose -f <compose-path> logs --no-color --tail=200 <service>

# 端口、进程和 OpenResty 入口（只读）
ss -lntup
curl --fail-with-body --max-time 10 --resolve <prefix>.cs.sopvip.com:80:<ip> http://<prefix>.cs.sopvip.com/
curl --fail-with-body --max-time 10 --resolve <prefix>.cs.sopvip.com:443:<ip> https://<prefix>.cs.sopvip.com/
```

SQLite 只读检查必须使用 URI `mode=ro` 或数据库工具的只读模式，不复制真实库到仓库：

```bash
sqlite3 'file:/opt/workmesh-server/data/workmesh.db?mode=ro' ".tables"
sqlite3 'file:/opt/workmesh-server/data/workmesh.db?mode=ro' "PRAGMA quick_check; PRAGMA integrity_check; PRAGMA foreign_key_check;"
sqlite3 'file:/opt/workmesh-server/data/workmesh.db?mode=ro' "SELECT id,name,type,status,task_id,task_status,port,updated_at FROM runtime_records ORDER BY id;"
sqlite3 'file:/opt/workmesh-server/data/workmesh.db?mode=ro' "SELECT id,runtime_id,status,step,progress,message,error,updated_at FROM runtime_tasks ORDER BY updated_at DESC LIMIT 100;"
sqlite3 'file:/opt/workmesh-server/data/workmesh.db?mode=ro' "SELECT task_id,line,created_at FROM runtime_task_logs ORDER BY id DESC LIMIT 200;"
```

证据必须同时包含：HTTP 请求时间和状态码、响应 `code/data/message` 摘要、Docker inspect/Compose 输出、监听端口、任务行数和错误字段、运行时目录/配置路径，以及清理后的再次查询结果。

## 6. PHP 专项验收

PHP 只有以下全部通过才可标记 `pass`：

1. `PHP_VERSION` 镜像可拉取或构建，`docker compose config` 无未替换变量，容器状态为 running。
2. FPM 配置 GET/POST 能读写受限 INI，更新后执行 FPM reload/restart；`GET /fpm/status/{id}` 通过 FastCGI 返回真实状态。
3. 扩展安装与卸载各至少一次：`php -m`、`.so`、`conf.d` INI、`php.ini` 和 `.env` 同步；重启失败必须恢复旧文件和旧扩展列表。
4. 扩展模板执行创建、全量搜索（`all=true`）、更新、删除，重启后数据仍来自 SQLite。
5. Supervisor 执行 create/update/start/stop/restart/delete；配置语法错误、`reread` 或 `update` 失败时恢复旧配置，out.log/err.log 可读和清空受限。
6. 通过独立 `*.cs.sopvip.com` 域名访问真实 PHP 页面；停止 FPM 时返回真实上游错误，恢复后返回 200，不能以缓存页面代替。

## 7. 当前阻塞证据

| 项目 | 状态 | 证据/解除条件 |
| --- | --- | --- |
| 代码单元测试 | `通过但不等于生产` | `runtime_lifecycle_test.go`、PHP/Supervisor 定向测试使用注入执行器；未证明真实 Docker |
| Docker 镜像和容器 | `blocked` | 本轮策略明确不连接或改动生产 Docker；需要隔离 Docker daemon、镜像清单和清理授权 |
| 六类生命周期 | `not-run` | 没有每类真实创建、启停、删除、重建的 HTTP、SQLite、Docker 三方证据 |
| PHP-FPM/扩展/Supervisor | `not-run` | 未执行真实 FPM FastCGI、扩展文件事务和 Supervisor 进程 |
| 运行时网站访问 | `blocked` | 需独立 `*.cs.sopvip.com` 前缀、OpenResty/WAF 配置和回滚窗口；不得手工编辑生产配置 |
| 登录后 API | `blocked` | 需管理员会话或 Bearer/API Key；凭据只能由唯一集成负责人安全注入 |
| 全量测试 | `not-run（本任务禁止）` | 本任务不执行全量 Go/race/vet，由主控在源码冻结后统一执行 |

## 8. 交接和执行顺序

1. 主控冻结源码并记录 Go 源码集合 SHA-256、服务二进制、SQLite migration 版本和隔离资源清单。
2. 唯一集成测试负责人修复/确认 HTTP 执行器的“实际发送凭据”和“证据脱敏”分离，准备管理员会话、真实运行时包、Docker 镜像、端口和测试域名。
3. 先执行 Go/Node/Python/Java/.NET，再执行 PHP 专项；每类完成后立即做删除、SQLite 重启恢复和同名重建，避免遗留资源影响下一类。
4. 运行时全部通过后，再执行与网站/OpenResty 的域名联动；不得因为 API 返回 Success 就跳过真实访问。
5. 由主控统一汇总本文件每个案例的证据，更新 [`integration-test-latest.md`](integration-test-latest.md) 和 [`2026-09-07-production-readiness-board.md`](2026-09-07-production-readiness-board.md)；在所有阻塞解除前保持 `[!] 阻塞`，不宣称正式上线。
