<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 部署与旧版本切换

安装脚本默认只执行预检。只有完成安装包校验、配置备份、Gateway 注册信息确认和健康检查后，才允许使用 `--apply` 写入 systemd。

当前安装流程管理 `workmesh-server.service`，并安装
`workmesh-server-update.service` 和 `workmesh-server-update.timer`。自动更新
默认关闭，只有在运行目录的 `config/update.env` 中显式设置
`WORKMESH_AUTO_UPDATE_ENABLED=1`，并配置受信任的 Ed25519 公钥和制品来源后
才会执行更新。旧版本清理属于单独的人工历史清理任务，不作为当前安装前置条件。

## 单二进制前端发布边界

生产发布方式与 1Panel 一致：`web` 的生产构建结果写入
`internal/webassets/dist`，由 `internal/webassets/embed.go` 声明的
`embed.FS` 收集，再由 `go build` 编进 `workmesh-server`。安装后只部署
`bin/workmesh-server`、配置和数据目录，不部署外置 `web/dist`。完整的
发布包和目标机验收要求见
[`docs/architecture/single-binary-frontend-release.md`](../../docs/architecture/single-binary-frontend-release.md)。

当前仓库的单二进制实现为：

- `web/vite.config.ts` 输出到 `internal/webassets/dist`；
- `internal/webassets/embed.go` 使用 `//go:embed all:dist`；
- `cmd/workmesh-server/http_routes.go` 从注入的 embed 文件系统提供 SPA、
  assets、favicon 和兼容静态资源入口；
- `Makefile` 先构建前端，再编译 Go 服务；
- `deploy/install/install.sh` 安装已编译的二进制、配置和 WorkMesh 的 systemd
  服务及更新 timer。

因此发布应明确区分：

| 发布类型 | 前端来源 | 是否已完成 |
| --- | --- | --- |
| 单二进制发布 | `workmesh-server` 内置 `index.html`、assets、公开资源和 SPA 入口 | 已实现，仍需目标机验收 |
| 外置目录兼容发布 | 二进制 + 目标机 `web/dist` | 仅用于历史兼容或回滚，不是标准生产流程 |

标准流程只替换 `bin/workmesh-server`，不要求同步 `web/dist`。

生产激活建议使用仓库提供的发布入口。它会校验发布包 SHA-256、备份当前
二进制、原子替换目标文件、重启 `workmesh-server.service`，并检查
`/health`、`/ready` 和 `/advanced/waf`；服务重启或 HTTP 验收失败时会尝试自动回滚：

```bash
cd /www/apps/workmesh-server
make build-release
WORKMESH_SERVER_ROOT=/opt/workmesh-server \
  make deploy-release
```

也可以直接指定发布包：

```bash
WORKMESH_SERVER_ROOT=/opt/workmesh-server \
WORKMESH_SERVER_BINARY=/path/to/release/workmesh-server-linux-amd64 \
bash deploy/install/activate-release.sh
```

当前受限开发环境只允许写 `/www`，不能代替目标机写入 `/opt` 或控制
systemd。因此在此环境完成构建不等于生产激活完成；必须在实际运行
`workmesh-server.service` 的宿主机执行上述命令并核对目标文件 SHA-256。
`deploy/install/deploy-frontend.sh` 只保留给历史兼容或回滚，不能用于证明或
更新单二进制前端。目标机验收必须在没有外置 `web/dist` 的条件下访问 `/`、
`/advanced/waf` 和入口 HTML 引用的 hashed assets。

## 构建、发布和安装命令

### 标准单二进制构建

```bash
cd /www/apps/workmesh-server
npm --prefix web ci
make clean-frontend
GOOS=linux GOARCH=amd64 make build-release
```

`make build-release` 固定执行以下顺序：

```text
web build:pro
  -> internal/webassets/dist
  -> go build ./cmd/workmesh-server
  -> .build/workmesh-server
  -> SHA-256
```

`make clean-frontend` 应在正式候选构建前执行，避免 Vite 输出目录中遗留的
旧 hashed assets 被一并嵌入。可通过 `BUILD_DIR`、`SERVER_BINARY`、`GOOS`
和 `GOARCH` 覆盖默认输出。

构建产物的运行时只需要二进制、配置和数据目录，不需要 `web/dist`。
目标二进制必须在隔离目录中通过 `/`、`/advanced/waf` 和真实 hashed asset
请求。不能用浏览器缓存、旧静态目录或反向代理缓存代替该验证。

### 运行目录和安装

```bash
cd /www/apps/workmesh-server
sudo WORKMESH_SERVER_ROOT=/opt/workmesh-server \
  deploy/install/prepare-runtime.sh

sudo WORKMESH_SERVER_ROOT=/opt/workmesh-server \
  deploy/install/prepare-runtime.sh --apply \
  --domain workmesh.cs.sopvip.com

sudo env \
  WORKMESH_SERVER_ROOT=/opt/workmesh-server \
  WORKMESH_SERVER_BINARY=/path/to/release/workmesh-server-linux-amd64 \
  deploy/install/install.sh --apply
```

`prepare-runtime.sh` 只准备 server.env、数据库/WAF 运行目录和 secret，不
构建前端、不安装 Go 二进制、不启动容器。`install.sh` 安装二进制、配置和
systemd，不复制 `web/dist`；已有服务会通过 `activate-release.sh` 原子切换并
执行健康检查，新安装实例才直接安装并启动。在单二进制目标中，生产目录不应出现依赖前端
服务的 `web/dist`。

### 自动更新

自动更新由 root 专用的 `deploy/install/auto-update.sh` 执行，systemd timer
默认每 6 小时检查一次，首次启动延迟 15 分钟并带随机延迟。完整流程为：

```text
获取制品和签名
-> SHA-256 与 Ed25519 验签
-> 使用已安装 CLI 暂存到 data/releases
-> 比较当前运行二进制摘要
-> 备份 server.json、server.env、SQLite、WAL/SHM
-> 停止 workmesh-server.service
-> 原子替换 bin/workmesh-server
-> 启动并检查 systemd、MainPID、9999、health、ready、WAF 页面
-> 失败恢复旧二进制并恢复服务状态
```

安装后先复制示例并保持关闭，确认发布源和公钥后再开启：

```bash
sudo install -m 0600 deploy/install/update.env.example \
  /opt/workmesh-server/config/update.env
sudo vi /opt/workmesh-server/config/update.env
sudo systemctl restart workmesh-server-update.timer
```

远程制品必须使用 HTTPS：

```dotenv
WORKMESH_AUTO_UPDATE_ENABLED=1
WORKMESH_UPDATE_ARTIFACT_URL=https://updates.example.com/workmesh-server-linux-amd64
WORKMESH_UPDATE_SIGNATURE_URL=https://updates.example.com/workmesh-server-linux-amd64.sig
WORKMESH_UPDATE_PUBLIC_KEY=/etc/workmesh/workmesh-update.pub
WORKMESH_UPDATE_SHA256=...
WORKMESH_UPDATE_VERSION=v2.20260913
```

更适合长期无人值守的是签名 manifest。manifest 本身也必须使用同一受信任
公钥签名，字段至少为 `version`、`os`、`arch`、`size`、`sha256`、
`artifact_url`、`signature_url`；脚本会先验 manifest，再验二进制。成功版本
写入 `data/auto-update-state.env`，低于已安装版本的签名 manifest 会被拒绝。

也可以配置 root 拥有的本地制品路径。公钥不能由远程 URL 提供，不能把私钥
放入 `update.env`。建议将公钥固定为 root 可读文件并记录 SHA-256。每次更新
的日志和备份分别位于：

```text
/opt/workmesh-server/logs/auto-update.log
/opt/workmesh-server/backups/auto-update-<UTC>/
```

检查 timer、手动执行一次和查看结果：

```bash
systemctl status workmesh-server-update.timer --no-pager
sudo systemctl start workmesh-server-update.service
journalctl -u workmesh-server-update.service -n 100 --no-pager
```

自动更新明确拒绝服务未 active、缺少签名或公钥、非 HTTPS 远程地址、并发
更新、制品摘要不匹配、systemd `ExecStart` 不指向固定
`bin/workmesh-server`。失败时保留候选制品和备份，便于人工调查。

### 生产卸载和重新部署

如果生产仍然加载旧版本，先执行只读卸载预览：

```bash
cd /www/apps/workmesh-server
bash deploy/install/uninstall-production.sh
```

执行卸载需要在真实生产主机完成，并要求 Docker daemon、systemd 和
`/opt/workmesh-server` 可写。默认卸载会停止并移除 WorkMesh systemd
服务、WAF/数据库容器和 WorkMesh 网络，同时把运行目录移动为带时间戳的
备份目录；不会删除 `/www/wwwroot`、`/opt/workmesh` 或旧
`WorkMesh-postgresql-ZNMP`、`WorkMesh-redis-ZNMP` 资源：

```bash
sudo env \
  WORKMESH_CONFIRM_PRODUCTION_UNINSTALL=REMOVE_WORKMESH_PRODUCTION \
  WORKMESH_SERVER_ROOT=/opt/workmesh-server \
  bash deploy/install/uninstall-production.sh --apply
```

只有确认不再需要 WorkMesh SQLite、配置、WAF 数据和命名数据库卷时，才使用
清理模式：

```bash
sudo env \
  WORKMESH_CONFIRM_PRODUCTION_UNINSTALL=PURGE_WORKMESH_PRODUCTION \
  WORKMESH_SERVER_ROOT=/opt/workmesh-server \
  bash deploy/install/uninstall-production.sh --apply --purge-data
```

清理模式仍不会删除网站根目录和旧 ZNMP 资源。卸载完成后再执行
`prepare-runtime.sh --apply`、数据库/WAF 镜像准备和 `activate-release.sh`，
避免把旧外置 `web/dist` 当成新版单二进制前端。

安装后：

```bash
systemctl is-active workmesh-server.service
curl --fail --silent --show-error http://127.0.0.1:9999/health
curl --fail --silent --show-error http://127.0.0.1:9999/ready
sudo WORKMESH_SERVER_ROOT=/opt/workmesh-server deploy/wh version
```

安装前必须保存候选二进制 SHA-256、旧二进制、`server.json`、
`server.env`、SQLite 主库和存在时的 WAL/SHM。`install.sh` 当前不是完整的
发布编排器，不会自动完成数据库备份、WAF 镜像发布、域名证书、浏览器验收
或回滚。

### 部署脚本接口

| 脚本 | 作用 | 是否修改生产 |
| --- | --- | --- |
| `prepare-runtime.sh` | 准备运行目录、secret、Compose、WAF 默认配置 | 仅 `--apply` 写入目录；不启动/删除容器 |
| `install.sh` | 安装二进制、配置和 systemd | 仅 `--apply` 写入并启动服务及更新 timer |
| `deploy-frontend.sh` | 备份和切换外置 `web/dist` | 仅用于历史兼容或回滚；单二进制发布不用 |
| `deploy/wh` | 加载环境并转发 CLI | 会执行被转发的 CLI 操作 |
| `../acceptance/preflight.sh` | 只读主机、Docker、WAF、域名预检 | 不修改生产 |
| `../acceptance/domain-acceptance.sh` | 只读域名、HTTP/HTTPS 和 WAF 请求 | 不 reload、不签发证书 |

## 上线最低依赖矩阵

WorkMesh 的上线依赖分为宿主机进程和 Docker 运行时。PHP、Go 网站运行时不是
宿主机 `php`、`php-fpm` 或 Go 编译器进程；它们由运行时包和 Docker 容器提供。

| 能力 | 当前部署方式 | 上线前必须具备 | 当前缺口 |
| --- | --- | --- | --- |
| WorkMesh Server（Go） | 预编译 Go 二进制 + `workmesh-server.service` | 可执行二进制、systemd、`server.json`、`server.env`、持久化 `data` | 真实 systemd 切换和 `/health`、`/ready` 仍需目标机验收 |
| Go 网站运行时 | 运行时应用包生成 Compose，默认使用包内镜像/配置 | Docker Engine、Compose、已审核的 `go/<version>` 运行时包、唯一监听端口 | 指定生产实例、镜像来源、端口和域名访问仍未验收 |
| PHP 网站运行时 | `1panel-php-fpm:<PHP_VERSION>` 构建/启动，扩展后提交镜像 | Docker Engine、Compose、对应 PHP-FPM 运行时包、FastCGI 端口、扩展构建依赖 | 目标机多版本 PHP-FPM、扩展、Supervisor 和真实 PHP 请求仍需验收 |
| OpenResty/WAF | `deploy/openresty-waf` 镜像 + Compose | 已构建且 digest 固定的 WAF 镜像、80/443、站点目录、证书和 WAF 目录 | 当前环境没有 Docker daemon，真实 `nginx -t`、reload 和拦截仍未完成 |
| PostgreSQL | 数据库 Compose 的 `postgres:18-alpine` | secret、持久化卷、容器健康、备份/恢复证据 | 生产容器尚未在当前环境启动；模板使用 tag，发布时仍应固定 digest |
| Redis | 数据库 Compose 的 `redis:8-alpine`，启用密码和 AOF | secret、持久化卷、容器健康、RDB 备份/恢复证据 | 生产容器尚未在当前环境启动；模板使用 tag，发布时仍应固定 digest |
| MySQL 兼容数据库 | 当前由 MariaDB 11 提供，服务名为 `mariadb` | MariaDB secret、持久化卷、健康和备份/恢复证据 | 仓库没有 Oracle `mysql:*` 服务；若上线要求必须是 Oracle MySQL，需要单独部署并登记外部数据库 |

因此，当前仓库可以部署 MySQL 协议兼容的 MariaDB，但不能宣称已经提供了
Oracle MySQL 镜像。不要把 `mariadb` 服务重命名为 `mysql`，数据库 Runtime API
和固定容器名契约使用 `workmesh-panel-mariadb`。

`WORKMESH_SERVER_ROOT` 支持自定义绝对路径。`install.sh --apply` 会把
`deploy/systemd/workmesh-server.service` 中的路径模板渲染为同一目录，并安装默认
`config/server.json`；正式安装前必须先用 `prepare-runtime.sh --apply` 生成包含
`WORKMESH_SERVER_ADDR` 和 `WORKMESH_DATA_DIR` 的 `config/server.env`。

## 首次准备运行目录

在真实部署主机上，先从仓库根目录执行运行目录准备。默认只做 dry-run，不会写文件、
启动或删除任何容器：

```bash
deploy/install/prepare-runtime.sh
```

本机当前网络配置的 IPv4 为 `61.184.12.165`，建议隔离验收域名使用
`workmesh.cs.sopvip.com`。先查看只读域名规划：

```bash
deploy/install/domain-plan.sh
```

规划输出的 DNS 记录应为：

```text
workmesh IN A 61.184.12.165
```

DNS 服务商完成 A 记录后，再用 `dig +short A workmesh.cs.sopvip.com` 验证解析；
脚本不会替你写 DNS。

脚本自身的安全检查可以在没有生产权限时执行：

```bash
deploy/install/prepare-runtime.sh --self-test
```

确认路径和操作无误后执行：

```bash
deploy/install/prepare-runtime.sh --apply \
  --domain workmesh.cs.sopvip.com
```

脚本可以通过 `WORKMESH_PUBLIC_DOMAIN`、`WORKMESH_ACCEPTANCE_DOMAIN` 或 `--domain`
设置 `WORKMESH_PUBLIC_URL`；未传入域名时不会擅自写入公网域名。脚本会创建
`/opt/workmesh-server/data/database`、四个随机数据库 secret、
`/opt/workmesh-server/data/database/docker-compose.yml`、OpenResty/WAF 配置目录、
默认 WAF JSON 和生成配置，并在缺少时创建 `config/server.env`。传入 `--domain`
会将 `WORKMESH_PUBLIC_URL` 设置为对应的 HTTPS 地址，同时保留其他
现有变量和凭据。已有 secret、WAF 设置和 Compose 文件默认保留；使用
`--force` 只替换运行目录中的 Compose 文件，不会覆盖 secret、WAF JSON 或生成配置：

```bash
deploy/install/prepare-runtime.sh --apply --force
```

脚本不会执行 `docker compose up`，也不会删除旧的
`WorkMesh-postgresql-ZNMP`、`WorkMesh-redis-ZNMP` 或任何网站目录。准备完成后必须
按顺序执行数据库预检、数据库黑盒验收、WAF 镜像构建/摘要固定、OpenResty `nginx -t`
和域名验收。没有真实 Docker 权限、公网 IPv4、DNS A 记录和证书时，不能把准备完成
当作上线完成。

推荐顺序：

1. 旧版本只读盘点和备份。
2. 执行 `prepare-runtime.sh --apply` 准备数据库和 WAF 运行目录。
3. 在维护窗口启动数据库和已固定 digest 的 WAF 镜像。
4. 新版本在 `${WORKMESH_SERVER_ROOT:-/opt/workmesh-server}` 预检安装并启动服务。
5. 验证 `/health`、`/ready`、Gateway 注册、主次节点、域名 HTTPS、WAF 实际拦截和全部功能。
6. 确认回滚窗口关闭后，如现场确有历史旧版本残留，再人工执行旧版本清理；默认保留旧数据目录。
