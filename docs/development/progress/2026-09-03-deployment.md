<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-09-03 构建与部署验收

状态：`[>] 进行中`

## 已完成

- Go 门禁：`GOWORK=off go test ./...` 通过；`GOWORK=off go vet ./...` 通过。
- 前端门禁：Node.js 20.20.2 下 `npm run type-check` 与 `npm run build:pro` 通过。
- 契约门禁：route scan 871 条、implementation scan 871 条、hidden-function scan 通过。
- Linux amd64 制品已构建：`.tmp/workmesh-server-linux-amd64`。
- 正式备份目录：`/opt/workmesh-server/backups/release-20260903-034502`，包含旧二进制、SQLite、OpenResty 配置、站点目录和前端归档。
- 已原子替换 `/opt/workmesh-server/bin/workmesh-server` 与 `/opt/workmesh-server/web/dist`，部署 SHA256：`7512ec9f699a40cbfff893b1e573d8c1b584fca94f4caabf0e6ee7faeca03239`。
- `workmesh-server.service` 重启后保持 `active (running)`；`/health`、`/ready` 均返回 HTTP 200。
- 真实依赖 smoke test：OpenResty `openresty -t` 成功；Showdoc、PostgreSQL、Redis 容器真实运行；Compose 状态可查询。
- SQLite 启动恢复核验：`schema_migrations=8`，`app_installs=3`，`app_install_tasks=4`，`login_logs=6`，旧 `runtime.json` 已归档。
- 本轮后端持久化变更构建 Linux amd64 制品：`.tmp/workmesh-server-linux-amd64-next`，SHA256=`8759928e456079fa80b388c45350b201a52b2b8765e48033aba5a98487cccee5`。
- 持久化制品已于 `2026-09-03 10:50 CST` 完成备份后原子替换并重启：当前二进制 SHA256=`8759928e456079fa80b388c45350b201a52b2b8765e48033aba5a98487cccee5`，备份目录为 `/opt/workmesh-server/backups/release-20260903-025041-persistence`；`workmesh-server.service`、`/health`、`/ready` 均正常。
- 网站主域名/域名列表契约及 SQLite 安全入口修复已部署：SHA256=`dbef1e186a3f50a121b49e7547efb97c0a38b43ba58af99104c3734a3d874b56`，备份目录 `/opt/workmesh-server/backups/release-20260903-031905-website-domain-security`。生产验证登录 200、1/1 网站域名非空、域名响应为数组且包含主域名；根路径按安全入口策略返回 404。
- PHP 运行环境扩展源与扩展模板对齐版本已于 `2026-09-03 15:48 CST` 部署：SHA256=`d45a2f2ecc60bd4bf312b34f4104048359d0e029f2493f3567f3400e3de7b559`，备份目录 `/opt/workmesh-server/backups/deploy-20260903T074806Z-php-parity`；服务为 `active/running`、`NRestarts=0`，`/health` 和 `/ready` 均返回 HTTP 200。
- PHP `codeDir` 校验修复及累计整改已于 `2026-09-03 19:36 CST` 重新打包部署：生产二进制 SHA256=`f74fc0a26b9440f4128022f4e0fc0d57417d7170467a6da71d754bd5dd62341a`，备份目录 `/opt/workmesh-server/backups/deploy-20260903T193600Z-runtime-php-codir-v2`；部署脚本 SHA256=`08a48b96f919dcedbe446599788f3c5460b3ee3e5d909550f91e9918c3861147`；服务为 `active/running`、`NRestarts=0`，`/health` 和 `/ready` 均返回 HTTP 200。
- 运行时构建/下载日志及 Compose 路径修复已于 `2026-09-03 20:22 CST` 打包部署：生产二进制 SHA256=`16eaeff1f862ba393c05b12f1c714c313c9f683841074e12df57a89c7264264c`，备份目录 `/opt/workmesh-server/backups/deploy-20260903T200000Z-runtime-logs`；服务为 `active/running`、`NRestarts=0`，`/health` 和 `/ready` 均返回 HTTP 200。

## 未完成与阻塞

- 2026-09-03 20:22 CST 已完成最新运行时日志与 Compose 路径修复制品部署：生产二进制 SHA256=`16eaeff1f862ba393c05b12f1c714c313c9f683841074e12df57a89c7264264c`，备份目录为 `/opt/workmesh-server/backups/deploy-20260903T200000Z-runtime-logs`，服务 `active/running`、`NRestarts=0`，`/health` 和 `/ready` 均返回 HTTP 200。
- 本次部署脚本为 `.tmp/deploy-runtime-logs-20260903T120000Z.sh`；SQLite 备份使用生产机 Python 标准库 `sqlite3.backup`（生产机无 `sqlite3` CLI）。
- 站点类型切换、字符串运行时 ID 和 Compose 默认变量修复已于 `2026-09-03 23:39 CST` 部署后端制品：SHA256=`5793f747f9e15666887e2d204b2352340abb88bafe3666bd18143c3abf66c431`，备份目录为 `/opt/workmesh-server/backups/deploy-20260903T233802+0800-site-runtime`；服务重启后 `active/running`，`/health` 与 `/ready` 均返回 HTTP 200。
- 对应前端产物已于 `2026-09-03 23:42 CST` 部署，备份目录为 `/opt/workmesh-server/backups/deploy-20260903T234230+0800-frontend`；生产构建使用 `VITE_REPORT=false` 避免可视化报告造成 OOM，`npm run type-check` 通过，`build:pro` 成功。

- Gateway 注册/授权凭据未配置，无法完成真实 Gateway 心跳与跨节点任务透传。
- 主节点到次节点 `162.14.96.198:9999` 的网络连通性仍需安全组放行后复验。
- 宿主无 `/dev/kvm`，CubeSandbox MicroVM E2E 只能保持 degraded/restricted。
- ACME 远程签发、部分 AI 连接测试和未接入外部资源的路由仍按真实错误返回，不能标记为已完成。
- `legacy_routes.go` 仍保留未迁移契约的明确 501 入口；实现扫描通过不等同于所有业务已完成。

## 复验命令

```bash
GOWORK=off go test ./...
GOWORK=off go vet ./...
npx --yes --package node@20 --call 'npm run type-check && npm run build:pro'
node test/contract/route-scan.mjs check --legacy ../workmesh-node --project . --manifest test/contract/routes.json
```
