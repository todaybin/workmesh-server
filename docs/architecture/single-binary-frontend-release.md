<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 单二进制内置前端发布契约

- 文档状态：单二进制 embed 构建链已实现；目标机和线上功能验收待完成。
- 更新时间：2026-09-13
- 适用项目：`/www/apps/workmesh-server`
- 业务参考：只读 `/www/apps/1Panel`

## 结论先行

WorkMesh 已按 1Panel 的发布方式实现：前端生产构建产物进入
`internal/webassets/dist`，由 `internal/webassets/embed.go` 的
`embed.FS` 收集，随后由 `go build` 编进 `workmesh-server` 二进制。
生产运行时只需要二进制、配置和数据目录，不需要读取外置的
`web/dist`。

当前实现由 `cmd/workmesh-server/http_routes.go` 接收内置文件系统并提供
SPA、assets、favicon 和兼容静态资源入口。`deploy/install/deploy-frontend.sh`
仍存在，但只适用于历史外置目录兼容或回滚，不是单二进制标准发布步骤。
构建完成不等于目标机上线完成，systemd、域名、WAF 和浏览器功能仍需现场验收。

## 与 1Panel 的同步关系

| 环节 | 1Panel 现状 | WorkMesh 当前现状 | WorkMesh 目标 |
| --- | --- | --- | --- |
| 前端源码 | `/www/apps/1Panel/frontend` | `/www/apps/workmesh-server/web` | 保持当前路径 |
| Vite 输出 | `frontend/vite.config.ts` 的 `outDir: ../core/cmd/server/web` | `web/vite.config.ts` 的 `outDir: ../internal/webassets/dist` | 已对齐 |
| Go embed 包 | `core/cmd/server/web/web.go` | `internal/webassets/embed.go` | 已实现 |
| 嵌入内容 | `index.html`、`assets`、`favicon.png`、`static/*` | `dist` 下的入口、assets、favicon 和已输出的兼容资源 | 已实现 |
| 构建顺序 | `build_frontend -> build_core_on_linux` | `make build-frontend -> make build-server -> make build-release` | 已对齐 |
| 运行依赖 | `1panel-core` 单二进制提供页面 | 单二进制提供页面，配置和数据目录独立 | 已对齐 |

1Panel 的关键依据是：

```text
/www/apps/1Panel/Makefile
  build_frontend
    -> frontend npm run build:pro
    -> frontend/vite.config.ts outDir=../core/cmd/server/web
  build_core_on_linux
    -> go build core/cmd/server/main.go

/www/apps/1Panel/core/cmd/server/web/web.go
  //go:embed index.html
  //go:embed all:assets
  //go:embed favicon.png
  //go:embed static/*
```

同步的是可观察发布行为和构建顺序，不复制 1Panel 的业务实现。WorkMesh
仍然只有一个主项目，control 和 node 仍然是同一进程内的业务分区；
`/www/apps/1Panel` 不参与 WorkMesh 的编译、运行或部署。

## 当前产物布局

仓库当前形成以下关系：

```text
web/
  src/
  package.json
  vite.config.ts
  -> npm run build:pro

internal/webassets/
  embed.go
  dist/
    index.html
    assets/
    favicon.png

make build-release
  -> workmesh-server
  -> 前端资源已经进入二进制
```

`go:embed` 只能引用同一个 Go 包目录及其子目录，因此前端必须先输出到
`internal/webassets/dist`，再执行 Go 编译。运行时不从仓库工作目录读取
`web/dist`。

嵌入清单至少包括：

- SPA 入口 `index.html`；
- Vite 生成的全部 `assets/**`，包括入口 chunk、路由 chunk、CSS 和字体；
- 当前 API 或前端实际引用且由构建输出的 `static/**`；
- favicon 及其他当前由公开资源路由提供的前端文件。

构建后不得遗漏 WAF 七个页面对应的路由 chunk。发布门禁应检查入口文件、
`assets`、WAF 页面 chunk 和构建版本信息，而不能只检查二进制文件存在。

## 命令接口

### 当前可执行构建

```bash
cd /www/apps/workmesh-server
npm --prefix web ci
make clean-frontend
GOOS=linux GOARCH=amd64 make build-release
```

`make build-release` 的固定顺序是：

```text
web -> internal/webassets/dist
  -> go build ./cmd/workmesh-server
  -> .build/workmesh-server
```

正式候选制品前先执行 `make clean-frontend`，避免旧 hashed assets 留在
`internal/webassets/dist` 并被下一次 Go 编译一并嵌入。

默认二进制路径为 `.build/workmesh-server`，可通过 `BUILD_DIR`、
`SERVER_BINARY`、`GOOS` 和 `GOARCH` 覆盖。发布前验证：

```bash
test -x .build/workmesh-server
file .build/workmesh-server
sha256sum .build/workmesh-server
```

当前 Makefile 的 `build-release` 会把候选二进制同步到 `release/` 并生成
对应的 `.sha256` 文件，但没有生成完整 release manifest；发布系统如需要
manifest，应额外记录版本、目标平台、Git 提交、二进制摘要、Go 版本和
Node/npm 版本。不要使用 `latest` 代替版本号，也不要只
根据文件修改时间判断前端是否更新。

在真实目标机激活候选包使用：

```bash
WORKMESH_SERVER_ROOT=/opt/workmesh-server \
WORKMESH_SERVER_BINARY=/www/apps/workmesh-server/release/workmesh-server-linux-amd64 \
WORKMESH_SERVER_SHA_FILE=/www/apps/workmesh-server/release/workmesh-server-linux-amd64.sha256 \
bash deploy/install/activate-release.sh
```

该步骤才会替换实际运行二进制并重启 systemd；构建完成或复制到仓库
`release/` 不等同于线上已切换。

### Release 包接口

安装包应至少包含以下内容；`.build/workmesh-server` 作为候选制品复制为
安装包中的二进制：

```text
release/
  workmesh-server-linux-amd64
  workmesh-server-linux-amd64.sha256
  release-manifest.json       # 如发布系统生成
  config/server.json
  deploy/install/install.sh
  deploy/install/prepare-runtime.sh
  deploy/systemd/workmesh-server.service
```

单二进制发布包不应包含生产 `data`、数据库 secret、Gateway secret、
WAF 证书、WAF 运行日志或用户站点文件。上述数据由目标主机的运行目录
提供，并按发布批次单独备份。

### 安装和启动

在目标 Linux 主机上，先执行运行目录准备。该脚本默认 dry-run，不会
启动或删除容器：

```bash
cd /www/apps/workmesh-server
sudo WORKMESH_SERVER_ROOT=/opt/workmesh-server \
  deploy/install/prepare-runtime.sh

sudo WORKMESH_SERVER_ROOT=/opt/workmesh-server \
  deploy/install/prepare-runtime.sh --apply \
  --domain workmesh.cs.sopvip.com
```

然后安装单二进制。`WORKMESH_SERVER_BINARY` 必须是绝对路径且可执行；
`install.sh` 会渲染 systemd 单元并启用
`workmesh-server.service`：

```bash
sudo env \
  WORKMESH_SERVER_ROOT=/opt/workmesh-server \
  WORKMESH_SERVER_BINARY=/path/to/workmesh-server \
  deploy/install/install.sh --apply
```

安装前必须确认：

- `config/server.env` 已由 `prepare-runtime.sh --apply` 创建，并包含
  `WORKMESH_SERVER_ADDR`、`WORKMESH_DATA_DIR`；
- `config/server.json` 存在且没有把生产数据目录指向临时目录；
- 目标二进制 SHA-256 与 release manifest 一致；
- 旧二进制、配置、SQLite、WAL/SHM 已保存到独立备份目录；
- systemd 的 `WorkingDirectory`、`ExecStart` 和 `EnvironmentFile`
  全部指向同一个 `WORKMESH_SERVER_ROOT`。

启动后使用 `deploy/wh` 执行 CLI：

```bash
sudo WORKMESH_SERVER_ROOT=/opt/workmesh-server \
  deploy/wh help
sudo WORKMESH_SERVER_ROOT=/opt/workmesh-server \
  deploy/wh version
```

`deploy/wh` 只负责加载 `server.env` 并转发到
`bin/workmesh-server`；它不会构建前端、复制 `web/dist`、启动 WAF 或
管理数据库容器。

## 部署脚本接口说明

| 接口 | 默认行为 | `--apply` 或运行时行为 | 单二进制发布中的定位 |
| --- | --- | --- | --- |
| `deploy/install/prepare-runtime.sh` | 预览运行目录、数据库和 WAF 文件 | 创建 `server.env`、数据库 secret、Compose、WAF 目录和默认生成配置；不启动容器 | 必须执行 |
| `deploy/install/install.sh` | 检查 systemd、模板和环境文件 | 安装 `bin/workmesh-server`、配置、服务及更新 timer，并启动服务 | 必须执行 |
| `deploy/wh` | 调用安装目录中的 CLI help | 转发任意受支持的 CLI 子命令 | 日常运维入口 |
| `deploy/install/deploy-frontend.sh` | 检查并打印外置 dist 计划 | 备份并切换 `/opt/workmesh-server/web/dist` | 仅历史兼容或回滚，标准发布不用 |
| `deploy/acceptance/preflight.sh` | 只读检查主机、Docker、WAF、域名和工具 | 无生产变更模式 | 发布前必须执行 |
| `deploy/acceptance/domain-acceptance.sh` | 只读 HTTP/HTTPS、Host 和 WAF 请求检查 | 不 reload、不签发证书、不改 DNS | 域名上线验收 |
| `deploy/acceptance/readonly-evidence.sh` | 采集脱敏运行证据 | 不写业务运行配置 | 发布证据归档 |

`deploy/install/deploy-frontend.sh` 不能证明单二进制内置资源存在。它只为
历史外置目录部署保留备份、切换和回滚能力；标准生产发布不应执行它，以免
旧 `web/dist` 掩盖二进制中实际打包的前端版本。

## 生产验证

### 1. 制品和服务验证

```bash
sha256sum /opt/workmesh-server/bin/workmesh-server
systemctl is-enabled workmesh-server.service
systemctl is-active workmesh-server.service
systemctl show workmesh-server.service \
  -p ExecStart -p WorkingDirectory -p EnvironmentFiles
curl --fail --silent --show-error http://127.0.0.1:9999/health
curl --fail --silent --show-error http://127.0.0.1:9999/ready
```

`/health` 返回 200 只证明进程存活；`/ready` 返回 200 才表示统一存储
初始化和迁移已完成。还应检查：

```bash
journalctl -u workmesh-server.service -n 200 --no-pager
```

日志中不得有迁移失败、端口冲突、静态资源初始化失败或反复重启。

### 2. 无外置前端目录验证

这一步必须在隔离发布目录或已批准的维护窗口执行。验证目标是证明服务
在没有 `web/dist` 时仍能返回前端，而不是依赖外置目录补齐页面：

```bash
test ! -e /opt/workmesh-server/web/dist
curl --fail --silent --show-error \
  http://127.0.0.1:9999/ -o /tmp/workmesh-index.html
curl --fail --silent --show-error \
  http://127.0.0.1:9999/advanced/waf -o /tmp/workmesh-waf.html
grep -q '<script' /tmp/workmesh-index.html
cmp -s /tmp/workmesh-index.html /tmp/workmesh-waf.html
```

`cmp` 只用于确认 SPA history 路由返回同一入口，实际运行还必须从
入口 HTML 中取出一个真实的 hashed asset，再请求该资源：

```bash
asset="$(sed -nE 's/.*src="([^"]+)".*/\1/p' \
  /tmp/workmesh-index.html | head -n 1)"
test -n "$asset"
curl --fail --silent --show-error \
  "http://127.0.0.1:9999${asset}" -o /tmp/workmesh-entry.js
test -s /tmp/workmesh-entry.js
```

随后逐页访问 `docs/img/` 对应的七个 WAF 路由，完成浏览器截图、布局、
菜单切换、保存和刷新验证。页面能打开不等于 WAF 设置已经生效。

### 3. WAF 和域名验证

WAF 镜像、OpenResty 配置和前端二进制是三个独立发布对象。WAF 设置保存
后必须保留以下真实证据：

1. API 保存成功；
2. `waf/config.json`、`rules.json` 或生成配置的 SHA-256 发生预期变化；
3. 容器内 `nginx -t` 通过；
4. 控制面执行 reload 成功；
5. reload 后再次读取 `effective` 状态；
6. 正常请求、observe 请求和 block 请求符合预期；
7. 失败时旧配置仍可恢复，不能只留下数据库或 JSON 表面状态。

域名验收命令示例：

```bash
DOMAIN=workmesh.cs.sopvip.com \
ADDRESS=61.184.12.165 \
TLS_VERIFY=1 \
CHECK_CONTAINER=1 \
CHECK_LOGS=1 \
WAF_LOG_DIR=/opt/workmesh-server/openresty-waf/waf/logs \
sh deploy/openresty-waf/tests/domain-acceptance.sh
```

### 4. 回滚

单二进制回滚至少恢复同一备份批次的：

- `/opt/workmesh-server/bin/workmesh-server`；
- `config/server.json` 和 `config/server.env`；
- `data/workmesh.db` 以及存在时的 `workmesh.db-wal`、`workmesh.db-shm`；
- 与站点和 WAF 运行状态匹配的 OpenResty 配置及证书。

恢复后重新执行 `/health`、`/ready`、无外置前端验证、SQLite
`quick_check`/`integrity_check`、OpenResty `nginx -t` 和域名验收。回滚
期间保留失败版本、摘要和日志，不得用清理文件来掩盖失败。

## 当前验收状态

代码和构建链已完成：

- [x] Vite 生产产物输出到 `internal/webassets/dist`；
- [x] `internal/webassets/embed.go` 使用 `go:embed` 收集前端资源；
- [x] 静态路由使用注入的 embed 文件系统；
- [x] `make build-release` 先构建前端，再编译 Go 二进制；
- [x] 生产运行不依赖外置 `web/dist`。

以下事项仍必须在目标机或线上环境完成并留存证据：

- [ ] 安装候选二进制并完成 systemd 启动、`/health` 和 `/ready` 验收；
- [ ] 在没有外置 `web/dist` 的条件下访问 `/`、`/advanced/waf` 和真实
      hashed assets；
- [ ] 按 `docs/img/` 完成 WAF 七个页面的布局、tab 切换、保存和刷新验收；
- [ ] 验证 WAF 设置保存后确实生成配置、执行 `nginx -t`、reload，并对
      observe/block 请求产生预期效果；
- [ ] 完成 `workmesh.cs.sopvip.com` 的 DNS、HTTP/HTTPS、证书和域名验收；
- [ ] 完成 PHP、Go、OpenResty、MySQL/MariaDB、PostgreSQL、Redis 的目标机
      容器健康、备份恢复和真实业务请求验收。

因此当前发布状态是 `single-binary-built / target-acceptance-pending`：
单二进制构建已经具备，不能据此宣称公网域名、WAF 实际生效或全部页面功能
已经线上验收完成。
