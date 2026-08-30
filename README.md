<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh Server

WorkMesh Server 是面向单机和多节点环境的自主运行服务，提供主机、容器、网站、数据库、文件、备份、计划任务、终端、日志、SSL、运行时和 AI 工作流管理能力。

## 运行

在本目录执行：

```powershell
go run ./cmd/workmesh-server
```

默认监听地址由环境变量 `WORKMESH_LISTEN_ADDR` 控制，默认端口为 `9999`。健康检查为 `/health`，依赖就绪检查为 `/ready`。

## 运行数据

节点数据目录由 `WORKMESH_DATA_DIR` 指定。服务使用嵌入式 SQLite WAL 保存任务、脚本、命令、站点、证书、备份、数据库实例、审计日志和登录会话，不要求额外启动数据库进程。

敏感配置只从环境变量或部署系统注入，不写入源码、示例配置或 Git。所有长任务都支持状态查询、超时、取消和重启恢复。

## API 与前端

HTTP API 统一使用 `/api/v2`，成功响应使用数字 `code: 200`，错误响应使用 `code: "ERR"` 和结构化错误详情。Web 前端位于 `web`，构建产物由同一进程提供静态资源和 history 路由。

所有接口都经过参数校验、资源归属校验和权限校验。文件操作使用路径穿越防护、上传上限和原子写入；命令执行使用白名单、超时和审计记录。

## 节点角色与网关

节点角色通过 `WORKMESH_ROLE` 配置为 `primary` 或 `secondary`。网关地址、节点标识和授权信息由部署配置提供，服务不会内置或生成任何凭据。普通站点与网关控制面使用独立域名、配置和数据库。

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
