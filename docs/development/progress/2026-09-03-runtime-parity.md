<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-09-03 六类运行环境对齐

状态：`[>] 进行中`

## 已完成

- 六类应用搜索按类型隔离；PHP 只显示统一应用，版本与同步后的 1Panel 应用目录一致。
- 创建、编辑、状态同步、启停、重启、删除和日志已接入真实 Compose/容器命令；错误不再返回模拟成功。
- 官方归档具备来源、重定向、超时、大小和安全解压校验，精确部署版本目录并保留旧目录。
- Go、Java、Node.js、Python、.NET 使用包内 Compose，写入版本、代码目录、脚本、容器名、端口、环境变量、卷及主机映射。
- PHP 使用 `1panel-php-fpm:<PHP_VERSION>` 目标镜像完成 build/up/install-ext/commit/down/up 链路，支持配置回滚、扩展模板多选和 SQLite CRUD；扩展目录已同步 1Panel 当前 63 项清单，单扩展安装使用 `install-ext + commit + down/up`，卸载按 `.so`、`conf.d` INI、`php.ini` 和 `.env` 文件事务处理并在重启失败时回滚。
- PHP-FPM 状态使用 FastCGI `/status`；慢日志只按运行时 ID 读取；Supervisor CRUD、生命周期、配置与日志使用运行时目录和容器内 `supervisorctl`。
- Node scripts、模块扫描和容器内 npm/yarn/pnpm 操作均设置目录、文件大小、模块数量和参数限制。
- 应用目录 `lastModified` 变化时更新包内 `run.sh`，成功时保留旧脚本，Compose 重建失败时回滚。
- 运行时创建时间、参数、状态、错误和任务信息使用 SQLite 恢复；重启中断任务标记为系统中断。
- TaskLog 合并同任务并发读取，组件销毁时停止定时器并忽略过期响应。

## 验证证据

- `GOWORK=off go test ./...` 通过；因测试需绑定本机临时端口，经人工批准后在沙箱外执行。
- `GOWORK=off go vet ./...` 通过。
- `npm run type-check` 与 `npm run build:pro` 通过。
- route、implementation、hidden-function 三项契约扫描通过，公开清单 871 条全部满足。
- 源码和 `web/dist` 均未发现 `/api/v1/runtimes` 或 `/api/v1/apps` 请求。
- 定向测试覆盖命令顺序、安全归档、版本目录部署、脚本备份/回滚、SQLite 恢复、类型隔离、状态同步、PHP 配置、FastCGI 解析、慢日志路径限制及 Supervisor CRUD/回滚。
- PHP 扩展定向测试覆盖 63 项目录、ionCube/SourceGuardian 文件映射、直接安装命令顺序、卸载文件事务、路径越界拒绝和重启失败回滚。
- Linux amd64 最新制品为 `.tmp/workmesh-server-linux-amd64-runtime-logs`，SHA256=`16eaeff1f862ba393c05b12f1c714c313c9f683841074e12df57a89c7264264c`；`web/dist` 已随本次部署更新。
- 最新部署脚本 `.tmp/deploy-runtime-logs-20260903T120000Z.sh`；生产二进制 SHA256=`16eaeff1f862ba393c05b12f1c714c313c9f683841074e12df57a89c7264264c`，备份目录为 `/opt/workmesh-server/backups/deploy-20260903T200000Z-runtime-logs`。

## 生产验收状态

- [x] 生产二进制、前端、SQLite、配置和运行时目录已完成最新备份及 SHA256 记录；备份目录为 `/opt/workmesh-server/backups/deploy-20260903T200000Z-runtime-logs`。
- [x] 最新构建已原子部署并重启 `workmesh-server.service`；服务为 `active/running`、`NRestarts=0`，`/health` 和 `/ready` 均返回 HTTP 200。
- [!] 真实安装 PHP `8.5.10/7.4.33/5.6.40`、Go `1.26`、Java `25`、Node.js `25.9.0`、Python `3.14.0`、.NET `10.0`。
- [!] 在真实容器上验收扩展卸载、FastCGI、慢日志、Supervisor、Node 模块管理、终端、日志和完整生命周期。

上述项目会修改生产 Docker/systemd 或生产数据，按 `AGENTS.md` 必须先取得人工确认。未执行前不得标记为已完成。

## 后续结构工作

- [>] PHP 扩展文件事务已迁入 `node/service`，命令执行可注入；六类运行时其余编排仍位于 `node/api`，后续应按稳定边界继续迁入 `node/service`，让 HTTP handler 仅保留协议适配。
