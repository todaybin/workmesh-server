<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh Server

统一架构说明见 [`docs/architecture/unified-server.md`](docs/architecture/unified-server.md)。当前主项目只有 `/www/apps/workmesh-server`，前端和后端都在本仓库内；业务行为只参考只读 `/www/apps/1Panel`。业务参考源与废弃代码边界见 [`docs/architecture/reference-source-policy.md`](docs/architecture/reference-source-policy.md)。control 与 node 是同一进程内的业务分区，共享一个 SQLite、认证上下文和生命周期。单二进制内置前端的目标、构建和生产验收见 [`docs/architecture/single-binary-frontend-release.md`](docs/architecture/single-binary-frontend-release.md)。

WorkMesh Server 是面向单机和多节点环境的自主运行服务，提供主机、容器、网站、数据库、文件、备份、计划任务、终端、日志、SSL、运行时和 AI 工作流管理能力。

## 运行

在本目录执行：

```powershell
go run ./cmd/workmesh-server
```

默认监听地址由环境变量 `WORKMESH_SERVER_ADDR` 控制，默认端口为 `9999`。健康检查为 `/health`，依赖就绪检查为 `/ready`。

开发环境如果需要访问管理端页面，先执行 `make build-frontend` 或
`npm --prefix web run build:pro`，再运行 Go 服务。生产构建与 1Panel
一致：前端从 `web` 构建到 `internal/webassets/dist`，由
`internal/webassets/embed.go` 通过 `go:embed` 嵌入 Go 二进制。生产运行不
依赖外置 `web/dist`。

## 运行数据

节点数据目录由 `WORKMESH_DATA_DIR` 指定。正式业务状态统一写入该目录下的 SQLite 数据库
`workmesh.db`，数据库迁移由启动流程按版本执行并记录在 `schema_migrations`；长内容和运行
文件仍使用受控目录下的真实文件、临时文件和原子 rename。历史 JSON 文件只允许在一次性导入
流程中读取，导入成功后不再作为运行时数据源，服务不要求额外启动数据库进程。

敏感配置只从环境变量或部署系统注入，不写入源码、示例配置或 Git。长任务必须使用各功能域的异步任务接口；尚未具备取消或重启恢复能力的功能必须在迁移清单中明确记录。

## API 与前端

HTTP API 统一使用 `/api/v2`，成功响应使用数字 `code: 200`，错误响应使用 `code: "ERR"` 和结构化错误详情。当前前端源码位于 `/www/apps/workmesh-server/web`，构建产物由同一进程提供静态资源和 history 路由。

所有非公开 API 统一经过本地 Session、Bearer、API Key 或节点签名鉴权。写接口按功能域补充参数、资源归属和幂等校验；文件操作使用路径穿越防护、上传上限和原子写入，命令执行使用白名单与超时。

构建和安装脚本接口、单二进制发布契约以及目标机验收要求见
[`deploy/install/README.md`](deploy/install/README.md) 和
[`docs/architecture/single-binary-frontend-release.md`](docs/architecture/single-binary-frontend-release.md)。
标准生产构建命令是：

```bash
cd /www/apps/workmesh-server
make clean-frontend
make build-release
```

该命令先构建前端，再编译包含前端资源的 Go 二进制；默认产物为
`.build/workmesh-server`。`make clean-frontend` 用于生成干净候选制品，避免
保留旧的 hashed assets。部署到目标机后仍需单独完成 systemd、域名、WAF
实际生效和浏览器页面验收，构建成功不等于线上验收完成。

## 节点角色与网关

节点角色通过 `WORKMESH_NODE_ROLE` 配置为 `primary` 或 `secondary`，节点标识使用 `WORKMESH_NODE_ID`。网关地址、节点标识和授权信息由部署配置提供，服务不会内置或生成任何凭据。普通站点与网关控制面使用独立域名、配置和数据库。

## 开发规范

- 新增 Go 导出符号必须包含中文注释。
- 新增代码文件使用 GPL-3.0 SPDX 头部。
- 列表接口必须分页或设置明确上限。
- 外部命令和网络请求必须设置超时。
- 修改公开接口时同步更新 API 文档、测试和功能清单。

## 验证

```powershell
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./..."
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go vet ./..."
```

完整接口、隐藏功能和部署验收记录位于 `docs/migration` 与 `docs/operations`。
