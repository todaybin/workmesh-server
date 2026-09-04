<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 进程与运行时接口

- `GET /api/v2/process/{pid}`：读取本机进程命令行和基础信息。
- `POST /api/v2/process/listening`：调用 `ss` 或 `netstat` 列出监听端口。
- `POST /api/v2/process/stop`：按 PID 终止进程，生产环境必须配置 `WORKMESH_COMMAND_TOKEN` 并在 `X-WorkMesh-Token` 中传入。

接口不拼接 shell 命令；进程读取失败返回结构化 `ERR`。

## 运行环境

- `POST /api/v2/apps/search` 按 `php/go/java/node/python/dotnet` 严格隔离 1Panel 应用目录；PHP 只公开统一 `php` 应用。
- `POST /api/v2/runtimes` 从应用详情指定的 HTTPS 归档安装。归档地址、跳转、下载大小、解压路径和解压总量均受限，且只部署精确的 `<type>/<version>` 目录。
- `POST /api/v2/runtimes/search` 支持类型、名称、状态和分页筛选；创建时间由 SQLite 持久化，旧记录从更新时间回填。
- `POST /api/v2/runtimes/update` 保留名称和代码目录，更新容器参数后执行真实 Compose 重建。应用目录版本变化时同步包内 `run.sh`，旧脚本保留为备份，重建失败恢复旧脚本。
- `POST /api/v2/runtimes/sync` 通过 Docker inspect 同步真实容器状态；启停、重启、删除和日志操作不返回模拟成功。
- `POST /api/v2/runtimes/del` 删除前检查网站引用，并处理 `forceDelete` 与 `deleteImage`。

非 PHP 环境执行 `docker compose pull` 和 `up -d`。PHP 使用包内构建上下文生成 `1panel-php-fpm:<PHP_VERSION>`，依次执行 build、up、扩展安装、commit、down 和最终 up，不从 Docker Hub 拉取目标镜像。

## PHP 专属接口

- PHP 参数、原始 `php.ini`/`php-fpm.conf`、FPM 性能参数和容器参数更新均写入运行时目录，并在容器重启失败时恢复旧配置。
- `GET /api/v2/runtimes/php/fpm/status/{id}` 只连接该运行时映射到 `127.0.0.1` 的 FastCGI 端口，请求 `/status` 并返回完整键值列表。
- `POST /api/v2/files/read/php` 只按运行时 ID 读取 `build.log`；`POST /api/v2/files/read/php-fpm-slow-logs` 只读取同一运行时的 `log/fpm.slow.log`。客户端路径参数不会参与文件定位。
- PHP 扩展列表通过容器内 `php -m` 获取；扩展模板 CRUD、创建时多模板合并及扩展安装/卸载均持久化到 SQLite。安装执行 `install-ext + commit + down/up`；卸载在运行时目录内原子删除模块 `.so`、`conf.d` INI，并同步更新 `php.ini` 与 `.env`，重启失败时恢复旧文件。
- Supervisor 列表、创建、更新、启停、重启、删除、配置文件和日志接口只操作 `<runtime>/supervisor`，并调用对应容器内的 `supervisorctl`。名称、用户、容器目录、文件类型、内容大小和操作均有白名单校验。

Node scripts 和模块扫描限制在授权代码目录内；模块安装、更新和卸载通过对应运行时容器中的 npm、yarn 或 pnpm 执行。
