<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WAF 图片页面与生效链审计（2026-09-12）

状态：`[>]` 代码对应关系已复核，浏览器截图和生产 OpenResty 验收未完成

本轮复核 `docs/img/` 的 7 张参考图、前端路由、页面组件、API 调用和后端
OpenResty 生效链。未修改前端样式，未执行生产 reload。

## 图片到页面对应关系

| 参考图 | 路由 | 页面组件 | 主要功能 | 当前结论 |
| --- | --- | --- | --- | --- |
| `概览.png` | `/advanced/waf/overview` | `web/src/views/advanced/waf/overview/index.vue` | 今日状态、30 日地图、7 日请求/拦截趋势 | 已有对应页面；地图使用 `china.json`/`world.json` 和 ECharts |
| `攻击报表.png` | `/advanced/waf/attack` | `web/src/views/advanced/waf/attack/index.vue` | IP/URL/网站 TOP 报表、拉黑、加白 | 已有对应页面；数据来自 WAF 日志聚合 |
| `拦截记录.png` | `/advanced/waf/intercept` | `web/src/views/advanced/waf/intercept/index.vue` | 条件筛选、批量拉黑、批量 URL 加白、导出/清空 | 已有对应页面；保存动作走黑白名单 API |
| `封锁记录.png` | `/advanced/waf/block` | `web/src/views/advanced/waf/block/index.vue` | 封锁列表、批量永久拉黑、导出/清空 | 已有对应页面；保存动作走黑名单 API |
| `黑白名单.png` | `/advanced/waf/blackwhite` | `web/src/views/advanced/waf/blackwhite/index.vue` | IP、URL、User-Agent、IP 组、状态、备注、分页 | 已有对应页面；`listMeta` 保存行级状态和备注 |
| `网站设置.png` | `/advanced/waf/websites` | `web/src/views/advanced/waf/websites/index.vue` | 站点开关、执行策略、检测强度、频率限制、站点规则 | 已有对应页面；详细设置为站点规则弹窗 |
| `全局设置.png` | `/advanced/waf/global` | `web/src/views/advanced/waf/global/index.vue` | 频率限制、默认规则、自定义规则、基础配置、Redis | 已有对应页面；保存后查询 `waf/status` 确认生效 |

路由入口在 `web/src/routers/modules/advanced.ts`，`/advanced/waf` 默认重定向到
`/advanced/waf/overview`。图片数量、路由数量和组件数量均为 7。

## 保存链路

前端 API 集中在 `web/src/api/modules/waf.ts`：

- 概览和报表：`GET /api/v2/websites/waf/overview`、`GET /api/v2/websites/waf/logs/:kind`。
- 网站设置：`GET/POST /api/v2/websites/waf/sites`、`GET /sites/:id/rules`、
  `POST /websites/waf/rules`、`POST /websites/waf/rules/delete`。
- 黑白名单和日志操作：`GET/POST /api/v2/websites/waf/access-lists`、
  `POST /api/v2/websites/waf/logs/:kind/clear`。
- 全局设置：`GET/POST /api/v2/websites/waf/global`、
  `GET/POST /api/v2/websites/waf/global/default-rules`、
  `GET/POST /api/v2/websites/waf/global/custom-rules`、
  `POST /api/v2/websites/waf/global/apply`。
- 生效确认：`GET /api/v2/websites/waf/status`。

后端路由在 `node/api/website_waf.go` 注册，同一批写操作进入
`node/service/website_waf.go` 和 `node/service/website_waf_files.go`：

1. 写入 WorkMesh WAF JSON 文件，包括全局配置、名单、默认/自定义规则和站点规则。
2. 生成 OpenResty/ModSecurity 运行文件。
3. 如配置了 `WORKMESH_WAF_RELOAD` 或容器/本机 OpenResty 目标，执行 `nginx -t`。
4. 执行 `nginx -s reload` 或容器内 reload。
5. reload 后再次探测配置有效性，写入 `runtime.json` 和配置 hash。
6. 任一步失败时回滚文件和内存状态；前端只在 `effective=true` 时提示“已确认生效”。

因此当前实现不是纯前端表面保存；但生产环境是否真正生效，仍取决于目标机器能否访问
OpenResty、Docker socket 和实际域名请求。

## 仍未完成

| 项目 | 原因 | 下一步 |
| --- | --- | --- |
| 浏览器截图像素验收 | 当前环境缺 Chromium/Chrome/Firefox 和 Playwright/Puppeteer | 安装浏览器依赖后逐页打开 7 个路由，对照 `docs/img/*.png` 保存截图证据 |
| 生产 OpenResty 生效 | 当前环境 Docker socket 不可访问、本机无 `openresty/nginx` | 在可控环境执行保存、`nginx -t`、reload、reload 后复检和失败回滚 |
| 真实 WAF 请求行为 | 当前没有可用隔离域名和生产维护窗口 | 用真实域名请求验证 observe/block、白名单优先级、黑名单和频率限制 |
| 当前站点数据 | SQLite `websites=0`、`website_domains=0`，但文件系统有 19 个 `site.conf` | 先完成站点对账分类，再做 WAF/域名生产 reload |

结论：图片页面任务没有丢失，源码中已有 7 图对应的 7 个页面和 API 保存链；
未完成的是浏览器截图验收、生产 OpenResty reload、真实域名/WAF 请求和站点目录
与 SQLite 对账。
