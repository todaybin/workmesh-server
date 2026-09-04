<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-09-03 PHP 运行环境创建表单对齐

状态：`[x] 已完成`

## 已完成

- PHP 7.4.33 仍从应用商店的 `PHP 7` 条目创建；详情接口会从同一份 1Panel 应用目录中读取通用 `php` 主版本 `7` 的表单定义，补齐 `PHP_VERSION`、`PHP_EXTENSIONS`、`CONTAINER_PACKAGE_URL` 和 `PANEL_APP_PORT_HTTP`。
- 创建抽屉显示与 1Panel 一致的扩展源：国内环境提供中科大、网易、阿里云、清华、xTom 香港、xTom 和 Debian 默认源，国际环境提供 Debian 默认源和 xTom。
- 默认扩展模板与 1Panel `InitPHPExtensions` 保持一致：`Default`、`WordPress`、`Flarum`、`SeaCMS`、`Dev`，扩展串逐项一致。
- 按产品要求将扩展模板选择由单选改为多选；选择多个模板后合并扩展、去除空项和重复项，再写入 `PHP_EXTENSIONS`。
- PHP 扩展模板的查询、创建、更新和删除接口已使用真实 SQLite 持久化；`all: true` 返回全部模板，不受分页大小截断。
- PHP 创建请求会把 `source` 同步写入 `CONTAINER_PACKAGE_URL`，并将扩展名转为小写、去重后保存为逗号分隔字符串，同时维护运行时扩展列表。
- PHP 运行时详情的扩展响应包含 `supportExtensions`，不再只返回已安装扩展名称。

## 验证证据

- `GOWORK=off go test ./...` 通过。
- `GOWORK=off go vet ./...` 通过。
- `npm run type-check` 通过。
- `npm run build:pro` 通过；生产 PHP chunk 中已确认两个扩展选择器均为 `multiple`，扩展源列表完整。
- 源码和前端产物均未发现 `/api/v1/runtimes/search` 或 `/api/v1/apps/search`。
- 后端测试覆盖 1Panel PHP 表单字段补齐、五个默认模板、全量查询、自定义模板 CRUD、SQLite 重启恢复及创建参数规范化。
- Linux amd64 后端与最新前端已部署；线上二进制 SHA256 为 `d45a2f2ecc60bd4bf312b34f4104048359d0e029f2493f3567f3400e3de7b559`。
- 部署备份位于 `/opt/workmesh-server/backups/deploy-20260903T074806Z-php-parity`，包含旧二进制、前端、SQLite、配置和运行时目录。
- `workmesh-server.service` 重启后为 `active/running`、`NRestarts=0`；`/health` 和 `/ready` 均返回 HTTP 200，启动日志无迁移或初始化错误。
- 生产 PHP chunk 已确认包含完整扩展源、`CONTAINER_PACKAGE_URL` 以及两个 `multiple` 扩展选择器；生产产物未发现运行时或应用搜索的 `/api/v1` 请求。

## 人工复验边界

- 生产业务接口要求有效的本地登录会话；无会话访问模板和应用搜索接口会按预期返回 `LOCAL_AUTH_REQUIRED`。页面中的模板名称、模板多选交互和创建提交仍需在已登录浏览器中做最终目视确认。
