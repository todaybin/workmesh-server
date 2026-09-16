<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WAF 页面与 OpenResty 生效闭环（2026-09-11）

状态：`[>]` 页面截图对齐和控制面代码闭环已完成，真实日志来源与生产运行时验收未完成

## 已完成

- 已核对 `docs/img/` 中的七张参考图，并完成七个 WAF 页面逐页截图对齐：
  - 概览
  - 攻击报表
  - 拦截记录
  - 封锁记录
  - 黑白名单
  - 网站设置
  - 全局设置
- 概览已接入真实 GeoJSON 地图资源 `china.json` 和 `world.json`，使用省/国家区域名称映射和来源数量着色，不再使用点阵或 CSS 假地图。
- 概览使用 `GET /api/v2/websites/waf/overview` 读取全量 JSONL，返回今日统计、7 日请求/拦截趋势和 30 日来源汇总。
- 日志字段和筛选已补齐：兼容 `client_ip`、`clientIP`、`ip`，支持 `ipRegion`、`ip_region`、`region`、`country`；查询支持分页、网站、Host、IP、IP 归属地、URL、规则、动作、状态、关键词和时间范围筛选。
- 攻击报表、拦截记录和封锁记录已显示 IP 归属地；拦截记录支持按行发起单 IP 或规范化 IP 段拉黑。
- 网站设置的“详细设置”已改为当前站点内的规则编辑弹窗，支持加载、新增、保存和删除站点规则，不再跳转到忽略 `websiteID` 的全局页面。
- 黑白名单页已按参考图改为规则表格，支持类型、规则、状态、备注、删除和分页；`listMeta` 只保存已存在规则的行状态/备注，旧数组格式继续兼容 Lua 匹配。
- 网站设置保存网站开关、观察/防护模式、检测强度和频率开关；网站 WAF 配置写入站点 `waf/config.json`。
- 全局频率设置同时写入 `frequency` 和 `frequencyLimit`；网站频率配置兼容 `rateLimits` 和 `frequencyLimit`。
- WAF 配置保存代码包含以下生效链：
  1. 配置文件快照；
  2. OpenResty `-t`；
  3. `-s reload`；
  4. reload 后再次检查可用性和配置；
  5. 写入 `runtime.json` 和配置 SHA-256。
- reload 或复检失败时恢复文件和内存状态，并尝试恢复旧配置；前端保存后重新读取状态，只有 `effective=true` 才提示确认生效。
- 网站别名/域名等普通网站更新如果会重写 WAF 站点配置，同样经过运行时 reload；新增回归测试确认 reload 失败时网站内存、SQLite 和 WAF 文件全部回滚。
- 2026-09-12 追加复核：七张截图与七个对应路由和页面组件已完成对齐；这部分任务不是因会话丢失而缺失，但不等于真实生产站点已经验收。
- 2026-09-12 追加修复：WAF 全局/站点规则、网站 HTTPS、域名增删、rewrite、日志开关、运行目录、负载均衡文件、默认页面同步等 OpenResty 相关写路径，失败时会返回补偿错误并恢复内存/文件状态，避免只保存表面配置。

## 自动化证据

通过：

```text
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./node/service -run 'Test(WAF|WebsiteUpdateReturnsWAFReloadFailure)|TestQueryWAFLogs|TestWAFOverview' -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./node/api -run 'Test(WebsiteWAF|ZNMPStaticWebsiteAllInterfaces|ExternalWAF)' -count=1
cd web && npm run type-check
git diff --check
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./node/service -run 'TestWebsite(ConfigUpdate|Update|Delete|Create|Stream)|TestWAF|TestWebsite.*Rewrite|Test.*LimitConn' -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./node/api -run 'Test(WebsiteWAF|Website|WAF)' -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go vet ./node/service ./control/api
```

新增/覆盖重点：

- fake OpenResty 的 `-t` 和 `-s reload` 调用；
- reload 失败后的内存、文件回滚；
- 网站更新触发 WAF reload 失败时的完整回滚；
- `frequencyLimit` 写入和 Lua 兼容字段；
- WAF 日志时间、状态、网站、IP 归属地和关键词筛选；
- 概览统计使用全部记录而不是单页记录；
- 七页面前端类型检查和生产 GeoJSON 资源加载。

## 外部黑盒证据

- [x] 2026-09-10 已执行 `WORKMESH_WAF_EXTERNAL_TEST=1 GOWORK=off go test ./node/api -run '^TestExternalWAFLifecycle$' -count=1`，在隔离 Docker/OpenResty 容器中验证站点规则 observe/block 隔离、IP/CIDR 黑白名单优先级、实际 HTTP 状态码和 JSONL 审计日志；测试结束后临时容器、网络和目录已清理。

## 未完成任务

### 日志与数据来源

- [!] 接入 GeoIP 数据源仍依赖部署环境的 GeoIP 模块/变量；Lua 已按可选 GeoIP 变量写入 `ipRegion`，没有数据时保持缺失，不能把 `source` 或规则名称冒充地域。
- [x] Server 已接入并归一化 ModSecurity 审计日志 `modsecurity-audit.json`，覆盖 JSONL、数组/对象格式以及 `transaction.client_ip`、`transaction.time_stamp`、`transaction.response.http_code`、`request.uri`、Host 等嵌套字段。
- [x] 已增加受控的普通 Nginx combined 格式 `access.log` 解析和概览统计，保留 JSON/JSONL 兼容，并忽略不符合格式的文本行。
- [x] OpenResty 模板已将普通 combined `access.log` 写入 `/opt/workmesh/waf/logs/access.log`，与 Server 查询的 WAF 日志根目录一致。
- [x] 默认规则、站点规则、IP 组、Redis/频率配置已接入 Lua 实际读取路径；Redis 不可用时回退 shared dict，IP 组支持站点规则引用。

### OpenResty、域名与生产验收

- [ ] 在可访问 OpenResty 或 `workmesh/openresty-waf` 容器的环境执行真实 `-t`、reload、健康请求和阻断请求，并保存成功/失败回滚证据。
- [ ] 复核 `WORKMESH_WAF_ROOT`、站点目录挂载、配置文件路径、生成的 `modsecurity-mode.conf` 和镜像内 Lua 运行时读取路径；当前保存链只有在配置了实际 OpenResty 执行器时才能完成真实 reload。
- [ ] 用隔离域名执行 HTTP/HTTPS Host 路由、证书、ACME HTTP-01 和 DNS-01 续期测试。
- [ ] 在生产维护窗口执行 WAF 规则、黑白名单、频率限制和站点规则变更，确认保存后实际 HTTP 请求行为改变。
- [ ] 执行数据库 Compose 启动、健康检查、备份恢复、制品发布和生产回滚。

## 环境阻断

- 当前工作区有 Docker CLI，但没有 Docker Socket 权限，不能在本环境重跑真实容器验收。
- 当前环境未安装 `openresty`/`nginx`，不能执行本机真实配置检查和 reload。
- `znmp.sopvip.com` 当前没有可靠 DNS 解析证据，且没有可用 DNS/ACME 凭据，域名和证书测试保持 `not-run`。
- 完整 Go 测试还受到沙箱禁止 IPv6 loopback 的限制：`httptest.NewServer` 在 `listen tcp6 [::1]:0` 处失败。

隔离 WAF 黑盒和 fake OpenResty 测试通过，只能证明代码路径及隔离资源行为；不能替代 GeoIP、ModSecurity 审计、真实生产 OpenResty、域名部署或生产发布回滚验收。
