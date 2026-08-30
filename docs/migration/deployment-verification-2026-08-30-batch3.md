<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-08-30 第三批迁移验收

## 本批制品

- 后端 Linux amd64 制品 SHA256：`0AB60D16FCC7DCDC0588D738488AA774CA5BEB3F65B0C7EA6D08B9CFF777FE88`
- 主要提交：`ee68942`（AI/应用目录）、`a071fbb`（设置快照与扫描器）、`0dd1089`（隐藏路由、统计接口、WebSocket 与逐路由清单）、`326be8b`（运行状态目录忽略）。
- 旧 Core/Agent 全源码路由基线：831 条；当前静态状态：`implemented 463`、`partial 230`、`compatibility 138`、`pending 0`、`missing 0`。
- 逐接口清单：[`function-checklist-generated.md`](./function-checklist-generated.md)。

## 自动化验收

```text
go test ./...       通过
go vet ./...        通过
route-scan          831/831 通过（允许 62 条扩展路由）
implementation-scan pending=0, missing=0
Vite build:pro      已生成 web/dist/index.html
```

## 部署状态

本批构建制品尚未替换线上主/次节点：使用 `config/remote-staging.env` 中的临时 SSH 凭据连接 `61.184.12.165:52834` 时，远端返回 `Permission denied (publickey,password)`。未继续重试，也未修改远端文件。

获得有效 SSH 凭据后，按以下顺序执行：

1. 上传制品到 `/tmp/workmesh-server-linux-amd64`。
2. 备份 `/opt/workmesh-server/bin/workmesh-server`（或次节点对应路径）。
3. 原子安装并重启 `workmesh-server.service`。
4. 检查 `systemctl is-active`、`/health`、`/ready`，再执行节点、容器、计划任务和 Gateway 联调。

部署脚本必须保留备份路径和 SHA256；配置、数据目录和用户上传资源不得被制品覆盖。

