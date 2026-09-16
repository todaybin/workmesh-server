<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 未完成工作登记（2026-09-08）

本登记区分“源码缺失/明确 501”“已有处理器但未完成业务闭环”“真实环境尚未执行”三类，不能把路由注册或定向测试误认为正式可用。

## 源码缺失或明确 501

2026-09-12 复核：实现扫描已达到 759/759 `implemented`，当前没有静态扫描识别的 `pending` 或 `partial`。此前防火墙 9 项、网站 LBS 删除和 SSL resolve/push 的结果均属于扫描器上下文或空集合误报，已按真实路由语义修正。此结论只表示源码路由闭合，不替代下文外部 `not-run` 验收。

依据 `node test/contract/route-gap-detail.mjs --write --summary` 当前统计：

| 功能域 | 总路由 | 明确 501/缺失 | partial | not-run |
| --- | ---: | ---: | ---: | ---: |
| Websites | 81 | 0 | 0 | 81 |
| Runtimes | 23 | 0 | 0 | 23 |
| Databases | 17 | 0 | 0 | 17 |
| Containers | 23 | 0 | 0 | 23 |
| Hosts | 42 | 0 | 0 | 42 |
| Cronjobs | 4 | 0 | 0 | 4 |
| Logs | 5 | 0 | 0 | 5 |
| Settings | 11 | 0 | 0 | 11 |

当前没有静态扫描识别的明确 501；8 个功能域共 206 条路由仍标记 `not-run`，表示尚未由该脚本逐条发送真实 HTTP/WS 请求，不表示处理器缺失。下一步优先顺序为：网站域名/ACME/WAF 生产验收、Docker 与数据库外部环境、主次节点 Gateway、日志/WS 权限边界和生产发布回滚。

## 已有代码但闭环不足

- 运行期保存错误边界已继续补齐：应用安装、运行时安装/Node 模块任务、设置/告警快照、容器 Compose/仓库/模板和网站扩展公共状态的关键写操作现在会回滚内存并返回错误；其余业务域仍需按同一规则复核，不能把本批局部收口视为全局完成。
- 文件管理长任务的压缩、解压和移动已从同步兼容处理改为可取消异步任务，状态和取消结果进入任务日志；分片上传、远程下载、媒体转换等其他文件长任务仍需分别完成真实中断和恢复验收。
- Gateway 绑定/授权和 Website 创建、更新、删除、stream 更新的持久化失败边界已完成隔离回归；域名重命名失败会恢复目录与 `site.conf`，WAF/OpenResty 生效失败会恢复 WAF 文件和网站状态。真实双节点 Gateway、生产 OpenResty reload 和域名/ACME 仍保持 `not-run`。
- 2026-09-12 追加：网站 HTTPS、域名增删、rewrite、日志开关、运行目录、负载均衡文件、默认页面同步和 WAF 全局/站点规则的补偿错误已不再静默吞掉；失败会恢复内存/文件快照并把补偿失败并入返回错误。真实生产 OpenResty reload 仍未执行。
- MongoDB 已接入受控 `mongosh` 执行器及创建、搜索、绑定、改密、权限、root 密码、删除检查/删除和远程同步接口；隔离 `mongo:8-noble` 基础生命周期已通过。备份恢复、集群编排和跨节点数据库操作仍未完成。
- MySQL/MariaDB、PostgreSQL 远程数据库列表同步已实现；MySQL 变量真实查询/更新、root 访问权限已实现。复杂变量兼容、集群编排、备份恢复和跨节点操作仍需验收。
- 设置接口已补齐 key/value 的数值、枚举、IP/CIDR、代理地址和绑定域名校验，并将端口/超时等字段按前端类型持久化；端口/绑定现在明确返回 `pending-restart`，SSL 导入校验证书与私钥，`ssl/reload` 无安全 hook 时返回 `503` 而不伪造 `reloaded=true`。仍需补齐真实进程重启/reload、升级发布源和生产设置黑盒。
- 备份账号 create/update/delete 的保存失败回滚、备份记录删除顺序和远端删除失败恢复已补齐，并有一致性测试；真实云端 provider、生产备份目录和恢复窗口仍需外部验收。
- 终端 AI 设置已从占位响应改为 `node_settings` 持久化，补齐默认值、账号必填、前缀和风险命令校验；真实 AI provider 执行链仍需单独验收。
- 网站创建回滚已补 OpenResty 二次校验；隔离黑盒已覆盖静态站、默认文档全套、反代、CORS、真实 IP、重定向、Basic Auth、HTTPS、防盗链、限流保存回填、负载均衡前端字段、负载均衡文件、PHP-FPM 运行时、子网站父目录访问、TCP/UDP 双向访问、WAF 规则和配置失败回滚/删除清理；空 upstream 删除现在会移除托管片段，避免生成非法空 upstream；仍需覆盖其他运行时语言、ACME HTTP-01 和生产域名/OpenResty。
- 日志已有 SQLite 查询、文件 source、统一脱敏和保留清理基础；生产维护窗口、真实登录后的七类日志 HTTP/WS 黑盒、journal/logrotate 外部策略仍需最终验收。
- 主次节点 Gateway 本地状态机和签名测试已有；新增 `TestGatewayHeartbeatDisconnectRecoveryKeepsBinding` 验证注册后 Gateway 断开/恢复时保留绑定并重新心跳，不重复注册。真实次节点联调、任务回放和权限切换未执行。
- 节点 HTTP 透传对 GET/HEAD/OPTIONS 及带 `Idempotency-Key` 的写请求支持受控断线重试；每次重试重新生成 nonce/签名，无幂等键的写请求不重放。`TestNodeRelayRetriesReplaySafeRequestAfterDisconnect` 和 `TestNodeRelayDoesNotReplayNonIdempotentWriteWithoutKey` 已通过。真实双节点代理断线、跨进程任务回放和权限边界仍未完成。
- 终端 WS 本地 TCP 握手测试已有；容器终端和 SSH PTY 已有隔离外部证据，代理 WebSocket 断线重连和权限边界仍未完成。
- 主机 Supervisor 工具已接入真实 `supervisord`/`supervisorctl` 配置、进程和日志文件操作；当前主机未安装守护进程，真实服务生命周期仍保持 `not-run`。
- 六类运行环境（Go/Node/Python/Java/.NET/PHP）生命周期、PHP-FPM 多版本、扩展和 Supervisor 已有隔离 Docker 证据；指定生产实例、编辑/日志/权限边界和宿主差异仍未在目标环境验收。
- 容器基础黑盒已完成临时容器、Compose、网络/卷生命周期、停止态资源零值及 Docker 依赖故障 503；多服务高级编排、日志流和生产 daemon 仍未验收。
- SQLite 迁移隔离演练已完成；生产制品替换、WAL/SHM 备份、失败回滚和重启后验收未执行。
- 网站普通页面与 WAF 七页的源码和隔离运行链已完成；`docs/img/` 七张图对应七个 WAF 页面路由，不是前端页面缺失。WAF Server 已接入 ModSecurity JSON/JSONL/数组/对象、Nginx combined `access.log`、IP 归属地筛选、IP 组/Redis/默认规则 Lua 运行时路径；真实 GeoIP 变量、域名部署测试、HTTP/HTTPS Host 路由、ACME HTTP-01/DNS-01、生产 WAF 拦截、Docker Compose 和发布回滚仍未完成。

## 外部条件阻塞但不等于完成

以下需要维护窗口、管理员会话、CSRF、隔离域名、可清理 Docker 资源或主次节点凭据：真实 `/api/v2` HTTP/WS 黑盒、Let's Encrypt HTTP-01、OpenResty reload、容器生命周期、Gateway 联调、生产发布。没有这些资源时必须记录 `not-run`，不得改成固定成功或模拟数据。

## 已完成的自动化证据

- `GOWORK=off go test ./...` 通过。
- `GOWORK=off go test -race ./...` 通过。
- `GOWORK=off go vet ./...` 通过。
- Node 契约、路由扫描、安全扫描和 `git diff --check` 最近一次通过。

自动化门禁通过只证明当前代码可编译、定向契约和本地测试通过，不证明上述真实业务已上线。

## 2026-09-09 本轮推进

- PHP 运行时扩展详情使用真实运行时记录；若运行时绑定容器则通过 `docker exec` 查询已安装扩展，并返回支持扩展状态；容器不可用时返回明确错误。
- `GET /api/v2/settings/daemonjson` 返回与容器 daemon JSON 读写共用的实际配置路径，不再由兼容占位处理。
- 容器文件内容、目录大小、目录搜索、文件/目录下载和日志下载补齐前端协议层；支持 `containerID` 字段，下载返回 Blob/`tar.gz`，命令失败保留 Docker 依赖错误语义。
- 定向测试：`GOWORK=off go test ./node/api ./node/service`（设置 `WORKMESH_OPENRESTY_BIN=/tmp/workmesh-test-missing-openresty`）通过；`go vet ./...`、759 条路由契约和 `git diff --check` 通过。生产 Docker/OpenResty 操作和未覆盖网站项仍保持 `not-run`。
- 隔离 Docker 黑盒已执行：`WORKMESH_CONTAINER_EXTERNAL_TEST=1 GOWORK=off go test ./node/api -run '^TestExternalContainerFileAndLogEndpoints$'` 通过，覆盖临时 `workmesh-acceptance-*` 容器的内容、大小、搜索、文件 Blob 下载和日志 Blob 下载；测试结束后容器无残留。
- 网站隔离黑盒已执行：`WORKMESH_WEBSITE_EXTERNAL_TEST=1 GOWORK=off go test ./node/api -run '^TestExternalWebsiteLifecycle$'` 通过，验证 `*.cs.sopvip.com` 隔离域名、OpenResty `-t/reload`、默认文档、防盗链、反代/CORS/真实 IP、限流回填、负载均衡、重定向、Basic Auth、HTTPS、失败回滚和删除清理；测试结束后容器、网络无残留。
- 本轮补充子网站外部验收：同一隔离黑盒创建父站点下的 `/child` 目录和 `child-*.cs.sopvip.com` 子网站，真实 Host 请求返回父站点目录内容；父站点存在子网站时删除被拒绝，删除子网站后父站点和全部目录均清理。子网站 TCP/UDP 仍未执行，保持 `not-run`。
- 本轮补充 PHP-FPM 外部验收：在隔离 Docker 网络启动 `1panel-php-fpm:7.4.33`，以相同绝对路径挂载站点目录，创建 `runtime/fpm` 网站并经 OpenResty `-t/reload` 后真实请求 `index.php` 返回 PHP 内容；FPM 容器、网站目录和配置均在测试结束清理。PHP-FPM 多版本、扩展安装和 Supervisor 生命周期仍未完成。
- 本轮补充 TCP/UDP 外部验收：在隔离 Docker 网络启动临时 Python TCP/UDP 上游和 OpenResty stream 容器，分别创建 `type=stream` 的 TCP 与 UDP 网站，验证真实 TCP 返回、UDP 回显、`stream.conf` 的监听和 upstream、OpenResty `-t/reload`、SQLite 配置回填及删除清理；测试结束后容器、网络和站点目录无残留。端口冲突、停止/恢复和生产 stream 仍未验收。
- 本轮完整门禁：`WORKMESH_OPENRESTY_BIN=/tmp/workmesh-test-missing-openresty GOWORK=off go test ./...`、`go test -race ./node/api ./node/service`、`go vet ./...`、759 条路由契约和差异检查均通过。

## 2026-09-10 本轮推进

- 联合外部黑盒完成：容器文件/日志、容器生命周期、Compose 生命周期、网站高级设置、TCP/UDP stream 和独立 WAF 均通过；临时资源均已清理。
- 数据库外部黑盒首次因宿主根分区无空间导致 MongoDB 未就绪；清理 Go 编译/测试缓存释放空间后重跑通过，覆盖 MariaDB、PostgreSQL、Redis、MongoDB 的真实创建、用户/权限、改密、同步、删除及 Redis 依赖失败 503。
- 修复容器列表前端手动刷新路径：停止、退出、创建和 dead 状态不再显示资源加载图标，CPU、内存、IO 等资源字段稳定回填为 0；运行中容器仍显示真实 Docker stats。前端 `npm run type-check` 通过。
- 主机磁盘只读接口已改为真实 `lsblk -b -J` 与 `df -P -B1` 采集，响应符合前端 `CompleteDiskInfo`（设备、分区、挂载点、文件系统、容量、使用率、系统盘标记）；命令不可用时返回 `503 + ERR`。
- 主机防火墙设置接口已返回 system/forwarding/docker 后端分组及真实安装/激活状态；新增防火墙规则查询从 `iptables -S`/`ip6tables -S` 读取并转换为库存结构，不再返回固定模拟规则。
- 新增磁盘契约与防火墙状态定向测试，`GOWORK=off go test ./node/api`、`go vet ./...`、`git diff --check` 通过；防火墙写规则、磁盘分区/挂载、Gateway 和生产资源仍需隔离环境验收，SSH PTY 已在后续隔离测试完成。
- 新增并执行 `WORKMESH_RUNTIME_EXTERNAL_TEST=1 GOWORK=off go test ./node/api -run TestExternalRuntimeLifecycles -count=1`：使用本地 `golang:1.24`、`node:22-alpine`、`python:3.12-alpine`、`eclipse-temurin:21-jdk-alpine`、`.NET 8 alpine` 和 `1panel-php-fpm:8.5.10`，逐项完成真实容器注册、停止、启动、重启、状态核验和删除，全部通过且无 `workmesh-acceptance-runtime-*` 残留。修复直接容器运行时在 Compose 路径不存在时错误 `chdir` 的问题。PHP 扩展安装、多版本 FPM 和 Supervisor 仍需单独验收。
- 新增并执行 `WORKMESH_TERMINAL_EXTERNAL_TEST=1 GOWORK=off go test ./node/api -run TestExternalContainerTerminalPTY -count=1`：临时 `python:3.12-alpine` 容器完成真实 WebSocket/PTY 握手、输入输出回显，测试后容器清理。SSH PTY 已在临时 sshd 隔离测试中通过，代理断线重放和权限边界仍未完成。
- 防火墙规则页的检查、创建、删除、更新、重置、后端选择/初始化/清理和 ping 策略路由已接入受控 iptables 执行器；仅允许 `WORKMESH_BASIC*` 管理链、白名单协议/地址/端口，所有外部命令使用参数数组，失败返回真实错误。nftables/firewalld/ufw 写入、规则同步及真实内核规则变更仍需隔离主机验收，当前未执行生产写操作。

## 2026-09-10 诊断与 SSH 密钥补充

- `/api/v2/hosts/diagnostics/profiles` 已改为真实 Go `pprof` 采样下载，支持 cpu/heap/goroutine/mutex/block，限制单次 CPU 采样最长 60 秒并以 gzip Blob 返回；非法类型、依赖缺失和写出失败均返回明确错误。新增 `TestRuntimeProfileDownload`。
- SSH 密钥 CRUD 已通过临时 `WORKMESH_SSH_HOME` 生命周期测试；同名更新在密钥写入、口令加密或 SQLite 更新失败时恢复旧私钥、公钥和 `authorized_keys`，避免覆盖后残留半成品。
- 新增可选真实 Docker 网络/卷生命周期测试 `TestExternalDockerNetworkVolumeLifecycle`（`WORKMESH_CONTAINER_EXTERNAL_TEST=1`）；普通环境未启用 Docker 时保持跳过，尚未触碰生产资源。
- 主机 Docker 端口防护接口已接入真实 Docker inspect：`/hosts/firewall/docker/ports`、`endpoints` 读取所有容器发布端口（含 IPv4/IPv6、协议、容器和 Compose 标签）；策略批量保存、删除、同步使用 SQLite 状态，不再返回固定空列表。防护初始化/绑定/解绑仅在显式 `WORKMESH_ALLOW_FIREWALL_MUTATION=1` 时执行 iptables 变更，默认返回依赖/安全错误。隔离测试已验证随机端口发布发现和网络/卷生命周期。
- 主机端口转发已接入 iptables/ip6tables NAT `PREROUTING` 真实读取、分页过滤和 add/remove 操作；端口、地址、协议、网卡均做白名单校验，写操作同样要求 `WORKMESH_ALLOW_FIREWALL_MUTATION=1`，失败返回真实命令错误并保留现有规则。新增转发参数和安全门禁单测；真实内核转发写入仍未在生产执行。
- 防火墙原生详情已接入真实 `firewall-cmd`/`ufw app info` 查询，缺少命令返回 `503`，非法名称和类型拒绝；规则同步预览读取当前 iptables 库存并返回明确 existing 状态，跨后端同步在适配器未具备时返回 `503` 且不执行写入，任务查询返回真实空闲状态。
- 防火墙 provider 适配继续推进：firewalld rich-rule 与 UFW allow/deny/reject 参数已接入规则创建、删除和更新，nftables 已支持 `nft -j list ruleset` 原生 JSON 库存读取及 `inet workmesh` 托管表/链初始化和清理；所有写操作仍受显式 mutation 开关保护。nftables 原生表达式保持 opaque，避免错误推断协议和动作。
- 容器镜像高级黑盒已执行：`WORKMESH_CONTAINER_EXTERNAL_TEST=1 GOWORK=off go test ./node/api -run '^TestExternalDockerImageBuildSaveLoad$' -count=1` 完成临时 Alpine 镜像 build、save tar、删除、load 和 inspect；匿名及 bcrypt Basic Auth `registry:2` push 也已完成真实验收并清理。
- PHP-FPM 多版本黑盒已执行：`WORKMESH_RUNTIME_EXTERNAL_TEST=1 GOWORK=off go test ./node/api -run '^TestExternalPHPMultiVersionFPMConfig$' -count=1` 在 `1panel-php-fpm:7.4.33` 和 `1panel-php-fpm:8.5.10` 临时容器中完成 `php-fpm -t` 配置检查，容器均已清理。
- PHP 扩展真实黑盒已执行：`WORKMESH_RUNTIME_EXTERNAL_TEST=1 GOWORK=off go test ./node/api -run '^TestExternalPHPExtensionInstall$' -count=1` 在临时 `1panel-php-fpm:8.5.10` 容器中完成 Redis PECL 安装、镜像 commit、运行时重启和 `php -m` 验证（65.7s），测试后容器已清理。多版本 FPM 与 Supervisor 外部生命周期仍未完成。
- Supervisor 外部黑盒已执行：`WORKMESH_RUNTIME_EXTERNAL_TEST=1 GOWORK=off go test ./node/api -run '^TestExternalSupervisorLifecycle$' -count=1` 在临时 PHP-FPM 容器中挂载独立 `supervisor.d`，真实完成进程配置创建、`reread/update`、运行状态确认、restart 和 delete（12.1s），测试后容器已清理。

## 2026-09-10 主机 SSH 与磁盘操作

- 新增主机 SSH 管理接口：`/api/v2/hosts/ssh/search` 真实读取 `sshd_config` 和 `systemctl` 状态；`operate` 支持 start/stop/restart/enable/disable；`update` 和 `file/update` 使用原子文件替换、指令白名单和服务重启失败回滚；`file` 支持 `authKeys`、`sshdConf` 和受限 `sshdConfPath` 读取。配置路径可用 `WORKMESH_SSHD_CONFIG` 隔离，服务名可用 `WORKMESH_SSH_SERVICE` 指定。
- 新增 SSH 密钥 CRUD：`/api/v2/hosts/ssh/cert`、`update`、`sync`、`search`、`delete` 支持导入或 `ssh-keygen` 生成、公钥授权、加密口令存储、文件及 SQLite 原子回滚；`WORKMESH_SSH_HOME` 可将密钥目录隔离到临时验收目录。已通过 `TestSSHCertLifecycleUsesIsolatedFiles`，真实 sshd/远程登录验收仍为 `not-run`。
- 新增磁盘分区、挂载、卸载处理器，调用真实 `parted`、`partprobe`、`mkfs`、`mount`、`umount`、`findmnt`，并支持 `fstab` 自动挂载项的原子写入/删除。拒绝根目录和系统挂载点、路径穿越及不支持文件系统；分区写操作默认关闭，隔离验收需显式设置 `WORKMESH_ALLOW_DISK_MUTATION=1`。当前未对生产磁盘执行写操作。
- 新增 SSH 文件/回滚和磁盘保护定向测试，`GOWORK=off go test ./node/api -run 'TestSSH|TestDisk' -count=1`、`go vet ./node/api`、`git diff --check` 通过。

## 2026-09-10 防火墙门禁与日志清理闭环

- 防火墙规则创建、删除、更新、重置及 ping 策略统一要求显式 `WORKMESH_ALLOW_FIREWALL_MUTATION=1`；缺少开关时返回 `503 + code=ERR`，避免只读面板请求意外修改主机内核规则。
- 修复 nftables 后端操作分支被前置校验排除的问题；新增隔离 fake 命令测试确认 nftables 初始化入口可达，未执行真实内核写入。
- `/api/v2/core/logs/clean` 与 `/api/v2/logs/clear` 仅接受 `login`、`operation` 或空类型；SQLite 日志删除使用事务，状态文件写入和数据库删除失败均返回错误并恢复内存/持久化状态，不再忽略错误固定返回成功。
- 新增防火墙写入门禁、nftables 入口和日志清理回归测试；完整 `go test ./...`、`go test -race ./node/api ./node/service`、`go vet ./...`、759 条路由契约及 `git diff --check` 均通过。
- 隔离 Docker 网络/卷、容器生命周期和 Compose 生命周期外部验收重跑通过，临时资源已清理。真实 firewalld/UFW/nftables 内核规则写入、跨后端同步、ACME 外部签发、Gateway 双进程和生产发布仍保持 `not-run`。
- 修复网站监控与 WAF xpack 旧别名：监控日志搜索/详情/统计/清理以及 WAF 攻击、拦截、关联、日志查询现在均转发到真实访问日志、ModSecurity 审计或 WorkMesh WAF JSON/JSONL 文件处理器，不再落到通用统计或内存快照；新增站点日志别名回归验证。
- 在 `unshare -n` 独立网络命名空间中完成真实 iptables 与 nftables 规则生命周期验收（托管链/表初始化、规则创建、原生库存读取、按 UUID 删除、清理）；新增 `TestExternalFirewallIptablesLifecycle` 和 `TestExternalFirewallNftablesLifecycle`，实际运行通过，测试命名空间销毁后无主机规则残留。另完成 system 子系统 iptables↔nftables 双向同步、预览、`resetSource` 源端清理及失败补偿，新增 `TestExternalFirewallCrossBackendSync`，实际运行通过。firewalld/UFW、转发和 Docker 策略跨后端同步仍保持 `not-run`。

## 2026-09-10 firewalld/UFW 规则库存闭环

- 新增 firewalld rich-rule 和 UFW numbered-rule 的真实只读库存解析；`POST /api/v2/hosts/firewall/rules/search` 不再对这两个已安装后端固定返回“不支持”。规则保留 provider、family、zone/chain、协议、端口、动作、原始行和稳定 UUID；无法安全推断的 firewalld service/log/masquerade 规则标记为 `opaque`。
- 防火墙规则 UUID 删除和更新现在会查询 firewalld/UFW 实时库存并调用对应 CLI，仍受 `WORKMESH_ALLOW_FIREWALL_MUTATION=1` 门禁保护；不可用后端只读请求返回真实依赖错误，不伪造空成功。
- firewalld/UFW 不再对没有独立 WorkMesh 托管链的初始化/绑定/清理请求返回伪造成功；依赖可用但操作无安全托管语义时明确返回 `503 + ERR`，规则级操作保持可用。
- 新增 fake CLI 契约测试，覆盖 rich-rule/UFW 解析、family 过滤、UUID 和依赖缺失错误。`GOWORK=off go test ./...`、`go vet ./...`、`git diff --check` 通过。真实 firewalld/UFW 内核规则写入仍未执行，继续标记 `not-run`。
- SQLite 日志保留清理已接入 `StartBackgroundTasks` 的独立维护周期，按固定策略事务清理过期/超量记录，并在 `operation_logs` 写入 `/internal/log-retention` 成功或失败审计；周期可通过 `WORKMESH_LOG_RETENTION_INTERVAL` 调整，文件型日志仍由 journal/logrotate 或维护窗口管理。
- 网站旧版 `websites/log/search` 在真实 access/error 日志读取失败时不再回退到扩展内存快照，改为返回站点/文件真实错误；避免错误被固定空列表或伪造日志掩盖。
- Docker 端口防护 `initialize/bind/unbind` 已接入真实 `WORKMESH_DOCKER` 与 `DOCKER-USER` iptables 链操作，绑定失败会回滚本次新建链，重复绑定/解绑保持幂等；仍需在隔离 Docker daemon 中验证策略规则实际拦截和 IPv6/nftables 路径。
- Docker 防护同步已支持从 SQLite 策略生成 `WORKMESH_DOCKER` iptables 规则；`deny_all`/`deny_sources` 可预览、写入，`allow_sources` 生成来源允许规则及同端口兜底拒绝规则，并在 `resetSource` 下全部成功后删除源策略。IPv6/nftables、真实 Docker DNAT 拦截仍需隔离验收。
- 旧版 `/hosts/firewall/docker/sync` 已改为调用同一同步执行器，不再仅探测 Docker 后固定返回 `synced:true`；该入口现在受 mutation gate 保护并返回真实成功/失败计数。
- Docker 防护策略保存增加后端地址族/CIDR、端口、协议和来源校验，拒绝跨族地址及命令注入形态；同步时 `allow_sources` 生成允许来源规则和同端口兜底拒绝规则，规则组任一步失败会回滚已写入规则。
- Docker 端口列表的 `effective` 现在只有在对应托管链真实绑定到 `DOCKER-USER` 后才为真；仅保存 SQLite 策略或仅初始化链不会再显示为已生效。

## 2026-09-10 容器镜像仓库推送

- 镜像构建接口兼容旧版 1Panel 请求：当 `dockerfile` 缺省时，将 `name` 作为构建上下文、`image` 作为目标标签，并自动使用上下文下的 `Dockerfile`；当前前端的 `name`/`dockerfile` 契约保持不变。
- 镜像推送接口在本地标签与仓库目标标签不同时自动执行受控 `docker tag`，再调用 `docker push`；认证仍通过 `--password-stdin`，密码不进入参数或响应。
- 新增并执行隔离 `registry:2` 黑盒：临时 Alpine 镜像 build、仓库登记、目标 tag、真实 push 和目标镜像核验均通过；另以 bcrypt htpasswd 验证认证 registry 的 `docker login --password-stdin`、push、logout 和目标镜像核验。两组测试结束后 registry、镜像和标签均清理，密码未进入参数或响应。push 失败时自动删除生成的临时 tag，fake CLI 回归确认 push→rmi 补偿顺序。
- 新增并执行隔离 SSH PTY 黑盒：临时 `sshd` 使用随机端口、临时 host key/authorized_keys 和独立 SQLite；真实主机登记、元数据更新凭据保留、WebSocket PTY 握手及远端命令回显均通过，sshd、密钥、临时主机记录和 known_hosts 均已清理。`terminalCommand` 新增可选 `WORKMESH_SSH_KNOWN_HOSTS`，用于部署时指定受控 known_hosts 文件，仍通过参数数组传递。
- 本轮门禁复核：`GOWORK=off go test ./...`、`GOWORK=off go test -race ./...`、`GOWORK=off go vet ./...`、759 条路由契约和 `git diff --check` 均通过；数据库隔离黑盒 `TestExternalDatabaseLifecycles` 亦重跑通过。真实 ACME、双节点 Gateway、SSH 代理断线/权限边界、firewalld/UFW 内核写入和生产发布仍保持 `not-run`。
