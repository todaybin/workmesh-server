<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 黑盒验收执行准备（2026-09-06）

状态：[>] 工具与资源准备审计完成；真实业务黑盒尚未执行。本文件只记录可立即执行的分组顺序、环境入口和阻断条件，不保存 Token、Cookie、密码、私钥、请求体或响应秘密。

本轮是只读审计：没有运行全量 Go/race/vet，没有编译前端，没有访问或修改 `/www/apps/1Panel`，没有连接会改变状态的 Docker、SSH、ACME 或 Gateway，也没有执行创建、更新、删除、重启、安装等业务请求。

## 一、当前合约范围

真实前端合约矩阵为 `docs/inventory/frontend-http-ws-contract-matrix.json`：

| 范围 | 数量 | 当前状态 |
| --- | ---: | --- |
| HTTP 案例 | 385 | `not-run`，需真实会话和逐项请求证据 |
| WS 案例 | 4 | `not-run`，需业务消息序列，不仅是 Upgrade |
| 合计 | 389 | 静态矩阵不是功能通过证明 |

WS 入口为：

- `/api/v2/core/script/run`
- `/api/v2/hosts/terminal/container`
- `/api/v2/hosts/terminal/local`
- `/api/v2/hosts/terminal/ssh`

前端菜单清单共有 13 个主菜单。矩阵的 UI 关联数量不是去重后的接口数量，执行时必须以案例 ID 去重，并按菜单/operation family 记录：

| 主菜单 | 页面路由 | UI 关联 endpoint 数 |
| --- | --- | ---: |
| Home | `/` | 137 |
| Advanced | `/advanced` | 23 |
| AI | `/ai` | 137 |
| App | `/apps` | 45 |
| Container | `/containers` | 87 |
| Cronjob | `/cronjobs` | 194 |
| Database | `/databases` | 43 |
| System | `/hosts` | 111 |
| Log | `/logs` | 93 |
| Setting | `/settings` | 53 |
| Terminal | `/terminal` | 23 |
| Toolbox | `/toolbox` | 41 |
| Website | `/websites` | 130 |

按案例的 operation family 统计：`endpoint 232`、`update 21`、`get 17`、`search 17`、`del 15`、`check 14`、`info 10`、`status 16`、`sync 5`、`download 5`、`config 3`、`upload 3`、`run 2`、`create 2`、`list 9`、`setting 2`、`tree 2`、`operate 6`、`delete 2`、`install 1`、`stop 1`。同一路径多方法/多操作必须按矩阵案例和方法分别验收，不能把一次请求当成整个功能族通过。

## 二、现有工具与已知限制

### 1. `http-smoke.mjs`

位置：`test/contract/http-smoke.mjs`。

用途：执行无需管理员凭据的 8 条安全边界：health、ready、v2 health、认证设置、未登录 current、未登录 dashboard、错误密码登录、根入口隐藏。可选 `--security-entrance` 会增加安全入口页面和登录页检查。

证据文件独立写入 `--out` 指定位置，不回写路由矩阵；响应只保留脱敏摘要。但错误密码登录会产生真实登录审计记录，正式执行前要确认允许这一只读边界探测的日志副作用。

### 2. `frontend-contract-executor.mjs`

位置：`test/contract/frontend-contract-executor.mjs`。

支持四级范围：

| 范围 | 实际选择 | 前置条件 |
| --- | --- | --- |
| `boundary` | 7 条边界（6 HTTP + 1 WS 未授权握手） | 无凭据可运行 |
| `read` | 非动态 GET/HEAD 与 WS 握手 | `WORKMESH_TOKEN` 或 `WORKMESH_COOKIE` |
| `fixtures` | 仅提供真实 fixture 的案例 | Token/Cookie、外部 fixture |
| `all` | fixture 案例，包括写操作 | 以上条件及 `WORKMESH_CONFIRM_DESTRUCTIVE=YES` |

重要限制：当前 `authHeaders()` 用于 HTTP 请求时将 Token/Cookie 替换为 `<redacted>`；脱敏值只适合证据，不适合发给服务端。因此当前工具不能作为登录后 HTTP 业务验收执行器，需工具维护者先修复“记录脱敏”和“实际发送凭据”分离，并由唯一集成测试负责人复验。WS 代码发送真实 Token/Cookie，但当前只验证 HTTP Upgrade，成功后立即发送关闭帧，不发送终端、脚本或容器业务消息，不能替代 4 条 WS 消息序列测试。

此外，`WORKMESH_SCOPE=read` 会执行符合条件的 WS 握手；`fixtures`/`all` 按矩阵逐项遍历，缺少 fixture 的案例为 `blocked` 或 `not-run`。执行器不会替动态路径填充 `1`、`test` 或空值，也不会生成业务 JSON。

## 三、凭据与环境变量入口

只记录变量名和用途，不读取、不打印、不写入真实值：

| 变量 | 用途 | 安全要求 |
| --- | --- | --- |
| `WORKMESH_BASE_URL` | API/WS 基地址；默认 `http://127.0.0.1:9999` | 测试环境必须明确写出，避免误打其他环境 |
| `WORKMESH_TOKEN` | Bearer/API 访问令牌 | 只从受保护进程环境注入，证据中仅记录存在性 |
| `WORKMESH_COOKIE` | 完整 Cookie 字符串 | 实际会话 Cookie 名称为 `workmesh_session`；若有安全入口还需保留 `SecurityEntrance` |
| `WORKMESH_SCOPE` | `boundary`、`read`、`fixtures`、`all` | 默认 `boundary`；不得无确认使用 `all` |
| `WORKMESH_REQUEST_FIXTURES` | 仓库外真实 fixture JSON 路径 | 不能进入 Git，不得包含密码、Token、Cookie、私钥或模拟资源 ID |
| `WORKMESH_CONFIRM_DESTRUCTIVE` | 写操作总确认 | 仅在隔离资源、回滚方案和人工批准齐备时设为 `YES` |
| `WORKMESH_TIMEOUT_MS` | 合约执行超时 | 按外部资源延迟设置，必须保留超时结果 |
| `WORKMESH_ADMIN_USERNAME` | 服务首次初始化/CLI 管理员用户名 | 只用于受控初始化，不写入报告，不当作测试凭据输出 |
| `WORKMESH_ADMIN_PASSWORD` | 服务首次初始化/CLI 管理员密码 | 禁止记录；生产不得使用默认值 |
| `WORKMESH_SERVER_CONFIG` | 配置文件路径 | 只注入路径，不读取或复制秘密 |
| `WORKMESH_SERVER_ADDR` / `WORKMESH_SERVER_PORT` | 监听覆盖 | 当前生产监听端口为 9999，变更需另行批准 |
| `WORKMESH_DATA_DIR` | SQLite 与运行数据目录 | 业务验收必须指向隔离数据目录或有可恢复备份 |
| `WORKMESH_GATEWAY_URL` / `WORKMESH_GATEWAY_ID` | Gateway 地址和节点标识 | 当前无有效次节点凭据时保持 `blocked` |
| `WORKMESH_GATEWAY_USERNAME` / `WORKMESH_GATEWAY_PASSWORD` / `WORKMESH_LINK_SECRET` | Gateway/节点链路认证 | 不读取、不记录；须由主控提供短期凭据和回滚窗口 |

认证实现还支持 `Authorization: Bearer`、`X-WorkMesh-Token`、`X-API-Key`。Cookie 会话写请求需要 `pcsrftoken` Cookie 与 `X-CSRF-Token` 双提交，并通过同源检查；Bearer/API Key 请求不走 Cookie CSRF 分支。登录成功响应会设置 HttpOnly `workmesh_session`，并返回兼容前端的 `token` 字段。登录接口接受 `name` 或 `username` 与 `password`，登录成功/失败均会记录真实 `login_logs`。

## 四、本机资源只读审计结果

| 资源 | 只读观察 | 对测试的意义 |
| --- | --- | --- |
| WorkMesh 服务 | systemd 单元工作目录 `/opt/workmesh-server`，EnvironmentFile 路径 `/opt/workmesh-server/config/server.env` | 可由主控在宿主机注入会话环境；本轮未读取 env 文件内容 |
| API 监听 | `*:9999` 存在监听 | 本机边界请求入口可用；不等于登录后业务可用 |
| OpenResty 入口 | 80/443 存在监听 | 可做隔离域名 HTTP/HTTPS 访问；正式域名验收仍需资源负责人授权 |
| SQLite | `/opt/workmesh-server/data/workmesh.db` 为 `root:root`、`0640` | 测试进程须有受控读权限；业务验收必须使用真实 SQLite，禁止 JSON fixture 代替数据库 |
| 网站根目录 | `/www/wwwroot` 为 `rstack:rstack`、可遍历 | 网站测试需使用 API 创建的隔离前缀，禁止手工覆盖既有 OpenResty/WAF 目录 |
| OpenResty 配置目录 | `/opt/workmesh-server/openresty-waf` 存在 | 只允许通过正式 API/部署流程变更，黑盒负责人不直接编辑配置 |
| Docker | `/usr/bin/docker` 存在，`/var/run/docker.sock` 为 `root:docker` | 本轮未连接 Docker；六类运行时需主控提供隔离容器和清理授权 |
| ACME | `certbot`、`openssl` 存在 | 工具存在不等于 DNS、80 端口和 Let's Encrypt HTTP-01 资源已就绪 |
| 测试工具 | Node、Go、curl 均存在 | 可执行脚本前提满足；认证执行器缺陷和真实凭据仍是阻断 |

## 五、立即执行的分组顺序

所有分组均使用独立时间戳证据文件；每组开始前记录测试环境标识、当前服务版本哈希、SQLite 迁移版本和资源清单，结束后记录 HTTP 方法、路径、状态码、脱敏响应摘要、耗时和副作用。不得把一组中的通过推断到另一组。

### 阶段 0：冻结与安全预检

1. 主控冻结所有源码、配置和前端改动；唯一测试负责人记录快照哈希。
2. 确认 API 基地址、隔离数据目录、回滚备份、测试域名前缀和清理责任人。
3. 确认 `WORKMESH_TOKEN` 或完整 Cookie 由外部安全注入；报告只记录 `token=true/false`、`cookie=true/false`。
4. 修复并验证 HTTP 执行器的凭据发送/证据脱敏分离；没有这一步，登录后 HTTP 组不得执行。
5. WS 业务消息使用专用测试工具，记录握手、认证、消息类型/序列、服务端响应和关闭释放；不得把握手 101 当成功闭环。

### 阶段 1：公开边界与认证

1. 运行 `http-smoke.mjs` 的公开 8 案例，确认 health/ready、v2 认证设置、未授权状态和错误密码边界。
2. 使用真实管理员凭据只登录一次，保存会话到受保护进程环境，不落盘到仓库；验证 `/api/v2/core/auth/current`、登出和会话失效。
3. 获取 CSRF Cookie 时只记录存在性；Cookie 写请求逐组验证正确 CSRF、错误 CSRF、跨源和过期会话。
4. 验证 Bearer、API Key、Cookie 三种授权方式的等价权限和错误 envelope；不记录凭据值。

### 阶段 2：无副作用读取组

按菜单依次执行无动态路径 GET/HEAD：概览、应用商店、AI、数据库查询、容器查询、系统/主机查询、终端能力查询、工具箱、日志审计、面板设置、网站列表/详情。动态 ID 必须来自本次真实创建或已有隔离资源，不能填假值。

验收重点：分页、排序、字段命名、`code/data` envelope、空结果与真实 SQLite 数据、权限边界、操作日志只读查询、访问日志/系统日志/任务日志/主机日志/登录日志/网站日志分类不得混淆。

### 阶段 3：可回滚 CRUD 资源组

按“创建 → 详情 → 列表/状态 → 更新 → 停止/启动/重启 → 日志/任务 → 删除 → 重启后确认”的顺序执行，每类资源独立前缀和独立清理记录：

1. 应用/应用商店安装记录。
2. Go、Node、Python、Java、.NET、PHP 运行环境；PHP 另测 FPM、扩展、配置和 FastCGI 站点访问。
3. 数据库及用户/密码设置（敏感字段只验证 `passwordSet` 等脱敏字段）。
4. 容器、镜像、仓库和容器操作。
5. 计划任务、异步任务状态、任务日志和失败重试。
6. 文件、备份、工具箱和下载上传功能。

每个写操作需使用真实 fixture，fixture 由资源负责人准备并放在仓库外；没有 fixture 就是 `blocked/not-run`，不可自动生成 JSON。

### 阶段 4：网站与证书组

为一键部署、运行环境、静态、反向代理、子网站、TCP/UDP 分配不同的 `*.cs.sopvip.com` 前缀和可回滚目录。依次验证域名、目录、默认文档、流量、反代/负载均衡、密码访问、CORS、HTTPS、真实 IP、伪静态、防盗链、重定向、PHP、资源和其他设置，并通过域名实际访问确认配置生效。

证书组顺序：HTTP-01 challenge 可达性 → 隔离测试域名正式签发 → 安装 → HTTPS 访问 → 续期 dry-run/续期策略 → 失败回滚。需要 DNS、80/443、Let's Encrypt 账户/限额和域名解析，缺一项就保持 `blocked`，不能用 staging 或固定证书宣称正式通过。

### 阶段 5：主站点/次站点与 WS

仅在主控提供次节点地址、短期凭据、签名密钥和隔离节点后执行：注册/登录、节点授权刷新/撤销、心跳、role epoch/fencing、Gateway 绑定、任务透传、断线重连和错误回滚。浏览器不得直连次节点，所有目标选择必须经过服务端 relay。WS 终端按 local/container/ssh 分别验证认证、命令/输入、输出、退出码、关闭、超时和资源释放。

### 阶段 6：更新与迁移组

在独立副本上执行二进制下载校验、备份、停止/启动/重启、SQLite 迁移、旧字段保留、回滚和健康/就绪检查。迁移前后记录 `schema_migrations`、`migration_runs`、表结构摘要和关键业务行数；不在生产真实库上试错。只有候选包哈希、迁移结果和回滚演练均有证据才可进入上线验收。

## 六、当前阻断分类

### 可由现有本机资源解除

- 公开边界 HTTP/WS：API 9999、80/443 和工具链存在，可在确认副作用后执行。
- 认证后读取：只需主控安全注入有效管理员 Token/Cookie，并先修复 HTTP 执行器凭据误脱敏问题。
- 本机 SQLite 只读核对：已有受控 root 权限和 URI `mode=ro` 方案。

### 不能仅靠本机资源解除

- 385 条登录后 HTTP 写操作：需要每个功能的真实隔离资源、fixture、权限矩阵和清理/回滚方案。
- 4 条 WS 业务消息：需要专用 PTY/脚本/容器消息测试工具，现有执行器只测握手。
- 六类运行时与 PHP FPM：需要真实 Docker/运行时包、端口、进程和可清理卷。
- 六类网站与域名访问：需要可用的 `*.cs.sopvip.com` 前缀、独立目录、OpenResty/WAF 加载和回滚授权。
- 主次节点：需要次节点在线、凭据、签名/epoch/fencing 测试窗口。
- Let's Encrypt：需要 DNS、HTTP-01 80 入口、ACME 账户/限额和正式域名授权。

## 七、结论与交接

当前可以立即执行公开边界和只读资源预检，但不能把现有工具直接用于登录后完整验收。最先需要主控安排的两个动作是：

1. 冻结源码并修复/验证 HTTP 执行器的真实凭据发送与脱敏记录分离。
2. 准备隔离管理员会话、真实资源 fixture、WS 消息工具、测试域名前缀、次节点和 ACME 资源。

满足这两个前置条件后，按阶段 0 → 1 → 2 → 3 → 4 → 5 → 6 执行；每阶段由唯一集成测试负责人出具独立证据，缺少真实输入或外部资源的项目继续标记 `blocked/not-run`。本审计没有访问或记录任何真实秘密，也没有改变服务、数据库、Docker、SSH、Gateway、ACME 或参考项目状态。
