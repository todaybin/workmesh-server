<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh Server 开发状态

这是开发恢复的唯一快速入口。先看本页，再打开对应任务详情；不要默认重新扫描全部迁移文档。

更新时间：2026-09-16

## 2026-09-16 反向站点代理配置

- [x] 反向站点创建后生成 `nginx/proxy/root.conf` 并由 `site.conf` 引用，代理菜单可读取默认配置。
- [x] 新增代理改为按名称独立维护 `.conf/.bak`，补齐编辑、启停、删除、文件更新和失败回滚；历史站点支持启动/首次读取补偿。
- [x] 代理定向测试（`GOWORK=off go test ./node/service -run 'TestWebsite' -count=1`、`GOWORK=off go test ./node/api -run 'TestWebsite' -count=1`）、`GOWORK=off go vet ./node/service ./node/api` 和 `git diff --check` 通过；完整测试在当前沙盒因 `httptest` IPv6 监听权限受阻，真实 OpenResty/上游访问待部署环境验收。
- [x] 详情见 [`2026-09-16-website-proxy-create.md`](progress/2026-09-16-website-proxy-create.md)。

## 2026-09-16 编译与部署状态

- [x] 已执行 `make clean-frontend && GOOS=linux GOARCH=amd64 make build-release`，生成单二进制发布包 `release/workmesh-server-linux-amd64`，SHA-256 为 `70943fc3fec6b7e17ba1cbceebbb4a710f16580dbfba547e21cab3899a0ab597`。
- [x] 发布包自校验通过，`--help` 可正常执行；源码工作区 `git diff --check` 通过。
- [x] 已部署到 `/opt/workmesh-server`：`workmesh-server.service` 于 `2026-09-16 16:48:11` 重启，MainPID 为 `3951658`；运行二进制与发布包 SHA-256 均为 `70943fc3fec6b7e17ba1cbceebbb4a710f16580dbfba547e21cab3899a0ab597`。
- [x] 部署后 `GET /health` 返回 `code=200,status=ok`，`GET /ready` 返回 `code=200,status=ready`；旧版本备份为 `/opt/workmesh-server/bin/workmesh-server.bak.20260916164810-3951537`。
- [!] 生产预检存在 Docker daemon、WAF 镜像引用、域名解析和浏览器验收阻塞，需在真实维护主机完成 `activate-release.sh` 与 `/health`、`/ready`、反向代理配置和上游访问验收。
- [x] 先前只读挂载阻塞已解除；本次使用 `deploy/install/activate-release.sh` 完成原子替换、systemd 重启、MainPID 和 9999 端口归属检查。

## 2026-09-13 网站监控概览与菜单层级

- [x] 网站监控概览已按 `docs/img/999.png` 重做，包含网站选择、8 项今日状态、30 日访客地图、实时请求/流量和访客趋势，并通过 WorkMesh 监控 API 加载真实访问日志数据。
- [x] 网站监控六项顶部导航、WAF 七项顶部导航和高级菜单二级入口层级已统一；监控/WAF 子路由不再作为侧栏三级菜单显示。
- [x] `npm run type-check`、`npm run build:pro`、`make build-release`、`git diff --check` 和前端菜单清单扫描已通过。
- [!] 浏览器截图像素验收、生产 systemd 激活和真实 `/health`、`/ready`、页面入口检查受当前环境 Docker/socket、浏览器、域名解析和 `/opt` 写权限限制，详见 [`2026-09-13-website-monitor-overview.md`](progress/2026-09-13-website-monitor-overview.md)。

## 2026-09-13 发布激活与线上旧版本排查

- [x] 已确认当前线上页面没有变化的直接原因：`/opt/workmesh-server/bin/workmesh-server` 仍为 2026-09-06 旧二进制，SHA-256 为 `4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f`；`/opt/workmesh-server/web/dist` 仍保留旧 WAF bundle，含 `waf-tabs`。
- [x] 当前源码、前端构建和 Go embed 发布包已包含七个 WAF 子路由：`/advanced/waf/overview`、`attack`、`intercept`、`block`、`blackwhite`、`websites`、`global`；本地最新发布包为 `/www/apps/workmesh-server/release/workmesh-server-linux-amd64`，SHA-256 为 `655207b820fa7e4c74ad1f10275ec428033c02dbfbfb60b9d24ef22b7e1c043a`。
- [x] 新增 `deploy/install/activate-release.sh`：强制校验 SHA 文件、备份旧二进制、同目录临时文件原子替换、发布锁、systemd ExecStart/MainPID/进程路径/9999 端口归属检查，以及新版 WAF bundle 内容检查；服务或 HTTP 检查失败时尝试回滚。
- [x] Makefile 的 `build-release` 现在会同步更新 `release/workmesh-server-linux-amd64` 和 `.sha256`；Go 构建默认使用可写缓存和离线模块模式，避免当前受限环境因 `/root/.cache` 或模块下载临时文件失败。
- [!] 生产激活仍未完成：当前执行环境的 `/opt` 挂载为只读，systemd 总线不可访问，9999 和公网 `61.184.12.165:9999` 无法在本环境验证；因此不能宣称生产二进制已替换、服务已重启或公网页面已更新。
- [>] 下一步必须在真实运行主机执行：`WORKMESH_SERVER_ROOT=/opt/workmesh-server WORKMESH_SERVER_BINARY=/www/apps/workmesh-server/release/workmesh-server-linux-amd64 WORKMESH_SERVER_SHA_FILE=/www/apps/workmesh-server/release/workmesh-server-linux-amd64.sha256 bash deploy/install/activate-release.sh`，然后核对 systemd MainPID、二进制 hash、`/health`、`/ready`、WAF 七个页面和浏览器缓存清理。
- [x] 本次重新部署尝试已执行并被安全拒绝，退出码为 `1`，原因是 `/opt/workmesh-server/bin` 不可写；生产旧二进制 SHA-256 保持 `4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f`，未发生半替换。
- [x] 新增 `deploy/install/uninstall-production.sh`：默认只读预览；正式执行前置检查 systemd、Docker、运行目录和 `/etc/systemd/system` 写权限，停止并移除 WorkMesh 服务/容器/网络，将 `/opt/workmesh-server` 改名保留备份；`--purge-data` 才删除 WorkMesh 数据和命名数据库卷，始终保护 `/www/wwwroot`、`/opt/workmesh` 及旧 ZNMP 资源。
- [!] 本次生产卸载尝试同样未执行，退出码为 `1`，原因是 `/opt` 不可写；当前 `/etc/systemd/system/workmesh-server.service`、生产二进制和运行数据均未改变。
- [x] 本次本地测试通过：`make build-release`、前端 `vue-tsc --noEmit`、WAF 后端定向测试、`cmd/workmesh-server` HTTP 静态路由测试、WAF runtime/image release contract、部署 acceptance self-test、`go vet ./...`、`git diff --check`。
- [!] 本次完整 Go 测试仍被沙箱禁止 `httptest` IPv6 loopback 监听阻断；Node contract 测试中的子进程执行也被沙箱 `EPERM` 阻断，但对应扫描器直接命令已可执行，生产 Docker/OpenResty、域名、浏览器截图和公网 HTTP 仍未测试。

## 2026-09-12 部署、域名、环境与目录审计

- [x] 当前主项目边界已确认：前端、后端、部署脚本和 WAF 镜像资产全部位于 `/www/apps/workmesh-server`；业务行为只参考只读 `/www/apps/1Panel`，已移除的 `workmesh-node` 不得作为实现、部署或测试来源。
- [x] 完成部署/域名/环境测试/目录结构相关文档和脚本只读审计；集中结论见 [`2026-09-12-deployment-domain-env-directory-audit.md`](progress/2026-09-12-deployment-domain-env-directory-audit.md)。
- [!] 当前 `deploy/acceptance/preflight.sh` 实测为 `warnings=5 failures=1`，严格门禁未通过：Docker daemon/socket 不可访问，生产 WAF 挂载缺 `generated/standard-rules.conf`，旧 WAF 镜像引用仍为 `20260904`，DNS 尚未解析，缺少浏览器和 Playwright/Puppeteer；验收域名已确定为 `workmesh.cs.sopvip.com`，公网 IPv4 为 `61.184.12.165`。
- [!] 当前 `deploy/database/preflight.sh` 为 `warnings=4 failures=0`，但严格门禁未通过：Docker daemon/socket 不可访问，生产数据库运行 Compose 和 secret 目录缺失，固定资源碰撞检查无法执行。
- [x] 新增 `deploy/install/prepare-runtime.sh` 运行目录准备入口：支持 dry-run、`--apply`、`--force` 和 `--self-test`；只创建缺失的数据库 secret、Compose、WAF 目录及默认生成配置，不启动或删除容器，不覆盖已有 WAF 数据。
- [x] 当前主机网络配置明确记录公网 IPv4 为 `61.184.12.165`；新增 `deploy/install/domain-plan.sh` 和 `deploy/acceptance/host-ip.sh`，默认验收域名为 `workmesh.cs.sopvip.com`，可输出 DNS A 记录和验收命令。
- [!] 本机仍不能执行真实上线：无法访问 Docker socket、解析 `workmesh.cs.sopvip.com`、构建/运行 OpenResty WAF 或进行浏览器截图验收；DNS A 记录必须由域名服务商实际配置后再验证。
- [!] 当前 `/opt/workmesh-server/openresty-waf/docker-compose.yml` 仍引用旧 `workmesh/openresty-waf:20260904`，且运行目录缺 `generated/standard-rules.conf`；需要在真实维护窗口用 `prepare-runtime.sh --apply --force --domain workmesh.cs.sopvip.com` 生成新运行文件，再绑定不可变 WAF 镜像 digest。
- [x] 当前部署现场证据已固化到 [`2026-09-12-deployment-current.md`](progress/2026-09-12-deployment-current.md)，包含主机能力探测、旧镜像/缺失文件、域名规划和真实部署顺序，避免会话丢失后重复盘点。
- [!] 当前 `deploy/acceptance/site-reconcile.sh` 已通过 Go SQLite fallback 读取 `/opt/workmesh-server/data/workmesh.db`，结果为 SQLite `websites=0`、`website_domains=0`，但 `/www/wwwroot` 存在 19 个 `nginx/site.conf`，全部未匹配 SQLite `site_dir`。未获授权前不得删除、移动或自动导入。
- [>] 目录结构任务仍处于分阶段迁移：repository/事务边界和职责拆分已有进展，但顶层物理目录尚未整体迁移；完成真实 OpenResty、域名、数据库和发布回滚验收前，不用目录搬迁替代生产闭环。

## 2026-09-12 运行期兼容性与错误边界

- [x] 应用安装主链路已检查 `storeAppInstallRecord`、无资源兼容登记、排序和忽略列表：SQLite/状态文件保存失败会恢复内存快照并返回 `503`，不会继续启动未持久化的安装任务。
- [x] 运行时安装路径、Compose/环境文件元数据和 Node 模块后台任务已补齐保存失败边界；Node 任务无法持久化 `running` 状态时不会执行 Docker 命令，运行时路径保存失败会停止后续安装。
- [x] 设置、设置快照、告警、容器 Compose/镜像仓库/模板和网站扩展公共状态的关键写接口已补齐回滚；保存失败返回错误，不再固定返回成功或保留脏内存快照。
- [x] 新增运行时路径、Node 模块任务持久化失败回归测试；`node/api` 功能定向测试和 `go vet ./node/api` 通过。
- [!] 2026-09-12 全量 `go test ./...` 复核仍被当前沙箱禁止 IPv6 loopback 阻断：`httptest.NewServer` 在 `listen tcp6 [::1]:0` 处失败；`go vet ./...`、`go test ./... -run '^$'`、前端类型检查、759 条路由扫描和 `git diff --check` 可作为当前代码门禁证据。
- [x] 修复无共享 SQLite 时应用/容器状态已写入 JSON 后仍继续调用 SQLite 保存的问题；无数据库兼容路径现在可正常保存，SQLite 启用时仍只写 repository 和关系表。
- [x] 登录审计、HTTP 操作审计、应用安装任务记录和日志保留维护直接使用 `SharedRepository()`，启动建表和旧数据导入边界保持不变。
- [x] 数据库备份记录、PostgreSQL/Redis 运行时配置的业务读写和兼容建表现在通过 `SQLExecutor`；运行期路由不再直接持有或调用 `*sql.DB`。
- [x] Docker CLI 因 daemon/socket 权限拒绝时统一识别为外部依赖不可用并返回 `503`，不再误报普通 `500`。
- [x] 临时文件系统不支持合成 UID/GID 或沙箱禁止 loopback 监听时，相关测试明确记录为环境跳过；生产目录的 chown 和真实 Docker/OpenResty 行为仍返回真实错误。
- [x] 本轮定向数据库、网站、日志和服务测试通过；完整 `cmd/workmesh-server` 测试仍受当前沙箱禁止 `httptest` IPv6 监听影响，保持环境阻塞记录。
- [x] Gateway 注册、登录、心跳、授权刷新和解绑的 repository 写失败会恢复内存绑定/授权状态；后台心跳和离线状态保存失败会记录错误，不再静默吞掉。
- [x] Website `Update` 已统一使用网站/WAF/关系表快照回滚；域名重命名会恢复目录和 `site.conf`，SQLite 或 OpenResty/WAF 生效失败不会留下新域名、别名或 WAF 文件。
- [x] Website 创建、更新、删除、stream 更新的持久化失败回归通过；本轮新增域名重命名回滚和 WAF 运行时失败回滚测试，`go vet ./node/service` 与 `git diff --check` 通过。
- [x] 2026-09-12 继续补齐网站配置补偿链：`UpdateConfig`、HTTPS、WAF 全局/站点规则、域名增删、rewrite、日志开关、运行目录、负载均衡文件和默认页面同步在持久化或 OpenResty/WAF reload 失败时返回 `errors.Join`，并恢复文件和内存快照。
- [x] 2026-09-12 文件管理长任务已补齐取消闭环：压缩、解压和移动返回异步 `taskID`，任务日志可轮询，`compress/stop`、`decompress/stop`、`move/stop` 通过 context 取消并保留失败/取消状态；临时归档输出不会在取消后替换正式目标。

## 2026-09-11 SQLite repository 抽象首批

- [x] `internal/storage` 新增 `SQLExecutor`、`Transactional` 和 `SQLiteRepository`，统一查询/写入与事务边界，同时保留旧 `DB()` 兼容出口。
- [x] `runtime/link.SQLiteSyncStore` 已接入 repository；compare-and-set、幂等重试和冲突游标行为保持不变。
- [x] `node/api` 日志分页/详情查询改为依赖 `SQLExecutor`，日志保留清理改为 repository 事务闭包，现有分页、脱敏和清理回滚测试通过。
- [x] 登录审计、HTTP 操作审计、应用/运行时任务状态和任务输出已接入 storage writer；文件型网站/WAF/系统/SSH/任务日志回退已统一使用有界只读 source。
- [x] storage 提交/回滚测试和链路同步接入测试通过；详见 [`2026-09-11-storage-repository.md`](progress/2026-09-11-storage-repository.md)。
- [x] 应用安装和运行时安装的任务状态/任务首条日志已进入 checked 错误边界；SQLite 状态与首条日志使用同一事务，后台任务遇到持久化失败会停止继续执行并回滚内存状态。
- [x] 新增 `internal/logsource.Source`，网站访问日志、WAF JSONL、SSH 日志、系统日志文件读取和任务日志文件回退统一使用有界只读 source；日志清理已接入 `FileMaintenance`，`journalctl` 读取仍保留为命令 source。
- [x] DNS 账户 CRUD、WebsiteSecurityService 的 ACME/CA/自签证书持久化，以及 SSLService 的证书记录、签发输入、DNS 账户读取和站点证书同步已接入 repository/事务边界；凭据不回传、无凭据更新不覆盖密钥、引用账户禁止删除。
- [x] website 关系表读取、OpenResty 配置读取、HTTPS 证书读取、默认页面、域名/限流/stream 配置路径已统一通过 repository；runtime 清单加载/保存和任务恢复也已移除业务层对底层 `db` 的直接查询。
- [x] `DatabaseRepository` 的列表、创建、更新、删除和连接信息读取已接入 repository；保留旧 `databases` 表缺少 `container_name` 时的兼容查询/写入。
- [x] `DatabaseAdminStore` 的用户、授权、变量和配置业务读写已接入 repository；底层 `*sql.DB` 仅保留给启动表初始化、旧 JSON 一次性导入和离线兼容。
- [x] 新增 `internal/logsource.FileMaintenance`，WAF、SSH、站点 access/error 日志清理统一经过受控路径、普通文件检查、截断/删除模式和上下文边界。
- [x] website `persist` 写入前的旧表 `payload` 兼容探测已通过 repository 执行；启动建表/列迁移仍保留 `*sql.DB`/`*sql.Tx` 作为 migration 边界。
- [x] 网站模板/模板产物 CRUD 已接入 repository；旧 `website_extension_state` BLOB 导入、建表和 migration 回调继续作为兼容边界保留。
- [x] 计划任务定义、执行记录、状态切换、删除和立即执行后的运行态保存已接入 repository；持久化失败会回滚内存执行记录，启动建表与旧 `cronjobs.json` 一次性导入继续保留在 migration 兼容边界。
- [x] 主次节点角色切换的 `role_state` 业务更新已接入 repository；角色表建表、首次读取和首次插入继续保留在 SQLite 初始化兼容边界。
- [x] Gateway 绑定状态的加载、保存、刷新和解绑已接入 repository；本地授权快照保留 access token 供重启恢复心跳，API 响应仍不暴露 token。
- [x] CoreService 的分组/设置运行期读写、脚本库查询/事务保存和快捷命令快照保存已接入 repository；旧 JSON 导入和表初始化继续保留在兼容边界。
- [x] `node/api` 脚本库与快捷命令的表初始化、查询和事务保存已完成 repository 收口；脚本新增、更新、远程同步在持久化失败时恢复内存快照，loopback 受限测试按环境明确跳过。
- [x] CoreService 分组和设置更新已补齐 SQLite 错误边界：分组增删、设置批量更新使用 repository/事务，持久化失败时恢复或保持原内存快照，并向 API 返回错误。
- [x] Node 运行时模块异步任务入队已补齐持久化错误边界：任务快照写入 SQLite 失败时返回 `503`，不启动后台 Docker 操作，也不保留内存任务。
- [x] 控制面 `role_nodes` 节点列表的加载和增删改/收藏快照保存已接入 repository；节点表建表和无数据库文件回退继续保留。
- [x] 共享应用/容器 JSON 状态、节点设置及应用/容器关系表的运行期读写已接入 repository；无公共数据库时继续保留原文件/内存兼容行为。
- [x] 主机列表、主机凭据读取、SSH 密钥证书 CRUD/同步/搜索，以及资源分组的运行期读写已接入 repository；表初始化和旧分组 JSON 导入仍保留兼容边界。
- [x] 数据库操作记录、数据库运行时配置和删除后的运行时配置清理已接入 repository；运行时配置表创建仍保留初始化兼容入口。
- [x] CoreService 用户凭据与 Passkey 的运行期保存、恢复和事务写入已接入 repository；核心表建表、首次用户读取和旧 JSON/Passkey 文件导入仍保留初始化兼容边界。
- [x] 后台计划任务启用计数、数据库备份记录查询/保存、MongoDB 删除依赖预检已接入 shared repository；备份元数据表创建仍保留路由初始化兼容入口。
- [>] 下一批继续迁移 runtime 外部资源状态、脚本及主机/control/application 等其他业务域剩余 `*sql.DB` 访问。
- [!] 2026-09-12 复核：当前沙箱有 Docker CLI 但无 Docker socket 权限，未安装 `openresty/nginx`，且 `znmp.sopvip.com` 解析失败；真实域名/ACME、生产 OpenResty、Docker Compose 和发布回滚仍是 `not-run`。

## 2026-09-11 WAF 页面与真实生效闭环

- [x] 已完成 `docs/img/` 七张参考图与七个 WAF 路由/页面组件的静态对应和页面功能实现：概览、攻击报表、拦截记录、封锁记录、黑白名单、网站设置、全局设置。
- [!] 七页浏览器截图像素验收尚未完成；当前缺少浏览器自动化依赖，不能把源码对应或类型检查写成截图验收通过。
- [x] 概览页已接入真实 `china.json`/`world.json` GeoJSON 地图资源，使用区域名称映射和来源数量着色；同时使用全量 JSONL 汇总接口展示今日状态、7 日请求/拦截趋势和 30 日来源统计。
- [x] WAF 日志字段和筛选已补齐：兼容 `client_ip`、`clientIP`、`ip` 以及 `ipRegion`、`ip_region`、`region`、`country`；支持分页、网站、Host、IP、IP 归属地、URL、规则、动作、状态、关键词和时间范围。
- [x] 网站设置“详细设置”已改为当前站点内的规则编辑弹窗，支持加载、新增、保存和删除站点规则，并调用现有 WAF 保存/生效链。
- [x] 黑白名单页已改为参考图对应的规则表格，补齐行状态、备注、删除和分页；后端保留旧数组格式并让 Lua 读取行级启用状态。
- [x] 网站设置页支持真实网站域名/备注、WAF 开关、观察/防护模式、检测强度和频率限制开关；保存后查询 `effective` 状态。
- [x] 黑白名单、规则、全局配置和网站配置的保存代码已统一接入 OpenResty 配置检查、reload、reload 后复检、配置 hash 和失败回滚；隔离测试证明失败会回滚，真实执行仍依赖外部 OpenResty 执行器和部署环境。
- [x] 网站通用配置、stream、域名删除、HTTPS 证书读取和连接限流配置已接入 SQLite repository/事务边界；旧数据库和无数据库兼容路径保留。
- [x] WAF 频率限制、默认规则、IP 组和 Redis 配置已完成控制面保存并接入 Lua 运行时；Redis 不可用时回退 shared dict，IP 组可被规则引用，默认规则文件会参与请求匹配。
- [x] WAF Server 日志已接入 ModSecurity JSON/JSONL/数组/对象和 Nginx combined `access.log` 归一化，支持 `transaction` 嵌套字段、分页筛选和清理。
- [x] OpenResty 普通访问日志已统一写入 `/opt/workmesh/waf/logs/access.log`，不再与 Server 查询目录分离。
- [x] 2026-09-12 补齐网站普通更新的 WAF 运行时失败回归：测试明确启用 `WORKMESH_WAF_ENFORCE=1`，验证 OpenResty reload 失败会拒绝更新，并恢复网站内存、SQLite 和 WAF 文件。
- [x] 2026-09-12 补齐网站域名重命名的统一回滚：SQLite 写入失败时恢复原目录、`site.conf`、主域名关系行、网站内存和 WAF 文件；补偿失败使用 `errors.Join` 返回。
- [x] 2026-09-12 复核 `docs/img/`：七张参考图对应七个页面路由和页面组件；源码和隔离运行链已有证据，但不等同于浏览器截图或生产站点验收。
- [x] 2026-09-12 WAF 保存链确认不是纯前端状态：全局、名单、默认/自定义规则、站点开关/模式/检测强度/频率字段写入 OpenResty WAF JSON 根目录；只有 `-t`、`reload`、reload 后复检和 hash manifest 全部通过才显示 `effective=true`。
- [x] 2026-09-12 收紧生产 WAF 门槛：服务数据根目录为 `/opt/workmesh-server/data` 时，即使未设置显式 reload 环境变量，WAF 写入也必须通过 OpenResty `-t`、reload 和复检；无法验证时写入回滚并返回错误，开发临时目录才可使用文件模式。
- [x] 2026-09-12 修正标准规则开关的 OpenResty 语义：生成 `standard-rules.conf` 独立控制 CRS include，`standardRules=false` 不再写 `SecRuleEngine Off` 误关闭自定义 ModSecurity/Lua 规则；Server 启动会为 bind-mounted WAF 目录补齐该文件。
- [x] 2026-09-12 WAF 镜像运行时契约已纳入当前仓库：`deploy/openresty-waf` 入口脚本会初始化缺失的 `generated/*.conf`，但不会覆盖控制面已保存文件；`runtime-contract.sh` 已验证 Compose 挂载、ModSecurity include 链和空挂载目录默认 CRS 行为。
- [x] 2026-09-12 WAF 镜像发布静态门禁已补齐：`image-release-contract.sh` 拒绝 `latest` 和旧 `20260904` 候选镜像，并检查 Dockerfile 构建期 `nginx -t`、Compose WAF 挂载和 `standard-rules.conf` include 链。
- [x] 2026-09-12 补齐域名/WAF 现场验收材料：`docs/operations/waf-domain-acceptance.md` 记录 HTTP-01/DNS-01/HTTPS/WAF/回滚矩阵，`deploy/acceptance/domain-acceptance.sh` 与 `deploy/openresty-waf/tests/domain-acceptance.sh` 提供只读 Host 路由、HTTPS、WAF 拦截、容器健康和日志检查脚本；两者均不替代真实执行证据。
- [x] 2026-09-12 新增 `deploy/acceptance/preflight.sh` 只读生产预检入口，统一检查 Docker、OpenResty/WAF 目录、数据库 Compose、域名解析、ACME 工具和浏览器自动化前置条件；本机预检显示生产 WAF 挂载目录仍缺 `standard-rules.conf`，需通过新版 Server 启动/部署流程补齐后再执行真实 WAF 验收。
- [x] 2026-09-12 预检入口已补齐跨工作目录执行契约，并新增 `deploy/database/preflight.sh` 数据库迁移前只读门禁；后者检查 secret 权限、固定容器/网络/卷名称、旧 ZNMP 资源保护和运行 Compose 文件，不执行容器或卷变更。当前数据库预检为 `warnings=4 failures=0`，严格模式返回非零。
- [x] 2026-09-12 新增 `deploy/acceptance/readonly-evidence.sh`，用于目标环境集中留存脱敏证据：OpenResty `-t`、WAF 文件 hash、站点配置、数据库 Compose 静态校验、SQLite 网站/域名/运行态摘要、只读 API 状态、HTTP/HTTPS/WAF 请求状态和响应头摘要；自检验证不会泄露 `CID` 明文或响应体。
- [!] 2026-09-12 新增 `deploy/acceptance/site-reconcile.sh` 只读站点对账脚本，核对 SQLite 网站/域名记录、站点目录 `nginx/site.conf` 的 `server_name` 和站点 WAF 文件；无系统 `sqlite3` 时回退仓库内 Go `mode=ro` 导出器。当前实测 SQLite `websites=0`、`website_domains=0`，文件系统有 19 个 `site.conf`，结果为 `warnings=0 mismatches=19`；清单和处理计划见 [`2026-09-12-site-reconcile-current.md`](progress/2026-09-12-site-reconcile-current.md)。
- [!] 2026-09-12 复核 `docs/img/` 七张 WAF 参考图与前端七个路由、七个组件和 WAF API 保存链；页面源码未丢失，但浏览器截图像素验收、生产 OpenResty reload、真实域名/WAF 请求和站点数据对账仍未完成，详见 [`2026-09-12-waf-image-page-audit.md`](progress/2026-09-12-waf-image-page-audit.md)。
- [x] 新增 WAF 日志时间、状态、网站、关键词筛选和概览聚合测试；Go WAF/API 定向测试与前端 `npm run type-check` 通过。
- [x] 独立 WAF Docker 黑盒已完成：规则 observe/block、站点隔离、IP/CIDR 黑白名单优先级、实际 HTTP 状态码和 JSONL 审计均已验证。
- [!] GeoIP 真实查询仍依赖部署环境的 GeoIP 模块/变量；Lua 没有数据时保持缺失，不能把规则来源当作 IP 归属地。
- [!] 真实生产 OpenResty reload、WAF 请求拦截、域名部署、ACME HTTP-01/DNS-01 和生产发布回滚仍受 Docker Socket、OpenResty、DNS/ACME 凭据和生产维护窗口阻断。
- [>] 下一步：配置并验证真实 GeoIP 变量，再在可访问 Docker、域名和证书资源的环境执行 HTTP/HTTPS Host 路由、真实 WAF 请求矩阵及生产发布回滚演练；详见 [`2026-09-12-waf-follow-up.md`](progress/2026-09-12-waf-follow-up.md)。

## 2026-09-10 数据库容器化并行实施

- [>] PostgreSQL、Redis、MySQL/MariaDB 的独立 Compose 模板、容器内 CLI 执行器、`ContainerName` SQLite 资源模型、Compose runtime 状态解析和备份恢复计划已并行实现；当前代码证据见 [`2026-09-10-database-containerization.md`](progress/2026-09-10-database-containerization.md)。
- [x] 数据库备份真实执行器和 API 已接入受控目录、元数据记录、错误脱敏、真实 symlink 检查、同容器互斥锁和 Redis `docker cp` 原子落盘；真实 Docker 黑盒仍需有 Docker 权限的隔离环境执行。
- [!] 数据库容器/备份已有隔离 Docker 黑盒证据，但当前沙箱无法访问 Docker Socket，不能在本环境重跑或宣称生产 daemon、Compose 和恢复窗口已验收；完整 service 测试还受 `httptest` IPv6 监听权限限制。

## 2026-09-10 本轮推进

- 容器、Compose、网站（含 TCP/UDP stream）和独立 WAF 联合外部黑盒通过。
- MariaDB、PostgreSQL、Redis、MongoDB 隔离数据库真实生命周期黑盒通过；首次 MongoDB 未就绪由宿主磁盘不足引起，清理 Go 缓存后复测通过。
- 修复容器列表手动刷新后停止态资源仍显示加载图标的问题；停止/退出/创建/dead 容器的 CPU、内存、IO 统一显示为 0，运行态继续使用真实 Docker stats。前端类型检查通过。
- 防火墙继续推进：firewalld/UFW 规则库存、UUID 删除/更新已接入真实 CLI；Docker 防护初始化/绑定/解绑及 SQLite 策略同步已接入 `WORKMESH_DOCKER`/`DOCKER-USER`，三种策略模式均有真实规则转换和失败回滚。真实 firewalld/UFW、IPv6/nftables 和 Docker DNAT 拦截仍需隔离主机验收。
- 日志继续推进：SQLite 保留清理接入后台维护周期并写入 `/internal/log-retention` 审计；网站旧日志接口读取失败不再回退内存快照。
- 主机 Supervisor 工具已接入真实 `supervisord`/`supervisorctl`：状态、配置读写、初始化、服务操作、进程查询/增删改启停和日志文件读写均使用参数数组及原子文件替换；缺少守护进程返回 `503`，写操作要求 `WORKMESH_ALLOW_HOST_MUTATION=1`，新增隔离错误边界测试。
- 修正容器镜像页面请求契约：拉取、推送、标签、批量删除、导入/导出和构建现在读取前端实际的 `repoID`、`imageName`、`tagName`、`sourceID`、`tags`、`names`、`paths`、`from` 字段；认证仓库通过 `--password-stdin` 登录，密码不进入 argv，新增 fake Docker 参数回归测试。
- 镜像构建兼容旧版 `name`/`image` 字段，推送时自动创建仓库目标 tag；临时 `registry:2` 已完成匿名及 bcrypt Basic Auth build、login、push、logout 和目标镜像核验，registry、镜像和标签均已清理；push 失败时自动删除临时 tag，并有 fake CLI 顺序回归测试。
- 节点 HTTP 透传增加受控断线恢复：GET/HEAD/OPTIONS 及带 `Idempotency-Key` 的写请求在传输错误后可重新签名重试，普通写请求不自动重放；断线恢复和非幂等写保护定向测试通过。真实双节点 Gateway/代理网络仍未执行。
- Gateway 本地客户端新增断线恢复回归：模拟注册、Gateway 暂时不可达及恢复，绑定保持且恢复阶段只发送心跳、不重复注册；真实主次节点联调仍未执行。
- 设置 `key/value` 写入新增类型归一化与边界校验：端口、会话/监控周期落为数字，代理/SSL/区域/主题/IP/CIDR/绑定域名按契约校验，非法值返回 `INVALID_SETTING`；定向测试通过。

仍未完成：ACME HTTP-01/DNS-01 真实签发、主次节点 Gateway 联调、SSH 代理断线重放与权限边界、firewalld/UFW 规则写入、转发/Docker 策略跨后端同步、多服务 Compose 高级编排，以及生产制品发布/回滚。Go/Node/Python/Java/.NET/PHP 隔离运行时生命周期、PHP 扩展/FPM/Supervisor、容器及 SSH PTY、基础镜像 build/save/load、匿名和认证 registry push、网络/卷基础生命周期已有外部证据；主机 Supervisor 代码已接入，但本机未安装守护进程，真实守护进程生命周期仍保持 `not-run`。

[x] 2026-09-09 数据库同步与容器资源继续推进：MongoDB 受控 `mongosh` 已修正非法 collection/删除用户语义，完成真实隔离生命周期和远程列表同步；PostgreSQL、MySQL/MariaDB 远程数据库列表同步、MySQL 变量查询/更新及 root 访问接口已实现并通过定向测试。数据库外部黑盒在磁盘容量允许时通过；本轮后续 MariaDB 重跑因宿主根分区满而阻塞，详见 [`2026-09-08-database-mvp.md`](progress/2026-09-08-database-mvp.md)。

[x] 2026-09-08 网站高级设置隔离黑盒验收完成：修正路由缺口扫描器对动态段、循环注册、统一分发器和下载方法的误报；网站设置 include 同次更新生效；删除清理完整站点目录；Basic Auth 密码仅保存哈希和 `hasPassword`，兼容非 root OpenResty worker；临时 OpenResty/上游容器完成静态、反代、CORS、真实 IP、重定向、Basic Auth、HTTPS、配置回滚和删除清理。全量 Go、race、vet、759 条路由和 diff check 通过。ACME、生产 OpenResty reload、TCP/UDP、运行时站点和生产发布仍未执行。详见 [`2026-09-08-website-advanced-blackbox.md`](progress/2026-09-08-website-advanced-blackbox.md)。

[x] 2026-09-08 首版基础资源集成完成：网站创建/更新/删除增加 OpenResty 语法校验与失败回滚；容器列表、详情、镜像、Compose 生命周期使用真实 Docker CLI，任务状态写入 SQLite，Docker 不可用返回 503；数据库 MySQL 元数据、用户、授权、变量、删除预检使用共享 SQLite。主控已统一通过 `GOWORK=off go test ./...`、`go test -race ./...`、`go vet ./...`、759 条路由契约和 `git diff --check`。真实域名、Docker 生命周期、远程数据库和 ACME 仍需授权隔离资源黑盒验收，不能据此宣称生产全量上线。详见 [`2026-09-08-mvp-priority.md`](progress/2026-09-08-mvp-priority.md)、[`2026-09-08-container-mvp.md`](progress/2026-09-08-container-mvp.md) 和 [`2026-09-08-incomplete-work-register.md`](progress/2026-09-08-incomplete-work-register.md)。
[x] 2026-09-10 防火墙与 SSL 路由缺口复核完成：防火墙规则 check/delete/update/reset/native detail/sync/preview/sync task 均确认使用真实后端库存和命令执行；新增 iptables 规则重排及任务执行状态查询；SSL push 按原系统契约校验证书状态和节点选择，缺少多节点执行器时明确返回 503；实现扫描 759/759，无 pending/partial。真实 firewalld/UFW 内核写入、ACME 签发、节点推送和生产验收仍保持 `not-run`。

[x] 2026-09-07 P2/P3 专项集成门禁完成：日志操作/登录/任务查询改为 SQLite 条件分页；隔离发布演练验证 Ed25519/SHA-256 制品验签、13 个迁移双启动幂等、失败数为 0 和审计哈希一致；终端 WebSocket 完成本地 TCP 握手、鉴权、命令输出、关闭码和资源释放测试。主控统一执行 Go 全量、race、vet、Node 7 项、759 路由和 diff 门禁均通过。真实生产业务仍需管理员会话、Docker/域名/ACME/节点凭据，未宣称全部上线。详见 [`integration-test-latest.md`](progress/integration-test-latest.md)、[`2026-09-07-upgrade-migration-readiness.md`](progress/2026-09-07-upgrade-migration-readiness.md)、[`2026-09-07-terminal-ws-readiness.md`](progress/2026-09-07-terminal-ws-readiness.md)。

并行状态：本批次共 4 个席位，主控 1 个；日志 SQL、升级迁移、终端 WS 3 个专项均已交付。下一批 AUDIT-03、REL-02、GATEWAY-02 已细化到生产就绪看板，待席位重新分配后并行执行；唯一全量测试负责人仍为主控。

发布回滚操作清单已新增：[`2026-09-07-release-rollback-checklist.md`](progress/2026-09-07-release-rollback-checklist.md)。

Gateway 下一批已细化为 6 个子任务（本地状态机、TCP 协议、v2 handler、任务透传、真实注册/心跳、真实跨节点任务）：[`2026-09-07-gateway-parallel-work.md`](progress/2026-09-07-gateway-parallel-work.md)。

代码质量下一批已细化为 QUALITY-01～04：先拆分日志/终端超长文件，再拆分网站和运行时方法，最后补齐中文注释；不改变 API、SQLite 或 OpenResty 语义，详情见生产就绪看板。

[x] 2026-09-07 AUDIT-03 日志策略专项完成：七类日志统一真实数据源，查询/导出响应执行递归脱敏，增加路径/query、嵌套集合和网站日志内容脱敏；SQLite 保留清理按时间和主键有界执行，缺少可选表安全跳过。隔离定向测试通过，生产清理仍待维护窗口。详见 [`2026-09-07-audit-policy.md`](progress/2026-09-07-audit-policy.md)。

[x] 2026-09-07 审计策略纳入后的最终门禁复核通过：Go 全量、race、vet、Node 契约 7/7、759 条路由和 `git diff --check` 全部通过；前端未变更，不重复编译。当前待执行项仍是需要外部资源的真实 HTTP/WS 黑盒、六类运行环境、ACME、主次 Gateway 和生产发布窗口。

[x] 2026-09-07 代码结构继续收敛：系统日志分页/文件读取从 `functional_logs.go` 拆至 `functional_system_logs.go`，原文件降至 465 行；结构变更后的完整门禁再次通过。质量扫描已无超长文件，剩余 88 个超长函数和 781 项中文文档注释缺口，继续按 QUALITY-01～03 并行处理。

[x] 2026-09-07 应用目录职责继续收敛：语义化版本解析闭包提取为独立函数，并补齐目录读取、远程刷新和标识辅助函数中文注释；应用定向测试通过，质量违规降至 860。随后执行的完整 Go/race/vet、Node 契约、759 路由和 diff 门禁全部通过。

[x] 2026-09-07 容器路由职责继续收敛：将无需 Docker 命令的查询分派提取为 `handleContainerQuery`，补齐容器状态、JSON 行解析、创建时间和资源名称辅助函数注释；容器定向测试通过，质量违规降至 854。随后完整门禁再次通过。

[>] 2026-09-07 首发正式使用优先级已明确：先完成核心平台基线门禁，再按“静态网站+域名+HTTPS → 反代/运行时 → 升级迁移回滚 → 日志审计/WS/主次节点”放行；最小可用、业务可用、正式生产发布三档标准见 [`2026-09-07-formal-first-release-priority.md`](progress/2026-09-07-formal-first-release-priority.md)。

[>] 2026-09-07 日志审计生产就绪评估完成：操作/登录/任务日志已有真实 SQLite 存量，但系统日志契约、SSH 日志、SQL 分页、脱敏与保留策略仍未达到正式放行；详情见 [`2026-09-07-logs-production-readiness.md`](progress/2026-09-07-logs-production-readiness.md)。

[>] 2026-09-07 SYSLOG-01 定向实现与验证完成：`POST /api/v2/logs/system/read` 增加真实 journal/file 读取、分页游标、过滤和 v2 `source/items/hasMore/nextCursor` 响应，同时保留 `path/content` 兼容字段；`node/api` 相关定向测试通过，待主控全量门禁和真实系统日志环境验收。详见 [`functional_logs.go`](../../node/api/functional_logs.go) 与 [`integration-test-latest.md`](progress/integration-test-latest.md)。

[>] 2026-09-07 正式使用执行计划已细化为 P0 静态网站、P1 反代/六类运行时、P2 升级迁移回滚、P3 日志审计与 WS/主次节点四个批次；每批均有智能体任务 ID、依赖、证据和 5 小时时间盒。当前优先 P0，外部凭据和隔离资源到位后由主控统一执行真实写操作与唯一全量门禁。详情见 [`2026-09-07-formal-execution-plan.md`](progress/2026-09-07-formal-execution-plan.md)。

[>] 2026-09-08 当前 4 个并行席位均在工作：主控负责集成和唯一全量门禁；`quality_runtime` 进入运行时质量批次；`audit_policy` 进入 Relay/fallback/契约审计批次；`release_checklist` 进入 Gateway v2 handler 契约测试批次。SQLite 启动迁移专项已交付，确认单进程、WAL、连接池、迁移 checksum 和二次启动 noop；Gateway 本地状态机专项已交付。原子任务、文件边界、定向测试和完成后轮换队列见 [`2026-09-07-production-readiness-board.md`](progress/2026-09-07-production-readiness-board.md) 与 [`2026-09-08-sqlite-startup-readiness.md`](progress/2026-09-08-sqlite-startup-readiness.md)。

[x] 2026-09-08 并行整改批次统一门禁通过：`go test ./...`、`go test -race ./...`、`go vet ./...`、Node 契约 7/7、759 条路由、严格安全契约扫描和 `git diff --check` 均通过。运行时结构、Gateway `link/status` 签名、SQLite 启动迁移和安全审计专项均有定向证据；质量规范扫描仍为 848 项违规，登录后 HTTP/WS、六类运行环境、网站全类型、主次节点和 ACME 真实验收仍未完成。详情见 [`integration-test-latest.md`](progress/integration-test-latest.md)。

[x] 2026-09-08 后续轮换批次完成并通过统一门禁：PHP 模板、Toolbox Handler 拆分、路由缺口矩阵和 v2 写请求参数审计均已交付；`go test ./...`、`go test -race ./...`、`go vet ./...`、Node 契约 7/7、759 条路由、严格安全扫描和 `git diff --check` 通过。质量扫描降至 843 项（中文文档 773、超长函数 70），仍未达到规范门禁；真实登录业务和外部资源验收保持未完成。详情见 [`integration-test-latest.md`](progress/integration-test-latest.md)、[`2026-09-08-route-gap-matrix.md`](progress/2026-09-08-route-gap-matrix.md) 和 [`2026-09-08-v2-write-parameter-audit.md`](progress/2026-09-08-v2-write-parameter-audit.md)。

[>] 2026-09-08 路由逐条审计继续推进：已生成 websites/runtime/databases/containers/hosts/cronjobs/logs/settings 的逐路由 501、partial、not-run 明细；当前最优先实现网站 P0 和系统/主机高频接口，不以兼容声明或路由注册替代真实处理器。详情见 [`2026-09-08-route-gap-detail.md`](progress/2026-09-08-route-gap-detail.md)。

[x] 2026-09-08 应用 v2 参数与 SQLite-only 批次完成：应用写请求拒绝 malformed/trailing JSON；共享 SQLite 模式下不生成 `apps.json`，状态落入 `app_store_state`/`app_installs`。新增应用拆分、Toolbox 和逐路由审计后，`go test ./...`、race、vet、Node 7/7、759 路由、严格安全扫描、逐路由明细和 diff check 均通过。质量扫描为 842 项，真实菜单业务接口仍按明细矩阵逐项推进。

[x] 2026-09-08 V2-PARAM-SETTINGS-01 完成：新增 `decodeSingleJSON`，设置/日志/备份/主机计划任务共用的 `requestMap` 与 `decodeJSON` 拒绝尾随 JSON；新增回归测试。全量 Go、race、vet、Node 契约、759 路由、严格安全扫描、逐路由明细和 diff check 再次通过。质量扫描为 842 项（378 文件、3081 函数），字段白名单和真实外部资源验收继续按矩阵推进。详情见 [`integration-test-latest.md`](progress/integration-test-latest.md) 和 [`2026-09-08-v2-write-parameter-audit.md`](progress/2026-09-08-v2-write-parameter-audit.md)。

[x] 2026-09-07 P2 隔离迁移演练完成：通过临时制品验签/原子安装和两个独立启动子进程验证 13 个启动迁移，第二次启动关键迁移均为 `noop`，失败审计为 0；未触碰生产服务。详情见 [`2026-09-07-upgrade-migration-readiness.md`](progress/2026-09-07-upgrade-migration-readiness.md) 与 [`startup_rehearsal_test.go`](../../cmd/workmesh-server/startup_rehearsal_test.go)。

[x] 2026-09-07 15:29 沙盒任务职责拆分后的最终冻结门禁通过：Go 全量/race/vet、Node 7 项、759 路由、diff check，以及 systemd、健康、就绪、OpenResty 只读检查均通过；当前 Go 源码集合 SHA-256 `a29a613a…628e317`，测试前后一致。质量报告为 362 个 Go 文件、2897 个函数、844 项违规（中文注释 758、超长函数 86），规范门禁仍未清零；真实登录后 HTTP/WS、运行时、网站、主次节点、ACME 和升级迁移继续为 `blocked/not-run`。详情见 [`integration-test-latest.md`](progress/integration-test-latest.md)。

[>] 2026-09-07 正式使用优先执行看板已建立：网站/OpenResty/HTTPS 为第一队列，六类运行时为第二队列，升级迁移为第三队列；已列明并行席位、依赖、验收证据和生产操作边界。详情见 [`2026-09-07-production-readiness-board.md`](progress/2026-09-07-production-readiness-board.md)。

[x] 2026-09-07 网站与运行时正式使用准备完成：网站只读盘点确认 `znmp.sopvip.com` 静态 HTTPS 真实可访问，但反代、运行时、PHP、TCP/UDP 测试资源尚未监听；六类运行时生命周期及 PHP-FPM/扩展/Supervisor 的真实验收矩阵已整理，当前均按资源缺失保持 `blocked/not-run`。详情见 [`2026-09-07-website-production-readiness.md`](progress/2026-09-07-website-production-readiness.md) 和 [`2026-09-07-runtime-production-readiness.md`](progress/2026-09-07-runtime-production-readiness.md)。

[x] 2026-09-07 15:15 AI/应用路由拆分及注释补齐后的最终冻结门禁通过：Go 全量/race/vet、Node 7 项、759 路由、diff check，以及 systemd、健康、就绪、OpenResty 只读检查均通过；当前 Go 源码集合 SHA-256 `d366e237…53dd14`，测试前后一致。质量报告为 362 个 Go 文件、2895 个函数、845 项违规（中文注释 758、超长函数 87），规范门禁仍未清零；真实登录后 HTTP/WS、运行时、网站、主次节点、ACME 和升级迁移继续为 `blocked/not-run`。详情见 [`integration-test-latest.md`](progress/integration-test-latest.md)。

[x] 2026-09-07 15:02 应用生命周期和 AI 路由分派拆分后的冻结门禁通过：Go 全量/race/vet、Node 7 项、759 路由、diff check，以及 systemd、健康、就绪、OpenResty 只读检查均通过；当前 Go 源码集合 SHA-256 `6a478e14…b90736`，测试前后一致。质量报告为 362 个 Go 文件、2895 个函数、853 项违规（中文注释 766、超长函数 87），规范门禁仍未清零；真实登录后 HTTP/WS、运行时、网站、主次节点、ACME 和升级迁移继续为 `blocked/not-run`。详情见 [`integration-test-latest.md`](progress/integration-test-latest.md)。

[x] 2026-09-07 01:40 网站测试、应用测试和网站扩展职责拆分后的最终冻结门禁通过：Go 全量/race/vet、Node 7 项、759 路由、diff check，以及 systemd、健康、就绪、OpenResty 只读检查均通过；当前 Go 源码集合 SHA-256 为 `5956a7de…01284`。质量报告仍有 886 项违规，真实登录后业务、WS、运行时、网站、主次节点、ACME 和升级迁移未验收，不能宣称正式上线完成。详情见 [`integration-test-latest.md`](progress/integration-test-latest.md)。

[x] 2026-09-07 00:31 CoreService、网站生命周期和启动入口职责拆分后的冻结门禁通过：`go test ./...`、`go test -race ./...`、`go vet ./...`、Node 7 项、759 条路由、diff check、systemd、健康检查和 OpenResty 均通过；该阶段 Go 源码集合 SHA-256 `20e1981b…f5a18` 前后一致。历史质量报告为 900 项，后续网站/应用测试拆分已在 01:40 快照更新；登录后 HTTP/WS、六类运行时、网站全量设置、主次节点、ACME 和升级迁移真实验收仍为 `blocked/not-run`。

[x] 2026-09-07 CoreService 结构批次完成：`core.go` 303 行，新增持久化、Passkey、分组职责文件；网站生命周期降至 496 行并新增操作辅助文件；启动入口提取 `runServer`、`initializeSharedServices`、`runHTTPService`。定向测试通过，详情见 [`2026-09-07-core-service-structure-quality.md`](progress/2026-09-07-core-service-structure-quality.md) 和 [`2026-09-07-core-website-lifecycle-quality.md`](progress/2026-09-07-core-website-lifecycle-quality.md)。

[>] 2026-09-06 23:30 计划任务与 Gateway 结构批次完成：`host_container_cron.go` 260 行、`gateway.go` 445 行，新增独立路由/持久化/运行时文件；定向测试和 `git diff --check` 通过。冻结交接哈希已更新，等待唯一集成负责人执行完整门禁。

[>] 2026-09-06 当前冻结交接已记录：tracked diff `8ffca803…ab7759`、untracked `26d21a52…f1df9c6`、Go 源码集合 `abf0dd00…e25eb`；角色、主机和节点透传变更待唯一集成负责人按固定顺序门禁。详情见 [`2026-09-06-freeze-handoff.md`](progress/2026-09-06-freeze-handoff.md)。

[>] 2026-09-06 22:59 角色控制器辅助逻辑拆分完成：`role.go` 488 行，新增 `role_helpers.go` 42 行；角色/节点/fencing 定向测试通过，等待唯一集成负责人覆盖当前冻结快照。

[>] 2026-09-06 22:36 主机运维路由拆分完成：`host_container_cron.go` 495 行，新增 `host_monitor_interfaces.go` 84 行；主机/容器/计划任务定向测试通过，等待唯一集成测试负责人对当前冻结快照重跑完整门禁。

[>] 2026-09-06 21:40 新增节点透传辅助文件拆分：`node_relay.go` 529 行降至 450 行，`node_relay_helpers.go` 91 行；定向 Relay/Forward/Node 测试通过，完整门禁待源码冻结后由唯一集成测试负责人重跑。

[x] 2026-09-06 21:33 冻结快照完整门禁通过：21:21:28/21:33:01 测试输入集合 SHA-256 前后一致（集合 `d188169a…9e7117b4`），Go 全量/race/vet、Node 7 项、759 路由、diff check 与生产只读检查通过。本轮覆盖网站域名配置拆分和语言包注释批次；源码未变更不重复门禁。[!] 真实登录后 HTTP/WS、运行时/网站、主次节点和 ACME 验收仍为 `blocked/not-run`；生产健康不等同当前源码已部署。证据及下一步见 [`integration-test-latest.md`](progress/integration-test-latest.md)。

网站域名目录与独立配置代码已完成，WorkMesh Server 与自有 WAF 已部署；`znmp.sopvip.com` 使用独立域名目录和 80/443 入口，详见 [`2026-09-04-website-paths.md`](progress/2026-09-04-website-paths.md)。

本次开发：已完成运行环境、网站、应用、容器、计划任务、日志和兼容路由的职责拆分，统一 SQLite 运行态、任务并发保护、网站启动配置恢复、真实 HTTP-01 certbot 流程和 OpenResty stream 占位清理。网站模板与产物已新增 `0015-website-template-relational` 迁移，预览和产物生成使用真实模板文件及 SQLite 关系表，详见 [`2026-09-06-website-template-relational.md`](progress/2026-09-06-website-template-relational.md)。本次后端候选版本已在门禁通过后备份并原子部署，制品 SHA256=`4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f`，备份目录为 `/opt/workmesh-server/backups/deploy-20260906T090558Z-website-template`；迁移账本、健康、HTTPS、OpenResty 和 SQLite 完整性已完成只读核对。该证据只覆盖网站上线基础，不等同于登录后全量 HTTP/WS 业务验收。

当前顶层目录尚未整体搬迁；本轮先完成兼容性安全的职责拆分，洋葱架构的实际目录迁移顺序和边界见 [`onion-migration-2026-09-05.md`](../architecture/onion-migration-2026-09-05.md)。

主窗口菜单、子页面和路由入口已从真实前端源码提取为 13 个主菜单、109 个入口，验收顺序和状态见 [`2026-09-05-main-window.md`](progress/2026-09-05-main-window.md)。

开发与测试已分离：所有全量门禁由唯一集成测试负责人执行，规则见 [`test-ownership.md`](test-ownership.md)；参考前端无变更时不重复编译。

当前并行任务角色、细分队列、唯一测试流水线和发布出口见 [`2026-09-06-agent-work-allocation.md`](progress/2026-09-06-agent-work-allocation.md)。

## 已完成

本轮新增 SQLite 运行时后台任务、任务日志、角色 epoch、节点清单、Gateway 绑定、链路同步及核心认证辅助状态，详情见 [`2026-09-04-sqlite-runtime-tasks.md`](progress/2026-09-04-sqlite-runtime-tasks.md)。

| 状态 | 领域 | 完成摘要 | 证据 |
| --- | --- | --- | --- |
| [x] | 开发会话连续性规则 | 已增加状态索引、进度模板和断线恢复读取顺序 | [`2026-09-02-session-continuity.md`](progress/2026-09-02-session-continuity.md) |
| [x] | 单进程启动、健康检查和数据目录初始化 | 统一服务入口、`/health`、`/ready` 和运行目录初始化已具备测试 | [`README.md`](../../README.md)、[`cmd/workmesh-server/cli_test.go`](../../cmd/workmesh-server/cli_test.go) |
| [x] | 本地 Session、Cookie、Bearer 和 API Key 鉴权 | 控制面与节点执行面统一鉴权，未授权请求和 CSRF 场景有测试 | [`core-auth-session.md`](../api/core-auth-session.md)、[`security_middleware_test.go`](../../server/control/api/security_middleware_test.go) |
| [x] | 状态文件原子写入与重启恢复基础能力 | 已迁移领域使用临时文件和原子替换，关键服务具备持久化测试 | [`unified-server.md`](../architecture/unified-server.md)、[`core_persistence_test.go`](../../server/control/service/core_persistence_test.go) |
| [x] | OpenResty 容器探测与状态数组契约 | 容器化探测、版本/端口状态和空数组契约已验证 | [`website-openresty-waf.md`](../api/website-openresty-waf.md)、[`website_test.go`](../../server/node/service/website_test.go) |
| [x] | 控制面状态 SQLite 迁移补充 | AI、主机监控、文件辅助/分享、共享功能域和数据库管理旧 JSON 已一次性导入并归档；重启从 SQLite 恢复 | [`2026-09-03-backend-routes.md`](progress/2026-09-03-backend-routes.md) |
| [x] | PHP 创建表单与扩展模板对齐 | 扩展源及五组默认模板与 1Panel 一致，模板支持多选合并，CRUD 和重启恢复使用 SQLite | [`2026-09-03-php-runtime-parity.md`](progress/2026-09-03-php-runtime-parity.md) |
| [x] | 六类运行环境代码与自动化门禁 | 应用目录隔离、安全归档、真实 Compose 命令、SQLite、PHP FastCGI/Supervisor/慢日志、63 项扩展精确卸载事务和 Node 容器模块操作已实现并通过全量测试、vet、前端构建与契约扫描 | [`2026-09-03-runtime-parity.md`](progress/2026-09-03-runtime-parity.md) |
| [x] | 运行时与网站职责拆分 | `runtime_toolbox.go`、`website.go` 已按存储、校验、生命周期、渲染、WAF、证书和任务职责拆分；网站启动可从 SQLite 恢复空配置，stream 不再生成假上游，HTTP-01 使用真实 certbot webroot | [`2026-09-05-runtime-release.md`](progress/2026-09-05-runtime-release.md)、[`2026-09-05-website-include-isolation.md`](progress/2026-09-05-website-include-isolation.md)、[`integration-test-latest.md`](progress/integration-test-latest.md) |
| [x] | 路由与前端调用基线 | 1Panel 路由 759 条（GET 138、POST 617、HEAD 4）；前端静态 HTTP/WS 调用 389 条（HTTP 385、WS 4）；均保留来源哈希和 `not-run` 状态 | [`2026-09-05-quality-routes.md`](progress/2026-09-05-quality-routes.md)、[`../inventory/route-inventory-1panel.json`](../inventory/route-inventory-1panel.json) |
| [x] | 候选版本基础生产检查 | 最终后端候选版本已备份、原子部署并完成全量 Go、race、vet、路由契约和 SQLite 完整性检查；健康、就绪、WAF 配置和站点入口均有独立证据；测试结果集中记录，不把静态清单当作通过 | [`backend-release-latest.md`](progress/backend-release-latest.md)、[`backend-blackbox-latest.md`](progress/backend-blackbox-latest.md) |
| [x] | 网站模板与产物关系化上线基础 | 新增 `0015-website-template-relational`、真实 ZIP 预览/产物渲染和受控文件清理；源码门禁通过并已部署，生产迁移账本、模板表、健康、HTTPS、OpenResty 和 SQLite 完整性均已核对 | [`2026-09-06-website-template-relational.md`](progress/2026-09-06-website-template-relational.md)、[`website-templates.md`](../api/website-templates.md) |
| [x] | WAF 文件数据源收口 | WAF 运行态统一由 WorkMesh 自有 JSON/JSONL 文件管理，不依赖外部旧仓库、`1pwaf/data` 或旧 WAF SQLite 表；全局/站点配置、规则、黑白名单和日志接口已接入并通过 Go 全量/Race/Vet、前端构建及 759 条路由契约检查 | [`website-openresty-waf.md`](../api/website-openresty-waf.md)、[`2026-09-06-waf-sqlite.md`](progress/2026-09-06-waf-sqlite.md) |
| [x] | WAF 七页界面和控制面配置闭环 | 七个截图页面已逐页对齐；概览已接入真实 GeoJSON 地图，日志字段/筛选和站点规则弹窗已完成；保存链包含 OpenResty `-t`、reload、复检和失败回滚代码 | [`website-openresty-waf.md`](../api/website-openresty-waf.md)、[`2026-09-11-waf-page-runtime.md`](progress/2026-09-11-waf-page-runtime.md) |

## 未完成

当前逐路由和运行环境的未完成登记见 [`2026-09-08-incomplete-work-register.md`](progress/2026-09-08-incomplete-work-register.md)。其中明确区分源码 501、已有处理器但闭环不足、以及等待外部资源的 `not-run`，不能用自动化门禁通过替代正式验收。

| 状态 | 领域 | 当前缺口 | 下一步 | 详情 |
| --- | --- | --- | --- | --- |
| [>] | 主机、容器和计划任务完整迁移 | 容器 API 已按 Compose、查询、请求、镜像、统计、搜索和操作拆为 8 个职责文件；计划任务服务已按执行、调度、记录和传输拆为 5 个职责文件；仍需补齐生产平台差异、资源配额、远程驱动和大任务异步化 | 按首批迁移清单逐项补齐并验证真实 Docker/计划任务生命周期 | [`first-batch-host-container-cron.md`](../api/first-batch-host-container-cron.md) |
| [>] | Gateway 注册、心跳和跨节点任务透传 | 本地绑定可恢复；真实 Gateway 心跳、授权和跨节点任务仍需继续复验 | 配置真实凭据后复验心跳和任务透传 | [`deployment-verification-2026-08-31.md`](../migration/deployment-verification-2026-08-31.md)、[`link.md`](../api/link.md) |
| [>] | 终端与流式连接恢复 | 容器/SSH PTY 已有真实隔离证据，跨重启 SSE/代理断线重放和权限边界仍为 `partial` | 验证代理断线重放、跨重启任务回放和权限边界 | [`function-checklist.md`](../migration/function-checklist.md) |
| [>] | AI、应用和在线开发剩余路由 | AI 执行模块已按账号、Agent、MCP、操作、任务沙箱和路由目录拆分；应用已完成 9 文件职责拆分；AI/运行时/ACME 等仍存在 `partial`、`pending` 项 | 按清单逐路由补齐真实行为、持久化和测试 | [`2026-09-03-backend-routes.md`](progress/2026-09-03-backend-routes.md)、[`ai-apps.md`](../api/ai-apps.md)、[`function-checklist.md`](../migration/function-checklist.md) |
| [>] | 构建、部署与正式验收 | 历史部署记录包含健康检查和站点入口证据；当前沙箱对 `znmp.sopvip.com` 解析失败，无法复验 HTTP/HTTPS。Gateway 凭据、次节点链路、KVM、全量 HTTP/WS 和周期性 ACME 续期仍未完成 | 获取有效账号/节点凭据和可解析域名后，持续按矩阵执行真实请求并保留响应摘要 | [`backend-release-latest.md`](progress/backend-release-latest.md)、[`integration-test-latest.md`](progress/integration-test-latest.md)、[`2026-09-12-deployment-domain-env-directory-audit.md`](progress/2026-09-12-deployment-domain-env-directory-audit.md) |
| [!] | WAF/域名生产验收 | WAF 页面源码、JSON 文件运行时和本地 fake/隔离验证已有证据；当前工作区无法访问 Docker Socket，生产 WAF 挂载缺 `generated/standard-rules.conf`，真实生产 OpenResty reload、WAF 请求拦截、域名部署、ACME HTTP-01/DNS-01、发布回滚仍未完成 | 先恢复 Docker/OpenResty、隔离域名和证书资源，再执行真实请求矩阵和回滚验收 | [`2026-09-11-waf-page-runtime.md`](progress/2026-09-11-waf-page-runtime.md)、[`2026-09-08-acme-acceptance.md`](progress/2026-09-08-acme-acceptance.md)、[`../operations/waf-domain-acceptance.md`](../operations/waf-domain-acceptance.md) |
| [!] | 站点目录和 SQLite 对账 | 当前生产 SQLite 可读但 `websites=0`、`website_domains=0`，文件系统存在 19 个孤立 `site.conf`；站点目录与数据库尚未达成一致 | 先只读分类，再由人工确认导入或清理；完成前不得对这些目录做生产 reload/删除 | [`../../deploy/acceptance/README.md`](../../deploy/acceptance/README.md)、[`2026-09-12-deployment-domain-env-directory-audit.md`](progress/2026-09-12-deployment-domain-env-directory-audit.md) |
| [!] | WAF 七页截图验收 | 七张图片与七个页面路由/组件已静态对应，浏览器截图、交互和保存后状态复核未完成 | 准备浏览器自动化依赖后逐页截图，并验证保存后 `effective` 和实际请求行为 | [`2026-09-12-waf-image-page-audit.md`](progress/2026-09-12-waf-image-page-audit.md) |
| [>] | 目录结构物理迁移 | 职责拆分和 repository 边界已有进展，顶层物理目录仍未按计划整体迁移 | 先完成 OpenResty、域名、数据库和发布回滚验收，再按迁移批次执行并复跑路由/前端调用门禁 | [`../architecture/onion-migration-2026-09-05.md`](../architecture/onion-migration-2026-09-05.md)、[`2026-09-12-deployment-domain-env-directory-audit.md`](progress/2026-09-12-deployment-domain-env-directory-audit.md) |
| [>] | WAF 日志真实数据源 | 页面字段、筛选、ModSecurity 审计归一化和 combined `access.log` 解析已完成；真实 GeoIP 变量仍需部署验收 | 配置 GeoIP 模块/变量并执行真实日志回归 | [`2026-09-12-waf-follow-up.md`](progress/2026-09-12-waf-follow-up.md) |
| [>] | 六类运行环境生产验收 | Go/Node/Python/Java/.NET/PHP 生命周期、PHP-FPM 多版本、扩展和 Supervisor 已有隔离 Docker 证据；指定生产实例、编辑/日志/权限边界和宿主差异仍未在目标环境执行 | 获得 Docker 操作授权后按六类运行时矩阵逐实例验收，失败时保留隔离环境证据并回滚 | [`2026-09-03-runtime-parity.md`](progress/2026-09-03-runtime-parity.md) |
| [!] | 代码规范门禁 | 当前质量扫描：372 个 Go 文件、2986 个函数、854 项违规（中文注释 768、超长函数 86、超长文件 0）；应用目录和容器请求职责已补齐注释，门禁仍未通过 | 并行拆分剩余超长函数并补充准确中文注释；不得批量添加无意义注释 | [`../inventory/quality-report.json`](../inventory/quality-report.json)、[`2026-09-07-production-readiness-board.md`](progress/2026-09-07-production-readiness-board.md) |
| [ ] | 全量完成门槛与最终发布验收 | 登录后业务闭环、六类真实运行环境、网站设置、WS/主次节点、HTTP-01 续期和 759 条路由实测尚未全部满足 | 完成所有阻断项后重新构建、部署、回归和发布签字 | [`TODO.md`](../../TODO.md)、[`2026-09-05-http-ws-test-matrix.md`](progress/2026-09-05-http-ws-test-matrix.md) |

## 使用规则

- `[x]` 只表示有真实实现、错误处理、持久化依据和测试/验收证据的事项。
- `[>]`、`[ ]` 和 `[!]` 都表示下次应继续处理的事项；进入任务后再创建或更新 `progress/` 下的详情文件。
- 完成里程碑后同步更新本页的状态、摘要、证据和下一步；不重复复制详情文档内容。
