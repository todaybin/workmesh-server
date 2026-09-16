<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 网站、OpenResty 与 WAF 接口

WorkMesh Server 将网站元数据和 OpenResty 控制面保存在
`WORKMESH_DATA_DIR/workmesh.db` SQLite 数据库中；WAF 运行态配置由 WorkMesh
自有 JSON/JSONL 文件管理，不依赖 `workmesh-node`、`1pwaf/data` 或旧
`website_waf_*` 表。JSON 文件使用临时文件和原子替换，OpenResty/ModSecurity
运行配置由配置应用流程生成，并在配置检查、reload 和 reload 后复检全部通过后
才标记为生效。

全局 WAF 文件位于 `<waf-root>/global.json`、`default-rules.json`、
`custom-rules.json` 和 `access-lists.json`；每个网站独立使用
`<site-root>/waf/config.json`、`rules.json`；WAF 自定义审计日志位于 WAF 根目录的
`logs/workmesh-custom-audit.jsonl`。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET/POST | `/api/v2/websites`、`/api/v2/websites/list`、`/api/v2/websites/search` | 网站列表、创建和搜索 |
| GET | `/api/v2/websites/{id}` | 网站详情 |
| POST | `/api/v2/websites/update`、`/api/v2/websites/del` | 更新或删除网站 |
| GET/POST | `/api/v2/websites/waf/access-lists` | 读取或更新 WAF 黑白名单，支持 IP、CIDR、URL、User-Agent 和 IP 组 |
| GET | `/api/v2/websites/waf/sites` | 列出网站 WAF 配置 |
| POST | `/api/v2/websites/waf/sites` | 更新网站 WAF 开关、observe/block 模式、检测强度和频率开关 |
| GET | `/api/v2/websites/waf/sites/{id}/rules` | 列出网站规则 |
| POST | `/api/v2/websites/waf/rules`、`/rules/delete` | 新增、更新或删除网站规则 |
| GET/POST | `/api/v2/websites/waf/global`、`/waf/status` | 读取或更新节点级 WAF 配置和状态 |
| GET | `/api/v2/websites/waf/overview` | 返回今日状态、7 日请求/拦截趋势和 30 日来源汇总 |
| GET/POST | `/api/v2/websites/waf/global/default-rules`、`/global/custom-rules` | 读取或更新全局默认规则、自定义规则 |
| POST | `/api/v2/websites/waf/global/apply` | 将全局默认规则应用到网站 |
| GET/POST | `/api/v2/websites/waf/logs/{access\|log\|attack\|intercept\|block}` | 查询 WorkMesh JSON/JSONL、ModSecurity `modsecurity-audit.json` 和 Nginx combined `access.log`，支持分页和筛选 |
| POST | `/api/v2/websites/waf/logs/{kind}/clear` | 只清理 WAF 管理范围内的对应 JSON/JSONL、ModSecurity 和 access 日志文件 |
| GET | `/api/v2/websites/waf/standard-rules` | 获取内置规则清单 |
| GET | `/api/v2/openresty/status`、`/modules`、`/https` | 查询 OpenResty 运行、模块和默认 HTTPS 状态 |
| POST | `/api/v2/openresty/update`、`/file`、`/scope`、`/build`、`/modules/update`、`/https` | 更新 OpenResty 配置摘要或默认 HTTPS 开关 |

## 建站前应用预检

`POST /api/v2/websites/check` 沿用 Node 原版契约。服务端读取 `openresty` 应用安装记录中持久化的 `containerName`，通过 Docker 精确查询该安装关联的全部容器（包含已停止容器），再同步应用状态。不会使用宿主机 `nginx` 二进制、监听端口或镜像名称替代容器归属判断。

请求体支持 `installIds`（兼容旧的 `InstallIds` 字段）。OpenResty 及请求指定的应用全部为 `Running` 时返回 `data: null`；安装记录缺失或任一容器状态异常时返回 `data` 数组，数组元素包含 `name`、`appName`、`version`、`status`。容器状态映射与原版一致：`running`、`exited`、`restarting`、`paused` 分别对应 `Running`、`Stopped`、`ReStarting`、`Paused`，容器缺失对应 `Error`，混合状态对应 `UnHealthy`。

所有成功响应使用数字 `code: 200`；参数错误返回 `ERR` 和 HTTP 400，资源不存在返回 HTTP 404。
OpenResty 的 `build`、模块和配置文件接口只更新本地控制面状态，不在 HTTP 请求中执行特权命令；实际运行时变更应由受控异步任务完成。

## WAF 生效判定

WAF 保存不是“写文件成功”就算生效。启用运行时校验时（设置
`WORKMESH_WAF_RELOAD=1`，或配置 `WORKMESH_OPENRESTY_BIN` /
`WORKMESH_OPENRESTY_CONTAINER`），每次 WAF 全局配置、网站配置、规则或黑白名单
变更都必须按以下顺序完成：

1. 保存前创建全局、站点配置和规则文件快照。
2. 执行 OpenResty `-t`，确认当前配置可加载。
3. 执行受控 `-s reload`，确认命令成功。
4. reload 后再次执行可用性和配置检查。
5. 只有全部通过才写入 `runtime.json`，并以配置 SHA-256 标记 `effective: true`。

任一步失败，API 返回错误，同时恢复内存状态和文件快照，并尝试重新加载旧配置。
前端只有读取 `/waf/status` 得到 `effective: true` 时才显示“已确认生效”；如果没有
配置真实 OpenResty 目标，状态只能表示文件保存成功，不能作为生产验收证据。

全局频率配置同时兼容 `frequency` 和 `frequencyLimit`；网站配置使用
`frequencyEnabled` 与 `rateLimits`。Lua 运行时读取这些文件并按文件变更重新解析，
频率命中会写入 WAF JSONL 审计日志，封禁时长使用 `blockTime`，而不是只在页面上
保存一个未被运行时读取的字段。

服务启动时会校验 SQLite 中处于运行状态的网站目录：缺失或空的 `site.conf`/`stream.conf` 会按已保存的结构化设置恢复；停止的网站不会被自动重新启用。历史 stream 配置中的 `127.0.0.1:9` 或空 `proxy_pass` 不会被继续保留，只有存在真实上游时才生成可加载的 `proxy_pass`。
