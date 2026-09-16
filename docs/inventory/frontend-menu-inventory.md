# 前端主菜单、子菜单、页面与接口映射清单

> 生成时间：2026-09-05T12:53:37.313Z  
> 来源：`web/src/routers/router.ts`、`web/src/routers/modules/*.ts`、路由组件页面和 `docs/inventory/frontend-api-inventory.json`。

## 当前统计

| 项目 | 数量/状态 |
| --- | ---: |
| 主菜单 | 13 |
| 路由节点（含隐藏页面和布局节点） | 109 |
| 唯一页面入口文件 | 94 |
| 前端接口清单总数 | 389 |
| HTTP 接口 | 385 |
| WS 接口 | 4 |
| 页面/路由验收状态 | `not-run: 109` |
| 已映射接口引用状态 | `not-run: 363` |

## 主菜单汇总

| 主菜单 | 路径 | 子路由 | 可见子路由 | 隐藏子路由 | API 模块 | 接口引用 | 验收状态 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| menu.home | `/` | 0 | 0 | 0 | 0 | 0 | not-run |
| menu.advanced | `/advanced` | 9 | 9 | 0 | 3 | 23 | not-run |
| menu.aiTools | `/ai` | 6 | 2 | 4 | 3 | 137 | not-run |
| menu.apps | `/apps` | 5 | 0 | 5 | 2 | 45 | not-run |
| menu.container | `/containers` | 11 | 0 | 11 | 3 | 85 | not-run |
| menu.cronjob | `/cronjobs` | 4 | 0 | 4 | 10 | 194 | not-run |
| menu.database | `/databases` | 11 | 0 | 11 | 3 | 43 | not-run |
| menu.system | `/hosts` | 13 | 3 | 10 | 5 | 93 | not-run |
| menu.logs | `/logs` | 8 | 0 | 8 | 3 | 93 | not-run |
| menu.settings | `/settings` | 9 | 0 | 9 | 5 | 50 | not-run |
| menu.terminal | `/terminal` | 1 | 1 | 0 | 1 | 23 | not-run |
| menu.toolbox | `/toolbox` | 8 | 0 | 8 | 4 | 41 | not-run |
| menu.website | `/websites` | 10 | 2 | 8 | 4 | 125 | not-run |

## 路由、子菜单和页面入口

字段说明：

- `visible` 表示可进入侧边栏菜单；`hidden` 表示详情页、配置页、操作页或兼容入口，不代表功能不存在。
- “接口引用”是静态页面导入和 WS source 匹配结果，状态仍为 `not-run`，不代表真实请求已经通过。
- 完整的请求路径、方法和每个页面的接口引用保存在同目录的 JSON 清单中。

| 层级 | 父路由 | 路由 | 名称 | 可见性 | 页面入口 | API 模块 | 接口引用 | 状态 |
| ---: | --- | --- | --- | --- | --- | --- | ---: | --- |
| 0 | — | `/advanced` | `Advanced-Menu` | visible | `src/views/advanced/website-monitor/index.vue` | — | 0 | not-run |
| 1 | Advanced-Menu | `/advanced/website-monitor` | `AdvancedWebsiteMonitor` | visible | `src/views/advanced/website-monitor/index.vue` | — | 0 | not-run |
| 2 | AdvancedWebsiteMonitor | `/advanced/website-monitor/dashboard` | `AdvancedWebsiteMonitorDashboard` | visible | `src/views/advanced/website-monitor/dashboard/index.vue` | — | 0 | not-run |
| 2 | AdvancedWebsiteMonitor | `/advanced/website-monitor/rank` | `AdvancedWebsiteMonitorRank` | visible | `src/views/advanced/website-monitor/rank/index.vue` | — | 0 | not-run |
| 2 | AdvancedWebsiteMonitor | `/advanced/website-monitor/trend` | `AdvancedWebsiteMonitorTrend` | visible | `src/views/advanced/website-monitor/trend/index.vue` | `website-monitor` | 0 | not-run |
| 2 | AdvancedWebsiteMonitor | `/advanced/website-monitor/log` | `AdvancedWebsiteMonitorLog` | visible | `src/views/advanced/website-monitor/log/index.vue` | `website-monitor` | 0 | not-run |
| 2 | AdvancedWebsiteMonitor | `/advanced/website-monitor/websites` | `AdvancedWebsiteMonitorWebsites` | visible | `src/views/advanced/website-monitor/websites/index.vue` | — | 0 | not-run |
| 2 | AdvancedWebsiteMonitor | `/advanced/website-monitor/setting` | `AdvancedWebsiteMonitorSetting` | visible | `src/views/advanced/website-monitor/setting/index.vue` | — | 0 | not-run |
| 1 | Advanced-Menu | `/advanced/waf` | `AdvancedWaf` | visible | `src/views/advanced/waf/index.vue` | `waf` | 0 | not-run |
| 1 | Advanced-Menu | `/advanced/multi-node` | `AdvancedMultiNode` | visible | `src/views/advanced/multi-node/index.vue` | `setting` | 23 | not-run |
| 0 | — | `/ai` | `AI-Menu` | visible | `src/views/ai/agents/agent/index.vue` | `ai`, `app`, `website` | 137 | not-run |
| 1 | AI-Menu | `/ai/agents/agent` | `Agents` | visible | `src/views/ai/agents/agent/index.vue` | `ai`, `app`, `website` | 137 | not-run |
| 1 | AI-Menu | `/ai/model/account` | `AIModel` | hidden | `src/views/ai/model/index.vue` | — | 0 | not-run |
| 1 | AI-Menu | `/ai/model/local` | `LocalModel` | hidden | `src/views/ai/model/index.vue` | — | 0 | not-run |
| 1 | AI-Menu | `/ai/mcp` | `MCPServer` | visible | `src/views/ai/mcp/server/index.vue` | `ai` | 36 | not-run |
| 1 | AI-Menu | `/ai/gpu/current` | `GPU` | hidden | `src/views/ai/gpu/current/index.vue` | `ai` | 36 | not-run |
| 1 | AI-Menu | `/ai/gpu/history` | `GPUHistory` | hidden | `src/views/ai/gpu/history/index.vue` | `ai` | 36 | not-run |
| 0 | — | `/apps` | `App-Menu` | visible | `src/views/app-store/index.vue` | `app` | 22 | not-run |
| 1 | App-Menu | `/apps` | `App` | hidden | `src/views/app-store/index.vue` | `app` | 22 | not-run |
| 2 | App | `/apps/all` | `AppAll` | hidden | `src/views/app-store/apps/index.vue` | `app` | 22 | not-run |
| 2 | App | `/apps/installed` | `AppInstalled` | hidden | `src/views/app-store/installed/index.vue` | `app`, `setting` | 45 | not-run |
| 2 | App | `/apps/upgrade` | `AppUpgrade` | hidden | `src/views/app-store/installed/index.vue` | `app`, `setting` | 45 | not-run |
| 2 | App | `/apps/setting` | `AppStoreSetting` | hidden | `src/views/app-store/setting/index.vue` | `app`, `setting` | 45 | not-run |
| 0 | — | `/containers` | `Container-Menu` | visible | `src/views/container/index.vue` | — | 0 | not-run |
| 1 | Container-Menu | `/containers` | `Container` | hidden | `src/views/container/index.vue` | — | 0 | not-run |
| 2 | Container | `/containers/dashboard` | `ContainerDashboard` | hidden | `src/views/container/dashboard/index.vue` | `container`, `setting` | 44 | not-run |
| 2 | Container | `/containers/container` | `ContainerItem` | hidden | `src/views/container/container/index.vue` | `container`, `setting` | 44 | not-run |
| 2 | Container | `/containers/container/operate` | `ContainerCreate` | hidden | `src/views/container/container/operate/index.vue` | `container` | 21 | not-run |
| 2 | Container | `/containers/image` | `Image` | hidden | `src/views/container/image/index.vue` | `container`, `setting` | 44 | not-run |
| 2 | Container | `/containers/network` | `Network` | hidden | `src/views/container/network/index.vue` | `container` | 21 | not-run |
| 2 | Container | `/containers/volume` | `Volume` | hidden | `src/views/container/volume/index.vue` | `container`, `files` | 62 | not-run |
| 2 | Container | `/containers/repo` | `Repo` | hidden | `src/views/container/repo/index.vue` | `container` | 21 | not-run |
| 2 | Container | `/containers/compose` | `Compose` | hidden | `src/views/container/compose/index.vue` | `container`, `setting` | 44 | not-run |
| 2 | Container | `/containers/template` | `ComposeTemplate` | hidden | `src/views/container/template/index.vue` | `container` | 21 | not-run |
| 2 | Container | `/containers/setting` | `ContainerSetting` | hidden | `src/views/container/setting/index.vue` | `container`, `setting` | 44 | not-run |
| 0 | — | `/cronjobs` | `Cronjob-Menu` | visible | `src/views/cronjob/index.vue` | — | 0 | not-run |
| 1 | Cronjob-Menu | `/cronjobs` | `Cronjob` | hidden | `src/views/cronjob/index.vue` | — | 0 | not-run |
| 2 | Cronjob | `/cronjobs/cronjob` | `CronjobItem` | hidden | `src/views/cronjob/cronjob/index.vue` | `cronjob`, `group` | 6 | not-run |
| 2 | Cronjob | `/cronjobs/cronjob/operate` | `CronjobOperate` | hidden | `src/views/cronjob/cronjob/operate/index.vue` | `alert`, `app`, `backup`, `container`, `cronjob`, `database`, `group`, `setting`, `toolbox`, `website` | 194 | not-run |
| 2 | Cronjob | `/cronjobs/library` | `Library` | hidden | `src/views/cronjob/library/index.vue` | `cronjob`, `group`, `setting` | 29 | not-run |
| 0 | — | `/databases` | `Database-Menu` | visible | `src/views/database/index.vue` | — | 0 | not-run |
| 1 | Database-Menu | `/databases` | `Database` | hidden | `src/views/database/index.vue` | — | 0 | not-run |
| 2 | Database | `/databases/mysql` | `MySQL` | hidden | `src/views/database/mysql/index.vue` | `app`, `database` | 39 | not-run |
| 2 | Database | `/databases/mysql/setting/:type/:database` | `MySQL-Setting` | hidden | `src/views/database/mysql/setting/index.vue` | `app`, `database` | 39 | not-run |
| 2 | Database | `/databases/mysql/remote` | `MySQL-Remote` | hidden | `src/views/database/mysql/remote/index.vue` | `database` | 17 | not-run |
| 2 | Database | `/databases/postgresql` | `PostgreSQL` | hidden | `src/views/database/postgresql/index.vue` | `app`, `database` | 39 | not-run |
| 2 | Database | `/databases/postgresql/remote` | `PostgreSQL-Remote` | hidden | `src/views/database/postgresql/remote/index.vue` | `database` | 17 | not-run |
| 2 | Database | `/databases/postgresql/setting/:type/:database` | `PostgreSQL-Setting` | hidden | `src/views/database/postgresql/setting/index.vue` | `app`, `database` | 39 | not-run |
| 2 | Database | `/databases/redis` | `Redis` | hidden | `src/views/database/redis/index.vue` | `app`, `command`, `database` | 43 | not-run |
| 2 | Database | `/databases/redis/remote` | `Redis-Remote` | hidden | `src/views/database/redis/remote/index.vue` | `database` | 17 | not-run |
| 2 | Database | `/databases/mongodb` | `MongoDB` | hidden | `src/views/database/mongodb/index.vue` | `app`, `database` | 39 | not-run |
| 2 | Database | `/databases/mongodb/remote` | `MongoDB-Remote` | hidden | `src/views/database/mongodb/remote/index.vue` | `database` | 17 | not-run |
| 0 | — | `/error` | `404` | hidden | `src/components/error-message/404.vue` | — | 0 | not-run |
| 1 | 404 | `/error/404` | `404` | hidden | `src/components/error-message/404.vue` | — | 0 | not-run |
| 0 | — | `/hosts` | `System-Menu` | visible | `src/views/host/file-management/index.vue` | `files`, `host`, `setting` | 73 | not-run |
| 1 | System-Menu | `/hosts/files` | `File` | visible | `src/views/host/file-management/index.vue` | `files`, `host`, `setting` | 73 | not-run |
| 1 | System-Menu | `/hosts/monitor/monitor` | `Monitorx` | hidden | `src/views/host/monitor/monitor/index.vue` | `host` | 9 | not-run |
| 1 | System-Menu | `/hosts/monitor/setting` | `HostMonitorSetting` | hidden | `src/views/host/monitor/setting/index.vue` | `host` | 9 | not-run |
| 1 | System-Menu | `/hosts/firewall/rules` | `FirewallPort` | hidden | `src/views/host/firewall/rule/index.vue` | `firewall`, `process` | 20 | not-run |
| 1 | System-Menu | `/hosts/firewall/docker` | `FirewallDockerGuard` | hidden | `src/views/host/firewall/docker/index.vue` | `firewall` | 17 | not-run |
| 1 | System-Menu | `/hosts/firewall/forward` | `FirewallForward` | hidden | `src/views/host/firewall/forward/index.vue` | `firewall` | 17 | not-run |
| 1 | System-Menu | `/hosts/firewall/setting` | `FirewallSetting` | hidden | `src/views/host/firewall/setting/index.vue` | `firewall` | 17 | not-run |
| 1 | System-Menu | `/hosts/disk` | `Disk` | visible | `src/views/host/disk-management/disk/index.vue` | `host` | 9 | not-run |
| 1 | System-Menu | `/hosts/process/process` | `Process` | hidden | `src/views/host/process/process/index.vue` | `process` | 3 | not-run |
| 1 | System-Menu | `/hosts/process/network` | `ProcessNetwork` | hidden | `src/views/host/process/network/index.vue` | — | 0 | not-run |
| 1 | System-Menu | `/hosts/ssh/ssh` | `SSH` | visible | `src/views/host/ssh/ssh/index.vue` | `host` | 9 | not-run |
| 1 | System-Menu | `/hosts/ssh/log` | `SSHLog` | hidden | `src/views/host/ssh/log/index.vue` | — | 0 | not-run |
| 1 | System-Menu | `/hosts/ssh/session` | `SSHSession` | hidden | `src/views/host/ssh/session/index.vue` | `process` | 3 | not-run |
| 0 | — | `/logs` | `Log-Menu` | visible | `src/views/log/index.vue` | — | 0 | not-run |
| 1 | Log-Menu | `/logs` | `Log` | hidden | `src/views/log/index.vue` | — | 0 | not-run |
| 2 | Log | `/logs/operation` | `OperationLog` | hidden | `src/views/log/operation/index.vue` | `log` | 5 | not-run |
| 2 | Log | `/logs/login` | `LoginLog` | hidden | `src/views/log/login/index.vue` | `log` | 5 | not-run |
| 2 | Log | `/logs/website` | `WebsiteLog` | hidden | `src/views/log/website/index.vue` | `website` | 79 | not-run |
| 2 | Log | `/logs/system` | `SystemLog` | hidden | `src/views/log/system/index.vue` | `log` | 5 | not-run |
| 2 | Log | `/logs/host` | `HostSystemLog` | hidden | `src/views/log/host-system/index.vue` | `log` | 5 | not-run |
| 2 | Log | `/logs/ssh` | `SSHLog2` | hidden | `src/views/host/ssh/log/log.vue` | `host` | 9 | not-run |
| 2 | Log | `/logs/task` | `Task` | hidden | `src/views/log/task/index.vue` | `log` | 5 | not-run |
| 0 | — | `/settings` | `Setting-Menu` | visible | `src/views/setting/index.vue` | — | 0 | not-run |
| 1 | Setting-Menu | `/settings` | `Setting` | hidden | `src/views/setting/index.vue` | — | 0 | not-run |
| 2 | Setting | `/settings/panel` | `Panel` | hidden | `src/views/setting/panel/index.vue` | `auth`, `setting`, `workmesh` | 43 | not-run |
| 2 | Setting | `/settings/bind` | `GatewayBind` | hidden | `src/views/setting/bind/index.vue` | `auth`, `workmesh` | 20 | not-run |
| 2 | Setting | `/settings/alert` | `Alert` | hidden | `src/views/setting/alert/index.vue` | — | 0 | not-run |
| 2 | Setting | `/settings/backupaccount` | `BackupAccount` | hidden | `src/views/setting/backup-account/index.vue` | `backup` | 4 | not-run |
| 2 | Setting | `/settings/about` | `About` | hidden | `src/views/setting/about/index.vue` | `setting` | 23 | not-run |
| 2 | Setting | `/settings/safe` | `Safe` | hidden | `src/views/setting/safe/index.vue` | `setting` | 23 | not-run |
| 2 | Setting | `/settings/snapshot` | `Snapshot` | hidden | `src/views/setting/snapshot/index.vue` | `backup`, `dashboard`, `setting` | 30 | not-run |
| 2 | Setting | `/settings/expired` | `Expired` | hidden | `src/views/setting/expired.vue` | `auth`, `setting` | 43 | not-run |
| 0 | — | `/terminal` | `Terminal-Menu` | visible | `src/views/terminal/index.vue` | `setting` | 23 | not-run |
| 1 | Terminal-Menu | `/terminal` | `Terminal` | visible | `src/views/terminal/index.vue` | `setting` | 23 | not-run |
| 0 | — | `/toolbox` | `Toolbox-Menu` | visible | `src/views/toolbox/index.vue` | `dashboard` | 3 | not-run |
| 1 | Toolbox-Menu | `/toolbox` | `Toolbox` | hidden | `src/views/toolbox/index.vue` | `dashboard` | 3 | not-run |
| 2 | Toolbox | `/toolbox/device` | `Device` | hidden | `src/views/toolbox/device/index.vue` | `toolbox` | 7 | not-run |
| 2 | Toolbox | `/toolbox/supervisor` | `Supervisor` | hidden | `src/views/toolbox/supervisor/index.vue` | `host-tool` | 8 | not-run |
| 2 | Toolbox | `/toolbox/clam` | `Clam` | hidden | `src/views/toolbox/clam/index.vue` | `toolbox` | 7 | not-run |
| 2 | Toolbox | `/toolbox/clam/setting` | `Clam-Setting` | hidden | `src/views/toolbox/clam/setting/index.vue` | `toolbox` | 7 | not-run |
| 2 | Toolbox | `/toolbox/ftp` | `FTP` | hidden | `src/views/toolbox/ftp/index.vue` | `toolbox` | 7 | not-run |
| 2 | Toolbox | `/toolbox/fail2ban` | `Fail2ban` | hidden | `src/views/toolbox/fail2ban/index.vue` | `toolbox` | 7 | not-run |
| 2 | Toolbox | `/toolbox/clean` | `Clean` | hidden | `src/views/toolbox/clean/index.vue` | `setting`, `toolbox` | 30 | not-run |
| 0 | — | `/websites` | `Website-Menu` | visible | `src/views/website/website/index.vue` | `group`, `website` | 81 | not-run |
| 1 | Website-Menu | `/websites` | `Website` | hidden | `src/views/website/website/index.vue` | `group`, `website` | 81 | not-run |
| 1 | Website-Menu | `/websites/:id/config/:tab` | `WebsiteConfig` | hidden | `src/views/website/website/config/index.vue` | `runtime`, `website` | 102 | not-run |
| 1 | Website-Menu | `/websites/ssl` | `SSL` | visible | `src/views/website/ssl/index.vue` | `website` | 79 | not-run |
| 1 | Website-Menu | `/websites/templates` | `WebsiteTemplate` | visible | `src/views/website/template/index.vue` | `website` | 79 | not-run |
| 1 | Website-Menu | `/websites/runtimes/php` | `PHP` | hidden | `src/views/website/runtime/php/index.vue` | `container`, `runtime` | 44 | not-run |
| 1 | Website-Menu | `/websites/runtimes/node` | `node` | hidden | `src/views/website/runtime/node/index.vue` | `runtime` | 23 | not-run |
| 1 | Website-Menu | `/websites/runtimes/java` | `java` | hidden | `src/views/website/runtime/java/index.vue` | `runtime` | 23 | not-run |
| 1 | Website-Menu | `/websites/runtimes/go` | `go` | hidden | `src/views/website/runtime/go/index.vue` | `runtime` | 23 | not-run |
| 1 | Website-Menu | `/websites/runtimes/python` | `python` | hidden | `src/views/website/runtime/python/index.vue` | `runtime` | 23 | not-run |
| 1 | Website-Menu | `/websites/runtimes/dotnet` | `dotNet` | hidden | `src/views/website/runtime/dotnet/index.vue` | `runtime` | 23 | not-run |

## 路由源文件完整性

以下 SHA-256 用于确认清单来自当前工作区源码；重新修改路由后必须重新生成清单。

| 源文件 | 字节数 | SHA-256 |
| --- | ---: | --- |
| `src/routers/modules/advanced.ts` | 3374 | `9f741e8a739a3478c6790c919cc34760bdba65a7abbc1613064600d380a14aee` |
| `src/routers/modules/ai.ts` | 2333 | `83d0a93583f774f1d3ef1cb8d79267a23c37c67df7247be6189c915557bd5f9e` |
| `src/routers/modules/app-store.ts` | 2636 | `140b63686014a336c87791c984501a5fba4d2a57940f0cab07598f773f44ce4a` |
| `src/routers/modules/container.ts` | 5590 | `6b5d9522ebb1508863d3c0ec76f68caf8061095d239fd144307cf169ed73af3e` |
| `src/routers/modules/cronjob.ts` | 1912 | `f5c1ec3c5add43ef303039c4d1eee2df87b884c03edd9a7dc9f89d033c4e882e` |
| `src/routers/modules/database.ts` | 5768 | `568200690fe43a8a2c2237394149741bc2e4772af9f4767045d3e8f0e15eb5b7` |
| `src/routers/modules/error.ts` | 447 | `c627415832f412c9b83461ad6a6404c2cd3eff65e82b127c66037b345e86a35c` |
| `src/routers/modules/host.ts` | 5566 | `70696d1868f51a0f7057be916614ebbf6f25d35c6c497413265f2dc985e8f382` |
| `src/routers/modules/log.ts` | 3789 | `9c92bd30bf241bf7897215b8ab675ea16863e56bd0c788c880d48047cea16f1d` |
| `src/routers/modules/setting.ts` | 4824 | `a5c18e7dfa96ee46eb3fe0e6a2321a350d0d1e462c1677392c0fa5af79ca24e2` |
| `src/routers/modules/terminal.ts` | 638 | `fdf4e0bded943e9fb0506649db988ef46c02ca651aa5719dcd29ac918e9291aa` |
| `src/routers/modules/toolbox.ts` | 3873 | `01c68cf13ce64856bad0c75ca2f76cede54fe9cc4429c8dd49fc00ed518abdef` |
| `src/routers/modules/website.ts` | 4114 | `adaad5455cb378df221802077ba9944b1665ce2a7dc980764bfdfc0b9a87c422` |
| `src/routers/router.ts` | 2745 | `c7543158962409b02c6593e396a811a1f191494bd81a34a8c84a80ed4c858c85` |

## 验收边界

- 本清单只负责菜单、子菜单、页面入口和接口引用的静态盘点，不把静态发现当作功能测试结果。
- HTTP、WS、下载、上传、SSE、终端 PTY、同一路由的 `type/operate/logType/source/scope` 变体仍需使用真实 SQLite 和真实服务逐项测试。
- 页面权限、动态隐藏菜单、Gateway/主次节点、弹窗操作和共享组件中的间接 API 调用，需要登录后浏览器或黑盒测试补充证据。
- 任何测试结果只能写入独立测试矩阵；未经实际请求证据不得把 `not-run` 修改为 `pass`。
