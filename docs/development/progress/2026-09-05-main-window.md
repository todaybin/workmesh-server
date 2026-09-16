<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 主窗口菜单与页面验收进度（2026-09-05）

状态：`[>]` 进行中。清单来自 `web/src/routers/modules/*.ts` 的真实源码，页面和接口状态默认是 `not-run`，不代表已经完成浏览器验收。

## 当前统计

| 项目 | 数量 | 证据 |
| --- | ---: | --- |
| 主菜单 | 13 | [`frontend-menu-inventory.json`](../../inventory/frontend-menu-inventory.json) |
| 页面/路由入口 | 109 | 同上 |
| 唯一页面入口文件 | 94 | [`frontend-menu-inventory.md`](../../inventory/frontend-menu-inventory.md) |
| 前端 HTTP/WS 调用 | 389 | [`frontend-api-inventory.json`](../../inventory/frontend-api-inventory.json) |
| 1Panel HTTP 基线 | 759 | [`route-inventory-1panel.json`](../../inventory/route-inventory-1panel.json) |

## 主菜单清单

| 主菜单 | 路径 | 子页面/路由数量 | 当前状态 |
| --- | --- | ---: | --- |
| 概览 | `/` | 1 | not-run |
| 应用商店 | `/apps` | 5 | not-run |
| 网站 | `/websites` | 10 | not-run |
| AI | `/ai` | 6 | not-run |
| 数据库 | `/databases` | 11 | not-run |
| 容器 | `/containers` | 11 | not-run |
| 系统 | `/hosts` | 13 | not-run |
| 终端 | `/terminal` | 1 | not-run |
| 计划任务 | `/cronjobs` | 4 | not-run |
| 工具箱 | `/toolbox` | 8 | not-run |
| 高级功能 | `/advanced` | 9 | not-run |
| 日志审计 | `/logs` | 8 | not-run |
| 面板设置 | `/settings` | 9 | not-run |

## 页面验收顺序

1. 概览：节点状态、CPU/内存/磁盘、快捷入口和最近任务。
2. 应用商店：目录、已安装、升级、设置、安装任务。
3. 网站：站点列表、创建、域名、SSL、运行环境、模板和六类网站类型。
4. AI：Agent、模型、MCP、GPU 当前状态和历史。
5. 数据库：MySQL、PostgreSQL、Redis、MongoDB、远程连接、配置和备份。
6. 容器：概览、容器、镜像、网络、卷、仓库、Compose、模板和设置。
7. 系统：文件、监控、防火墙、磁盘、进程、SSH 和主机日志。
8. 终端：终端连接、命令库、终端设置和 WS 释放。
9. 计划任务：任务 CRUD、运行记录、导入导出、备份任务和任务日志。
10. 工具箱：设备、Supervisor、ClamAV、FTP、Fail2Ban、磁盘清理。
11. 高级功能：网站监控、WAF、多节点。
12. 日志审计：操作、登录、网站、系统、主机、SSH、任务日志。
13. 面板设置：面板、Gateway、告警、备份账号、关于、安全、快照和过期设置。

## 完成判定

每个菜单页面只有同时满足以下条件，才能标记为 `pass`：

- 页面在真实安全入口和有效登录 Session 下可以打开。
- 页面所有 HTTP/WS 请求路径、方法、参数和响应字段与 1Panel 前端契约一致。
- 同一路由的 `type`、`operate`、`logType`、`source`、`scope` 等分支均有真实请求证据。
- 创建、更新、删除、启停和重启操作写入真实 SQLite，并能在服务重启后恢复。
- 任务、操作日志、访问日志、系统日志、登录日志、主机日志和网站日志可查询。
- 失败分支、权限错误、资源不存在和外部依赖不可用时返回真实错误，不能返回固定成功或模拟空数据。
- WS/流式页面验证握手、鉴权、消息、关闭、重连和资源释放。

当前主窗口进度不是“没有内容”，而是已经从源码提取出 13 个主菜单和 109 个入口；下一步是获取有效登录凭据后逐菜单执行真实浏览器/API/WS 验收，并回写每一项状态。
