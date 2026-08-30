<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 备份、告警、日志与系统设置

这些控制面接口由单进程服务提供，状态保存于 `WORKMESH_DATA_DIR/domains.json`，通过临时文件原子替换，重启后自动恢复。所有成功响应使用 `{"code":200,"data":...}`，参数错误返回 `ERR` 和 `details.errCode`。

## 备份

账号和记录使用独立集合保存于 `domains.json`：`POST /api/v2/backups`（或公共账号别名 `/api/v2/core/backups`）创建账号，`POST /backups/search` 分页查询账号，`POST /backups/update`、`POST /backups/del` 更新或删除私有账号；`/core/backups/update`、`/core/backups/del` 按名称兼容公共账号操作。账号响应默认脱敏 `accessKey`、`credential` 和 `refresh_token`。

`GET /api/v2/backups/local` 返回本地备份目录，`GET /api/v2/backups/options` 返回账号选项，`GET /api/v2/backups/check/{name}` 检查账号是否存在及是否被记录引用。`POST /api/v2/backups/backup` 创建真实记录；传入 `source/path/filePath` 时复制文件或目录到数据目录的 `backups` 子目录。`record/search`、`record/search/bycronjob`、`record/size`、`record/download`、`record/description/update` 和 `record/del` 提供记录分页、大小、下载路径、描述与批量删除。`search/files` 列出账号目录文件。

`recover` 和 `recover/byupload` 校验记录或上传文件并支持指定目标路径复制；`upload` 同时支持 JSON 本地文件导入和 multipart 上传，大小限制 128 MiB，文件名按 `filepath.Base` 清理。`conn/check` 执行本地目录检查并校验远端凭据字段，`buckets` 返回本地目录桶，`refresh/token` 持久化刷新状态。云厂商实际 API 调用仍需配置对应适配器，接口会明确返回当前本地状态。

## 告警

`POST /api/v2/alert/update` 支持按 `id` 更新或创建告警，`alert/search`、`config/search`、`cronjob/list` 查询列表；`config/update` 和 `config/info` 管理通道配置，`config/test` 返回可观测测试结果。告警日志支持搜索和清理。

## 日志

日志搜索、详情、统计、清理以及任务日志接口均限制请求体 4 MiB。系统日志读取接口仅允许读取调用方指定的普通文件，并将内容限制为 2 MiB；不存在或非普通文件返回 `NOT_FOUND`。系统文件、服务、状态和执行任务计数接口返回稳定空列表，待接入平台采集器后可透明替换。

## 设置

`/api/v2/config/global` 和 `/api/v2/core/settings/*` 的查询与更新统一合并 JSON 键值。未知键会保留，空键会忽略；敏感凭据不得写入设置或日志。
