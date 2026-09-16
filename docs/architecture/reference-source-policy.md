<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 业务参考源与废弃代码边界

## 正式决策

`/www/apps/1Panel` 是 WorkMesh Server 当前唯一的业务参考源。它只读，用于核对：

- 前端菜单、页面操作和 API/WS 调用；
- Core/Agent 路由、请求参数、响应 envelope、错误码和多分支 `type`/`operate` 行为；
- 鉴权、中间件、日志、计划任务、初始化和语言包等隐藏能力；
- 网站、OpenResty/WAF、SSL/ACME、运行环境、数据库、容器、文件和终端的业务语义。

参考源不参与 WorkMesh Server 的运行、编译、部署或数据读写。所有实现、测试、迁移记录和发布制品只属于 `apps/workmesh-server`。

## `apps/workmesh-node` 处理方式

`apps/workmesh-node` 是此前的过渡实现，已完成的 OpenResty/WAF 移植不再作为业务基准。它不再用于：

- 生成或校验路由清单；
- 推导 API/WS 参数和返回结构；
- 复制业务实现、菜单或语言包；
- 编译、部署或启动 WorkMesh Server。

历史文档中保留的 `workmesh-node` 路径只表示当时的迁移证据，不代表当前依赖。旧 systemd 单元和卸载脚本可以作为历史清理工具保留，但不得被新服务启动流程引用。

## 现行校验入口

路由契约、前端接口清单和实现扫描均必须显式指向 1Panel：

```bash
node test/contract/route-scan.mjs generate \
  --legacy /www/apps/1Panel \
  --out test/contract/routes.json

node test/contract/route-scan.mjs check \
  --legacy /www/apps/1Panel \
  --project . \
  --manifest test/contract/routes.json

node test/contract/implementation-scan.mjs \
  --legacy /www/apps/1Panel \
  --project . \
  --manifest test/contract/routes.json
```

当前契约快照为 759 条路由，旧 `/api/v1` 仅保留在迁移字段中，运行时和前端统一使用 `/api/v2`。

## 实现原则

复刻的是 1Panel 的可观察业务行为，不复制其目录结构或私有实现。WorkMesh Server 继续保持单进程、SQLite 唯一正式数据源、真实文件和真实 OpenResty/WAF；HTTP/WS transport、application、domain、infrastructure 的边界按洋葱架构逐步收敛。

