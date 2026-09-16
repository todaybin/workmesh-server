<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# HTTP/WS v2 契约差异审计（2026-09-06）

状态：`[>]` 静态审计完成，真实业务验收未完成

## 1. 审计范围和证据

本次只读比较：

- 参考前端：`/www/apps/1Panel/frontend/src`；
- 正式前端：`/www/apps/workmesh-server/web/src`；
- 比较范围：`api/modules`、`api/interface`、路由模块、终端组件、进程 WS 组件；
- 不修改参考项目，不发送登录后业务请求，不改变当前后端。

静态清单命令及结果：

```text
node test/contract/frontend-inventory.mjs \
  --frontend /www/apps/1Panel/frontend \
  --out /tmp/workmesh-ref-api.json
# 389 条调用，770 个源码文件

node test/contract/frontend-inventory.mjs \
  --frontend /www/apps/workmesh-server/web \
  --out /tmp/workmesh-current-api.json
# 393 条调用，782 个源码文件

node test/contract/frontend-menu-inventory.mjs \
  --frontend /www/apps/1Panel/frontend/src \
  --out /tmp/workmesh-ref-menu.json
# 12 个主菜单，99 个路由

node test/contract/frontend-menu-inventory.mjs \
  --frontend /www/apps/workmesh-server/web/src \
  --out /tmp/workmesh-current-menu.json
# 13 个主菜单，109 个路由
```

说明：清单是静态字符串提取结果，不代表接口已经可用；动态拼接、权限、运行时资源和真实 HTTP/WS 状态必须由唯一集成测试负责人验收。

## 2. 总体差异结论

### 2.1 菜单差异

当前正式前端相对于 1Panel 增加了 `Advanced-Menu`（`/advanced`），包含多节点、WAF 和网站监控页面；参考前端的 `/settings/license` 在当前正式菜单中不再出现，替换为 `/settings/bind` Gateway 绑定页。

| 范围 | 参考 | 当前 | 结论 |
| --- | ---: | ---: | --- |
| 主菜单 | 12 | 13 | 当前新增高级功能菜单 |
| 路由入口 | 99 | 109 | 新增高级功能和 Gateway 绑定相关页面 |
| 参考独有页面 | `/settings/license` | — | 许可证 UI 被移除，需确认是否为有意的产品策略 |
| 当前新增页面 | — | `/advanced/*`、`/settings/bind` | 后端能力和权限需单独验收 |

当前新增页面清单：

```text
/advanced
/advanced/multi-node
/advanced/waf
/advanced/website-monitor
/advanced/website-monitor/dashboard
/advanced/website-monitor/log
/advanced/website-monitor/rank
/advanced/website-monitor/setting
/advanced/website-monitor/trend
/advanced/website-monitor/websites
/settings/bind
```

### 2.2 API 路由差异

静态提取的参考前端有 389 条调用，当前正式前端有 393 条。按方法和规范化路径比较：

| 类别 | 路径/数量 | 说明 |
| --- | --- | --- |
| 网站路径缺失 | 0 | `/api/v2/websites`、`/api/v2/openresty` 及网站相关 WS endpoint 在规范化后均存在 |
| 当前新增 WAF | 7 | `/api/v2/websites/waf/status`、`standard-rules`、`sites`、`sites/:id/rules`、`rules`、`access-lists`、`test` |
| 当前新增 Gateway/节点 | 5 | `/api/v2/workmesh/gateway/{status,login,register,unbind}` 和 `capabilities/route` |
| 当前新增节点角色 | 2 | `/api/v2/core/nodes/add`、`/api/v2/core/nodes/role` |
| 当前提取出的动态容器日志路径 | 1 | `/api/v2/containers/search/log?:param`，需人工确认具体 query 组合 |
| 参考独有许可证路径 | 7 | enterprise/master/SMS/license status 等接口在当前正式 API 模块中被删除 |
| 参考独有防火墙调用形态 | 3 | `forward/enable`、`filter/operate`、`docker/operate` 的旧请求变体提取不到当前同样的参数形态 |

由于提取器会把 URL query 拼接归一化，容器日志的两个旧 query 组合和当前的 `?:param` 不是可靠的业务缺失结论，必须结合调用页面和后端路由实测确认。

## 3. 网站 HTTP 契约差异

### 3.1 网站 API 路径和方法

对比文件：

```text
/www/apps/1Panel/frontend/src/api/modules/website.ts
/www/apps/workmesh-server/web/src/api/modules/website.ts
```

两者共有的网站 API 导出函数和路径基本一致；静态差异只有：

| 函数/字段 | 参考前端 | 当前正式前端 | 风险和验收 |
| --- | --- | --- | --- |
| `getWebsite` 参数 | `id: number` | `id: number \| string` | 当前是类型放宽，不改变 URL；验收数字和字符串 ID 都能查询同一记录 |
| 网站详情 `runtimeID` | `number` | `string \| undefined` | 当前适配运行时 ID 字符串化/缺省；验收响应真实字段和运行时详情关联 |
| `WebSiteCreateReq` | 固定 `appInstallId`、`webSiteGroupId` 等 | 增加 `appInstallID`、`appID`、`appInstall`、`appinstall`、`runtimeID`、`runtimeType` 可选兼容字段 | 属于请求兼容扩展；服务端必须确定优先级，不能因别名产生重复安装或错误类型 |
| `PHPVersionChange.runtimeID` | `number` | `string` | 可能影响后端数字解析；需用真实 PHP 运行时 ID 做切换闭环 |
| `WebsiteDatabase.databaseName` | `number` | `string` | 当前修正为数据库名称文本；需核对列表和绑定页面的 JSON 类型 |

网站 API 的成功响应泛型、相对路径和 HTTP 方法未发现静态变化；`web/src/api/index.ts` 仍以当前 origin 的 `/api/v2` 为 base path，并保持 `{ code: 200, data: ... }` envelope 解包。

以下重点变体必须由真实测试逐项覆盖：

```text
POST /api/v2/websites/operate       { id, operate: start|stop|restart }
POST /api/v2/websites/config        { websiteId, scope, operate, params? }
POST /api/v2/websites/log/search    { id, logType: access.log|error.log, page, pageSize }
POST /api/v2/websites/log/operate   { id, logType, operate: enable|disable|delete }
POST /api/v2/websites/proxies/update { operate: create|edit|delete, ... }
POST /api/v2/websites/redirect/update { operate: create|edit|delete|enable|disable, ... }
POST /api/v2/websites/{id}/https    { enable, websiteSSLId, httpConfig, SSLProtocol, ... }
POST /api/v2/websites/stream/update { streamPorts, udp, algorithm, servers }
```

网站完整的字段、页面和响应矩阵见 [`../../api/website-contract-matrix.md`](../../api/website-contract-matrix.md)。

### 3.2 当前正式前端新增 WAF API

当前 `web/src/api/modules/waf.ts` 新增：

| 函数 | 当前路径 | 响应类型 | 静态审计结论 |
| --- | --- | --- | --- |
| `getWafStatus` | GET `/api/v2/websites/waf/status` | `WafStatus` | 与后端专用 WAF 路由一致，未真实请求 |
| `updateWafGlobal` | POST `/api/v2/websites/waf/global` | 操作结果 | 请求字段为 `enabled/standardRules/mode/paranoiaLevel/inboundThreshold/requestBodyLimit`，需验收 SQLite |
| `listWafStandardRules` | GET `/api/v2/websites/waf/standard-rules` | `WafStandardRule[]` | 后端有同路径专用路由，未真实请求 |
| `testWafRules` | POST `/api/v2/websites/waf/test` | `WafTestResult` | 需要真实规则和站点，不能使用假请求数据 |
| `listWafSites`/`updateWafSite` | GET/POST `/api/v2/websites/waf/sites` | `WafSiteConfig[]`/操作结果 | 需覆盖站点开关和模式 |
| `listWafRules` | GET `/api/v2/websites/waf/sites/{websiteID}/rules` | `WafRule[]` | 需覆盖规则归属和删除 |
| `upsertWafRule`/`deleteWafRule` | POST `/api/v2/websites/waf/rules[/delete]` | `WafRule`/操作结果 | 需校验网站权限、规则字段白名单 |
| `getWafAccessLists`/更新调用 | GET/POST `/api/v2/websites/waf/access-lists` | `WafAccessLists`/操作结果 | 需验证白名单/黑名单真实落盘 |
| `getWafAudit` | POST `/api/v2/xpack/waf/{attack/stat\|block/search\|log/search}` | 分页记录 | 后端已通过兼容别名承接 `/xpack/waf`；未登录边界实测返回统一 `401 LOCAL_AUTH_REQUIRED`，登录后的分页字段、权限和真实 WAF 记录仍待验收 |

`getWafAudit` 的路径别名已完成未登录边界核对，但不能仅凭 `401` 判定审计页可用；仍需管理员会话验证分页字段、WAF 真实记录和权限拒绝分支。

## 4. HTTP 客户端、鉴权和响应差异

当前正式前端的 `web/src/api/index.ts` 相对于参考客户端增加/保持以下策略：

- `baseURL` 通过 `configuredApiPath()` 固定在当前 origin 的 `/api/v2`，即使构建变量是完整 URL，也不让浏览器直连远程节点地址；
- 自动携带 `Accept-Language`、当前节点 `CurrentNode`、登录入口码和 Cookie；写请求从 `pcsrftoken` 注入 `X-CSRF-Token`；
- `web/src/api/gateway.ts` / `client.ts` 使用 `fetch` 调用 Gateway，显式携带 `credentials: include` 并解包 v2 envelope；
- 成功优先读取 `{ code, data }`，Gateway 客户端同时兼容非 envelope 的原始 JSON；这属于客户端容错，不能作为后端改变正式响应格式的理由；
- 当前前端的 API/WS URL 工具阻止浏览器使用内部节点 origin，节点只作为 `operateNode` relay 选择参数。

必须真实验收：未登录 401、权限不足 403、CSRF 缺失 403、错误参数 400、资源不存在 404；不能用前端 catch 后的空状态当作接口成功。

## 5. WS 路由和参数差异

### 5.1 终端 WS

参考和当前共有以下业务 endpoint：

```text
GET/WS /api/v2/core/script/run
GET/WS /api/v2/hosts/terminal/container
GET/WS /api/v2/hosts/terminal/local
GET/WS /api/v2/hosts/terminal/ssh
```

网站运行时终端的脱敏参数仍为：

```text
endpoint=/api/v2/hosts/terminal/container
args=source=container&containerid=<containerID>&user=<user>&command=/bin/bash
```

当前正式前端与参考实现的主要差异：

| 项目 | 参考前端 | 当前正式前端 | 结论 |
| --- | --- | --- | --- |
| URL 构造 | 从页面 URL 直接拼接 `ws/wss://host/api/v2/...` | `buildSameOriginWebSocketUrl()` 使用当前 origin，再以 `operateNode` 交给服务端 relay | 当前更符合浏览器不暴露节点地址的架构；必须验证 relay 实际转发 |
| 节点判断 | 通过参数字符串是否包含 `id=` 判断 local | 解析 `URLSearchParams` 后只检查独立 `id` 字段 | 避免 `containerid` 等字段误判；需覆盖本地/容器/SSH 三种参数 |
| 终端尺寸 | 拼接 `cols`/`rows` | 通过 `URLSearchParams.set` 写入 `cols`/`rows` | 参数名称保持一致，需验证重复 query 不产生两份字段 |
| WS 鉴权 | 直接创建 WebSocket 前由页面流程处理 | 创建前调用 `checkStreamAuth`，然后同源 Upgrade | 需真实验证 Session、Origin 和失败提示 |
| 消息解析 | 直接 `JSON.parse` | 终端主组件捕获非法 JSON，并处理 `error` 消息 | 属于客户端容错；服务端消息 type/data 仍必须兼容 |

### 5.2 其它当前 WS 调用

当前正式前端还统一将下列进程流改为同源 relay URL：

```text
WS /api/v2/process/ws
WS /api/v2/files/wget/process
```

参考前端通过内部地址拼接 `operateNode`；当前通过 `buildSameOriginWebSocketUrl(path, currentNode)`。这不是业务路径删除，但需要确认后端 Upgrade 路由、鉴权和断线清理保持一致。

## 6. 参考独有和当前新增契约

### 6.1 参考许可证接口被当前前端移除

静态差异发现当前 `setting.ts` 删除了以下参考调用：

```text
/api/v2/core/licenses/status
/api/v2/core/licenses/master/status
/api/v2/core/licenses/sms/info
/api/v2/core/licenses/search
/api/v2/core/licenses/upload
/api/v2/core/licenses/update
/api/v2/core/enterprise/licenses/info
/api/v2/core/enterprise/licenses/status
/api/v2/core/enterprise/licenses/community-restore/status
```

其中部分路径在自动清单中只体现为 GET，实际上传/更新为 multipart 或 POST。当前前端不再调用不等于后端可以删除兼容路由；是否保留由 v1/v2 迁移策略和产品授权方案决定。

### 6.2 防火墙任务字段收窄

参考 `firewall.ts` 的部分调用支持可选 `taskID` 和 `postWithConfig`：

```text
forward/enable       { taskID? }
filter/operate       { name, operate, taskID? }
docker/operate       { operation, taskID? }
```

当前正式前端对应方法去掉了 `taskID` 参数，并将部分 `postWithConfig` 改为普通 `post`；`operateFire` 的前端超时由 10 分钟收窄为 60 秒。该差异可能造成长任务超时或任务关联丢失，必须在计划任务/防火墙真实操作中验证，不能仅凭 HTTP 200 判定。

## 7. 未覆盖项和 blocked 原因

本次未执行以下项目：

| 项目 | 状态 | 原因 |
| --- | --- | --- |
| 登录后 HTTP 契约 | `blocked/not-run` | 当前任务无可复用管理员 Session、CSRF Cookie 和权限上下文 |
| 网站六类型生命周期 | `blocked` | 需要 Docker/运行时镜像、OpenResty 及真实上游 |
| 网站域名访问 | `blocked` | 需要 `*.cs.sopvip.com` 前缀解析和公网 80/443 |
| Let's Encrypt HTTP-01 | `blocked` | 需要真实 DNS、80 端口、ACME 账户和 challenge 可达性 |
| WAF 实际拦截 | `blocked` | 需要 OpenResty/WAF 运行时及真实规则 |
| WS Upgrade/消息序列 | `blocked` | 需要有效会话、容器、shell 或 SSH 目标 |
| Gateway 主/次节点 | `blocked` | 需要真实 Gateway URL、注册凭据、次节点网络 |
| SQLite 重启/迁移 | `not-run` | 本次只读前端契约审计，未触碰用户数据库 |
| 全量 Go/Node/race/vet | `not-run` | 由唯一集成测试负责人统一执行，避免并行重复测试 |

## 8. 集成测试交接清单

唯一集成测试负责人接续时应优先执行：

1. 用有效 Session 对网站 API 的共线路径做参数变体测试，记录真实 HTTP 状态、响应 envelope 和脱敏请求摘要；
2. 优先核对 `POST /api/v2/xpack/waf/*` 与 `/api/v2/websites/waf/*` 的实际注册命中情况；
3. 用真实网站记录验证 `runtimeID` 字符串、PHP 运行时切换、数据库名称字符串和创建请求别名优先级；
4. 验证同源 WS relay 的 `operateNode`、Origin、Session、断线清理和消息序列；
5. 对参考独有许可证路径和防火墙 `taskID` 差异决定“保留兼容、正式删除或补回前端”，并在 `docs/migration` 留记录；
6. 将每条真实结果写入集成测试证据，不把本文件的 `not-run` 或 `blocked` 改成 `pass`。
