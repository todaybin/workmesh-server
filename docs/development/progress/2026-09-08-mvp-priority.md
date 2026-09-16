<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 首版可上线优先级（网站、容器、数据库）

## 首版目标

用户能够通过前端面板使用 `/api/v2` 管理三类基础资源：网站、容器、数据库。运行态数据必须来自真实 SQLite；外部 OpenResty、Docker、数据库命令失败时返回真实错误并回滚，不允许固定成功、空列表或 JSON 模拟数据。

## P0 网站

1. 网站 CRUD：创建、详情、列表、更新、删除，主域名/附加域名全局冲突校验。
2. 网站类型：静态、反向代理、运行环境、子网站；TCP/UDP 作为 P1，必须明确 not-run 或真实验收。
3. 配置：目录、默认文档、代理、伪静态、重定向、CORS、真实 IP、限流、密码访问、WAF。
4. OpenResty：生成配置、`nginx -t`/`openresty -t`、失败清理、删除清理；不以配置文件存在代替校验。
5. HTTPS：证书查询、上传、绑定；Let's Encrypt HTTP-01 在具备隔离域名后执行，失败保留旧证书。

## P0 容器

1. Docker 状态和 Compose 列表真实读取。
2. Compose 配置检查、创建/更新、启动/停止/重启、删除，任务状态写 SQLite。
3. 容器详情、日志、统计、网络/卷只在 Docker daemon 可用时执行；daemon 不可用返回明确错误。
4. 镜像拉取、构建、推送、文件操作属于 P1，不阻塞首版静态管理页面，但不得由 fallback 冒充成功。

## P0 数据库

1. 数据库列表、详情、状态、创建/删除/检查必须接入真实数据库服务或明确返回服务不可用。
2. 用户、密码、授权、变量、描述使用 SQLite 持久化并保持前端字段契约。
   当前数据库创建接口已完成真实 SQLite 登记与连接参数校验；MySQL/MariaDB 服务端 `CREATE DATABASE` 需在目标服务凭据可用时由数据库执行器完成，不能把登记成功视为远程实例已创建。
3. MySQL/MariaDB 先作为首版目标；PostgreSQL、MongoDB、Redis 保持独立 P1 任务，不得共用错误处理器伪造成功。
4. 数据库备份/恢复、远程数据库和配置文件更新在 P1 执行，但路由必须返回明确状态。

## 首版验收门槛

- 三个领域各自完成至少一条真实成功生命周期和一条外部失败回滚测试。
- 前端实际调用的请求方法、字段、响应 envelope 与 `/api/v2` 契约一致。
- SQLite 重启后数据可恢复；无 `apps.json` 或其他运行态 JSON 数据源。
- `GOWORK=off go test ./...`、race、vet、Node 契约和 `git diff --check` 全部通过。
- 没有 Docker、OpenResty、数据库服务或域名资源时，报告必须标记 `not-run`，不能宣称上线。

## P1 后续

日志审计、AI、运行时六语言、主次 Gateway、SSH/KVM、ACME 续期、镜像高级操作、PostgreSQL/MongoDB/Redis、升级回滚在首版三类资源可用后逐项迭代。
