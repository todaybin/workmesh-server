<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 后端只读黑盒与运行证据

本记录只描述本轮在当前执行环境实际观察到的结果。检查过程没有修改业务代码、前端代码、生产配置、OpenResty 配置或 SQLite 数据，也没有重新编译前端、重启服务或创建业务测试数据。

## 结果分类

- `pass`：本轮命令实际得到符合预期的结果。
- `blocked`：检查目标存在，但当前命名空间或权限无法访问；不能据此判断生产服务失败。
- `not-run`：需要有效管理员凭据、次节点凭据、外部 Docker/ACME 资源或明确的破坏性测试授权，本轮没有用模拟数据替代。
- `observed`：读取到了真实状态或风险信号，但证据不足以判定功能验收通过。
- 历史记录只作为上下文，不覆盖本轮的 `blocked`、`not-run` 或失败结果。

## 环境与制品核对

| 项目 | 本轮结果 | 证据 |
| --- | --- | --- |
| 生产二进制路径 | `pass` | `/opt/workmesh-server/bin/workmesh-server` 存在，大小 `18555042` bytes，权限 `750` |
| 当前二进制 SHA-256 | `pass` | `7f796181450e149e0f7465871af0d60f1f09604f255d4e64f2d03e5f868b6d60` |
| systemd unit 文件 | `pass` | `/etc/systemd/system/workmesh-server.service` 存在；`ExecStart=/opt/workmesh-server/bin/workmesh-server`，工作目录 `/opt/workmesh-server`，`Restart=on-failure` |
| systemd 运行状态 | `blocked` | `systemctl show/status` 返回 `Failed to connect to bus: Operation not permitted`；当前沙箱无 systemd D-Bus 访问权 |
| 制品备份 | `pass` | `/opt/workmesh-server/backups/deploy-20260906T013200+0800-backend-sqlite-strict` 存在，包含 `workmesh-server.old`、配置和 SQLite 文件；旧制品 SHA-256 为 `89597a1e2a67818c35a92bfb2e567705b6e55d854ec36bca29476aa4bb384e01` |
| 当前制品与上一份部署记录 | `observed` | 当前二进制 SHA-256 为 `7f796...`，与既有部署记录中的 `89597...` 不同；需要在可访问宿主机的环境补做 systemd 主进程与版本对应关系核验 |

`server.json` 的只读内容显示监听地址为 `127.0.0.1:9999`，数据目录为 `./data`，请求超时 `30s`，关闭超时 `10s`，后台任务开启；并发限制为任务 `4`、转换 `2`、SSE `64`、AI 作业 `4`。`server.env` 仅记录变量名，值未写入本报告。

## HTTP 与 WebSocket 黑盒

### 直接服务地址

执行命令：

```bash
curl -sS -i --max-time 5 http://127.0.0.1:9999/health
curl -sS -i --max-time 5 http://127.0.0.1:9999/ready
```

本轮 `/health`、`/ready`、`/api/v2/health`、`/api/v2/core/auth/setting`、未登录鉴权接口和错误密码登录共 8 个 HTTP 案例均得到 `curl: (7) Failed to connect to 127.0.0.1 port 9999`，因此全部为 `blocked`，不是 HTTP 失败响应。当前网络命名空间同时观察到 `ss` 无法打开 netlink socket（`Operation not permitted`），不能从该现象推断宿主机服务未运行。

执行器证据写入临时目录，未覆盖项目清单：

```bash
WORKMESH_BASE_URL=http://127.0.0.1:9999 \
  node test/contract/http-smoke.mjs \
  --out /tmp/workmesh-http-smoke-20260906.json
# HTTP smoke: fail ({"blocked":8})

WORKMESH_BASE_URL=http://127.0.0.1:9999 WORKMESH_SCOPE=boundary \
  node test/contract/frontend-contract-executor.mjs \
  --out /tmp/workmesh-contract-boundary-20260906.json
# counts: {"blocked":7}; 进程退出码为 0，因为执行器把无法连接记录为 blocked 而不是 fail
```

其中 7 条边界案例为：健康、就绪、v2 健康、认证设置、未登录当前用户、未登录概览和未授权本地终端 WebSocket Upgrade。WS 案例得到 `connect EPERM 127.0.0.1:9999`，没有建立连接，也没有发送终端业务消息。

### 外部网站入口

对 `http(s)://znmp.sopvip.com/` 和 `http(s)://znmp.sopvip.com/health` 的请求均因当前环境 DNS 解析失败（`Could not resolve host`）而 `blocked`。不能将此前部署记录中的 `Host: znmp.sopvip.com` HTTP 200 直接当作本轮结果；历史记录仍保存在 `docs/development/progress/2026-09-05-postdeploy-regression.md`。

## OpenResty WAF

### 可读取的静态证据

- `/opt/workmesh-server/openresty-waf/docker-compose.yml` 存在，容器名为 `workmesh-openresty-waf`，映射 80/443，挂载 `/www/wwwroot`、`conf.d`、`stream.d`、`default`、SSL、WAF 数据和日志目录。
- 对 `/opt/workmesh-server/openresty-waf` 与 `/www/wwwroot` 的配置扫描未发现精确形式 `proxy_pass http://;`，结果为 `invalid_proxy_pass=not-found`。
- `/www/wwwroot/znmp.sopvip.com/nginx/site.conf` 当前为 `0` bytes。该站点由 Gateway/WAF 组合提供入口，不能仅依据这个空文件声称站点访问已通过。
- WAF 日志最近记录 ModSecurity-nginx `v1.0.3`，本地规则 `809` 条。
- WAF 错误日志持续出现 `4096 worker_connections exceed open file resource limit: 1024` 警告。这不是本轮改动，需在宿主机确认容器实际 `ulimit -n` 及是否影响并发后单独处理。

### 本轮不能执行的命令

```bash
docker exec workmesh-openresty-waf nginx -t
# permission denied while trying to connect to the Docker API socket
```

因此 WAF `nginx -t` 本轮状态为 `blocked`，不是 `pass`。历史部署记录曾记载该命令成功，但需要宿主机重新执行确认当前制品。

## TLS 与 Let's Encrypt

对现有证书执行了只读解析：

```text
subject=CN = znmp.sopvip.com
issuer=C = US, O = Let's Encrypt, CN = YE1
notBefore=Aug 30 12:44:36 2026 GMT
notAfter=Nov 28 12:44:35 2026 GMT
```

证书公钥与私钥公钥摘要均为：

```text
1639be62a277a5842b5817471789360df50158b7716e1ff73e52b2e502708b8a
```

所以当前文件级证书/私钥配对为 `pass`。这不等价于本轮已完成公网 HTTP-01 签发或续期：DNS、外部 ACME 验证和 `certbot`/ACME 账户操作本轮为 `not-run`。WAF 访问日志中可见历史 Let's Encrypt HTTP-01 请求成功（HTTP 200）和更早的 404，必须在宿主机使用真实 `*.cs.sopvip.com` 测试域名再次验证。

## SQLite 只读完整性

使用 Python 标准库以 `file:/opt/workmesh-server/data/workmesh.db?mode=ro` 只读打开数据库；没有执行 INSERT、UPDATE、DELETE、VACUUM 或迁移。结果：

| 检查 | 结果 |
| --- | --- |
| 文件 | `/opt/workmesh-server/data/workmesh.db`，`32460800` bytes |
| journal mode | `wal` |
| `PRAGMA integrity_check` | `ok` |
| `PRAGMA foreign_key_check` | `0` 行 |
| 表数量 | `63` |
| `schema_migrations` | `12` 条，最后为 `0012-runtime-task-state` |
| `operation_logs` | `2046` 条 |
| `login_logs` | `166` 条 |
| `runtime_task_logs` | `18145` 条 |
| `runtime_records` | `6` 条 |
| `runtime_tasks` | `31` 条 |
| `app_installs` | `3` 条 |
| `app_install_tasks` | `66` 条 |
| `websites` | `1` 条 |
| `website_domains` | `1` 条 |
| `website_settings` | `14` 条 |
| `website_waf_sites` | `1` 条 |
| `website_ssls` | `1` 条 |
| `gateway_binding` | `1` 条 |
| `cronjobs` | `0` 条 |
| `migration_runs` | `0` 条 |

SQLite 已有真实运行数据，不能以 JSON 文件或固定返回值替代。`legacy_imports` 有 4 条历史导入记录，说明旧 JSON 只用于迁移审计；本轮没有把这些文件当作业务数据源。

## HTTP/WS 契约与静态覆盖

### 路由契约

```bash
node test/contract/route-scan.mjs check \
  --legacy /www/apps/1Panel \
  --project . \
  --manifest docs/inventory/route-inventory-1panel.json
```

结果：`路由契约通过: 759 条`，另有 `191 条扩展路由（兼容允许）`。这是源码/清单契约校验，不是 759 条接口已登录实测。

### 前端接口范围

当前清单包含：

- HTTP `385` 条；
- WS `4` 条；
- 合计 `389` 条；
- 所有案例状态仍为 `not-run`，没有被本轮黑盒执行器回写为通过。

### 本轮静态测试

通过：

```text
frontend-contract-matrix.test.mjs       2/2
frontend-menu-inventory.test.mjs        1/1
frontend-contract-executor.test.mjs     2/2
route-scan.mjs check                     759/759
git diff --check                         pass
```

未通过：

```text
node --test test/contract/*.test.mjs     4 pass, 1 fail
失败项：test/contract/implementation-scan.test.mjs
```

直接生成实现扫描报告的命令可正常退出并报告 `751` 条 `implemented`，但这不能抵消其配套测试失败；两者计数不同，需由后端主线修复/复核扫描器测试后才可把该门禁标记为通过。

## 尚未完成的真实验收

以下项目本轮保持 `not-run` 或 `blocked`，没有构造 JSON fixture、固定成功响应或空数组：

1. 管理员登录后的全部 HTTP 接口参数、响应 envelope、分页、错误码和副作用。
2. 4 条 WS 的本地终端、容器终端、SSH 终端和脚本执行消息序列。
3. Go、Node、Python、Java、.NET、PHP 六类运行环境的创建、查询、停止、启动、重启、日志、任务、删除和持久化重建；PHP FPM、扩展、配置和站点访问仍需真实资源。
4. 一键部署、运行环境、静态、反向代理、子站点、TCP/UDP 六类网站及域名绑定。
5. 网站域名、目录、默认文档、限流、代理、负载均衡、密码、CORS、HTTPS、真实 IP、伪静态、防盗链、重定向、PHP、资源和 WAF 设置的域名访问闭环。
6. 主站点/次站点 Gateway 注册、鉴权、心跳、任务透传和次节点 `162.14.96.198:9999` 连通性。
7. 真实 `*.cs.sopvip.com` 子域名的 Let's Encrypt HTTP-01 签发、安装和续期。

## 宿主机继续执行清单

这些命令必须在能访问生产 systemd、Docker、监听端口、DNS 和真实凭据的宿主机执行；执行前不需要重新编译前端：

```bash
systemctl show workmesh-server.service -p ActiveState -p SubState -p MainPID -p NRestarts -p ExecMainStatus
curl -fsS -i http://127.0.0.1:9999/health
curl -fsS -i http://127.0.0.1:9999/ready
docker exec workmesh-openresty-waf nginx -t
curl -fsS -H 'Host: <真实测试子域名>.cs.sopvip.com' http://127.0.0.1/
```

然后在不改动现有 `route-inventory`/`frontend-http-ws-contract-matrix` 状态的前提下，提供真实管理员 Cookie 或 Token，按 operation family 分批执行登录后的 HTTP/WS、运行时、网站、日志、任务、更新迁移和 ACME 验收，并把每批请求方法、路径、状态码、响应摘要、SQLite 副作用、站点访问结果和失败原因写入独立时间戳证据文件。
