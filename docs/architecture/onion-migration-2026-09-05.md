<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh Server 洋葱架构迁移说明

## 为什么当前目录看起来没有整体变化

本轮先完成了不改变 import 路径和生产启动方式的安全重构：运行时、网站服务、功能域和 Supervisor 已按职责拆分，但仍保留在原有 `node/api`、`node/service`、`runtime` 包中。因此文件数量和单文件规模发生了变化，顶层目录名称暂时没有大搬迁。

原因是当前项目存在 759 条 HTTP 路由、389 条前端 HTTP/WS 调用、生产 systemd 服务和真实 SQLite 数据。如果先移动所有包，再补测试，容易出现路由注册丢失、循环依赖、生产部署路径变化和旧接口无法对应。目录迁移必须以“先建立边界、再迁移包、每批回归”为顺序。

## 当前实际分层

```text
cmd/workmesh-server       启动、CLI、静态文件入口
config                    启动配置和资源限制
control/api               控制面 HTTP、认证、主次节点、Gateway
control/service           控制面业务和 SQLite 状态
node/api                  Agent/主机业务 HTTP 与兼容路由
node/model                领域数据结构
node/service              网站、SSL、任务、数据库等领域服务
runtime/*                 跨模块基础能力：HTTP、日志、Gateway、链路、角色、调度、存储
internal/storage          SQLite、迁移、旧数据导入
internal/testenv          隔离测试环境
i18n                      语言包
web                       前端和构建产物
```

## 目标洋葱架构

```text
cmd/workmesh-server
└── 仅负责启动、信号、CLI 和依赖组装

internal/
├── domain/                纯领域模型和规则，不依赖 HTTP、Docker、SQLite
│   ├── website
│   ├── runtime
│   ├── database
│   ├── container
│   ├── task
│   ├── audit
│   └── node
├── application/           用例编排、事务、任务生命周期
│   ├── website
│   ├── runtime
│   ├── task
│   ├── update
│   └── auth
├── infrastructure/        外部资源适配器
│   ├── sqlite
│   ├── docker
│   ├── openresty
│   ├── acme
│   ├── filesystem
│   ├── gateway
│   └── logging
└── transport/              协议层，不包含业务规则
    ├── http/control
    ├── http/node
    ├── ws/terminal
    ├── ws/process
    ├── sse
    └── middleware

control/                    迁移完成后仅保留控制面适配器别名
node/                       迁移完成后仅保留 Agent 适配器别名
runtime/                    迁移完成后仅保留兼容公共基础包
i18n/
web/
```

## 迁移顺序

| 阶段 | 范围 | 方式 | 验收门槛 |
| --- | --- | --- | --- |
| A | 运行时、网站、功能域拆分 | 已完成，暂不改 import 路径 | `go test ./...`、`go vet ./...` |
| B | SQLite、日志、任务、更新、鉴权接口抽象 | 从现有实现提取 interface 和事务边界 | 真实 SQLite 重启恢复、任务日志和迁移测试 |
| C | HTTP/WS transport 与 application 分离 | 先新增适配器，再逐路由切换 | 路由清单 759 条不减少，WS 握手/释放测试通过 |
| D | domain 与 infrastructure 迁移 | 按网站、运行时、数据库、容器、日志批次移动 | 每批 route scan、接口回归和 OpenResty 验证 |
| E | 删除兼容壳 | 仅在前端和旧调用方完成 v2 迁移后删除 | 无旧 import、无 v1 运行调用、发布回滚验证 |

## 当前不允许直接做的操作

- 不把所有 `node/api` 一次性移动到新目录；这会同时改变路由注册、测试包和 import 图。
- 不删除 `control`、`node`、`runtime` 兼容包；它们目前仍是生产入口。
- 不把 SQLite、Docker、OpenResty、ACME 调用放进 domain 包。
- 不用目录迁移掩盖尚未完成的登录后业务和 WS 测试。
- 不修改生产网站根目录、WAF 自定义路径和 systemd 工作目录。

## 下一批实际改动

1. 继续迁移 `node/api/runtime_*`、`node/service/website_*` 及剩余 control/application 运行期 SQLite 访问，不改变 HTTP 路径。
2. 将旧 `website_extension_state` BLOB、启动导入和 migration 回调明确隔离到 infrastructure/migration 包，保留可重试兼容行为。
3. 在可访问 Docker/OpenResty/域名/节点凭据的环境执行 HTTP/HTTPS Host 路由、WAF 实际拦截、ACME、Gateway 联调和数据库 Compose 验收。
4. 执行制品备份、发布、失败回滚和重启后 SQLite/WAF/OpenResty 状态验收。
5. 上述边界和外部验收稳定后，再分批移动物理目录；每批执行全量 Go、前端、路由契约、SQLite 和 OpenResty 回归。

## 2026-09-11 执行状态

- [x] 新增 `SQLExecutor` 和 `Transactional` 最小接口，保留 `*sql.DB` 兼容出口。
- [x] `runtime/link.SQLiteSyncStore` 完成第一批接入，链路同步事务、幂等和冲突语义保持不变。
- [x] `node/api` 日志查询、详情和保留清理完成第一批接口接入，分页、脱敏和清理策略保持不变。
- [x] 登录、HTTP 操作审计以及应用/运行时任务状态和输出完成首批 writer 接入。
- [x] 新增 SQLite repository 提交/回滚和链路同步接入测试。
- [x] 网站/WAF/系统/SSH 文件日志已统一只读 source 接口，任务后台写入错误边界已补齐；剩余是领域策略和外部轮转依赖。
- [ ] website 和 runtime 业务仍未完成完整接口迁移。
- [ ] 真实域名/ACME、生产 OpenResty、Docker Compose 和生产发布回滚仍按 `not-run` 管理。

## 2026-09-12 目录与边界复核

- [x] `internal/storage` 的 `SQLExecutor`/`Transactional` 边界已覆盖 SQLite、日志、任务、网站、WAF、SSL、DNS、数据库、主机、角色、Gateway、Core、脚本库和备份元数据的主要运行期路径。
- [x] Core 用户/Passkey 的运行期保存已改为 repository 事务；旧 JSON、旧 BLOB、启动建表和 schema migration 仍明确留在兼容/基础设施边界。
- [x] 目录物理结构暂不整体搬迁，继续保持 `control`、`node`、`runtime` 作为 transport/compatibility 层；当前阶段优先验证接口边界、路由契约和真实资源行为。
- [ ] website/runtime 剩余资源状态、旧扩展 BLOB 兼容层和部分日志/导入路径仍需进一步拆分；完成前不删除旧包或改变生产工作目录。
- [ ] 真实生产 OpenResty、域名/ACME、Docker Compose 和发布回滚仍需外部权限，不能由本地编译和静态目录检查替代。
