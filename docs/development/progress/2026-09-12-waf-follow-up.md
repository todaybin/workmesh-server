<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WAF 跟进任务（2026-09-12）

状态：`[>]` 页面和控制面已完成，运行时数据源与生产验收待继续

## 已确认完成

- `docs/img/` 七张参考图对应的七个 WAF 页面已完成逐页截图对齐。
- 概览已使用真实 `china.json` 和 `world.json` GeoJSON 地图资源。
- 日志字段兼容和筛选已完成，包含 IP、IP 归属地、网站、Host、URL、规则、动作、状态、关键词和时间范围。
- Server 已接入 ModSecurity JSON/JSONL/数组/对象和 Nginx combined `access.log` 归一化；查询、概览和清理接口使用同一批日志白名单。
- OpenResty 普通 combined `access.log` 已统一写入 `/opt/workmesh/waf/logs/access.log`，与 ModSecurity/WorkMesh 审计日志共用 WAF 挂载目录。
- 网站设置已提供当前站点的规则编辑弹窗，支持加载、新增、保存和删除。
- 黑白名单已改为逐行规则表格，状态/备注通过 `listMeta` 持久化，并由 Lua 在请求匹配时执行行级启用判断。
- Lua 已读取默认规则、站点规则、自定义规则、IP 组和频率配置；Redis 频率计数失败时回退 shared dict。
- WAF 配置保存代码包含文件快照、OpenResty `-t`、reload、复检、配置 hash 和失败回滚；隔离 fake/OpenResty 测试已通过。
- 已修正标准规则开关的生成语义：`standardRules=false` 现在只移除 CRS include，不再写入 `SecRuleEngine Off` 误关闭整个 ModSecurity 引擎；自定义 ModSecurity 基础规则和 WorkMesh Lua 规则仍保持独立执行。
- 已核对生产 Compose 的 WAF 挂载：`/opt/workmesh-server/openresty-waf/waf` 对应容器 `/opt/workmesh/waf`，其中 `data`、`generated`、`logs` 分别对应 Server 默认 WAF 数据、生成配置和日志目录。
- `deploy/openresty-waf` 已纳入当前 `workmesh-server` 仓库，包含入口脚本与运行时契约：空挂载目录首次启动会生成 `generated/modsecurity-mode.conf`、`custom-rules.conf`、`standard-rules.conf`、`global-rules.conf`；已有控制面保存文件不会被覆盖；`modsecurity.conf` 通过 `IncludeOptional` 读取这些生成文件。
- `deploy/openresty-waf/tests/image-release-contract.sh` 已纳入当前仓库的镜像发布静态门禁：候选 `IMAGE_REF` 不能是 `latest`、不能是旧 `20260904`，并检查 Dockerfile 构建期 `nginx -t`、Compose WAF 挂载和 `standard-rules.conf` include 链；门禁本身不执行 build/push 或容器变更。
- 已新增只读域名/WAF 验收脚本 `deploy/acceptance/domain-acceptance.sh`，并保留镜像目录下的 `deploy/openresty-waf/tests/domain-acceptance.sh` 作为容器内请求矩阵；用于现场验证 HTTP/HTTPS Host 路由、HTTPS 握手、攻击请求拦截、容器 `nginx -t`/health 和日志文件存在；脚本不执行 DNS 写入、证书签发、reload、重启或清理。
- 已新增只读生产预检入口 `deploy/acceptance/preflight.sh`，统一检查 Docker、OpenResty/WAF 目录、数据库 Compose 模板、域名解析、ACME 工具和浏览器自动化前置条件；当前本机预检输出 `warnings=5 failures=1`，其中包括 Docker Socket 不可用、`workmesh.cs.sopvip.com` 尚未完成 DNS 解析、无浏览器自动化依赖、生产 WAF 挂载目录缺少新版 `standard-rules.conf`，以及旧 `20260904` 镜像引用硬失败。
- `preflight.sh` 已改为从脚本自身路径定位当前 `workmesh-server` 仓库，可从任意当前工作目录执行；新增合约测试覆盖路径定位、Shell 语法、当前仓库 WAF 资产和只读边界。
- 已新增 `deploy/acceptance/readonly-evidence.sh` 只读证据收集器，集中保存采集时间、Git 状态、OpenResty `-t`、WAF 文件列表/hash、站点 `site.conf` 脱敏快照、数据库 Compose `config --quiet`、SQLite 网站/域名/运行态摘要、只读 API 状态、HTTP/HTTPS/WAF 探测状态和响应头摘要；脚本不保存响应体或 `CID` 明文。
- 已新增 `deploy/acceptance/site-reconcile.sh` 只读站点对账脚本，核对 SQLite `websites`/`website_domains` 与 `/www/wwwroot/*/nginx/site.conf` 的 `server_name`，并检查站点 WAF `config.json`/`rules.json`；目标环境缺少 `sqlite3` 时会回退到仓库内 `modernc.org/sqlite` Go 导出器并保持 `mode=ro`。
- 已验证 `STRICT=1 deploy/acceptance/preflight.sh` 在当前阻断环境返回非零，可作为上线前硬门禁；默认模式保留 0 退出码，便于日常只读诊断收集完整报告。

## 未完成任务

1. GeoIP：在 OpenResty 部署中配置真实 GeoIP 模块/变量并验证日志中的 `ipRegion`。
2. OpenResty：在有 Docker Socket 或本机 OpenResty 的环境执行真实 `-t`、reload、健康请求和 block 请求。
3. 域名与生产：执行 HTTP/HTTPS Host 路由、ACME HTTP-01/DNS-01、生产 WAF 变更、制品发布和回滚验收；无凭据验收矩阵和命令模板见 `docs/operations/waf-domain-acceptance.md`。
4. 新版 WAF 镜像需要从当前仓库 `deploy/openresty-waf/` 重新构建/发布，并用 `IMAGE_REF=<new-tag-or-digest> sh deploy/openresty-waf/tests/image-release-contract.sh` 过门禁；当前生产镜像 `workmesh/openresty-waf:20260904` 尚未在本环境重建验证。
5. 站点对账：执行 `deploy/acceptance/site-reconcile.sh --strict`，确认 SQLite 记录、`site_dir`、`server_name` 和站点 WAF 文件一致，再清理历史/测试配置。当前只读实测已打开 `/opt/workmesh-server/data/workmesh.db`，SQLite 中 `websites=0`、`website_domains=0`，而 `/www/wwwroot` 有 19 个 `nginx/site.conf`，结果为 `warnings=0 mismatches=19`；这些配置暂判为“文件系统存在但数据库无对应记录”，不得直接删除，需先完成分类、导入或经授权清理，清单见 `2026-09-12-site-reconcile-current.md`。
6. 证据归档：真实执行域名/WAF/HTTPS/数据库验收时，使用 `deploy/acceptance/readonly-evidence.sh` 保存 API 摘要、SQLite 记录、`site.conf`、WAF 生成文件 hash、`nginx -t` 输出、HTTP/HTTPS/WAF 请求状态和清理结果。

## 当前阻断

- 当前环境无 Docker Socket 权限，未安装 `openresty`/`nginx`。
- `workmesh.cs.sopvip.com` 尚未形成可靠 DNS 解析证据，仍缺 DNS/ACME 凭据。
- 完整 Go 测试仍受沙箱 IPv6 loopback 监听限制。

验收原则：页面完成、控制面保存成功、隔离测试通过，均不能替代真实 GeoIP/ModSecurity 数据、OpenResty reload、域名访问和生产回滚证据。
