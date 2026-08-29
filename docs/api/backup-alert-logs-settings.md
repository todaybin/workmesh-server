<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 备份、告警、日志与系统设置

这些控制面接口由单进程服务提供，状态保存于 `WORKMESH_DATA_DIR/domains.json`，通过临时文件原子替换，重启后自动恢复。所有成功响应使用 `{"code":200,"data":...}`，参数错误返回 `ERR` 和 `details.errCode`。

## 备份

`GET /api/v2/backups/local`、`POST /api/v2/backups/search` 和记录搜索接口返回 `{items,total}`。`POST /api/v2/backups/backup` 创建记录；传入 `source/path` 时复制本地文件到数据目录的 `backups` 子目录。`update`、`del`、`record/description/update`、`record/size`、`record/download` 和 `recover` 提供记录维护与恢复。上传接口限制 128 MiB，文件名按 `filepath.Base` 清理。

## 告警

`POST /api/v2/alert/update` 支持按 `id` 更新或创建告警，`alert/search`、`config/search`、`cronjob/list` 查询列表；`config/update` 和 `config/info` 管理通道配置，`config/test` 返回可观测测试结果。告警日志支持搜索和清理。

## 日志

日志搜索、详情、统计、清理以及任务日志接口均限制请求体 4 MiB。系统日志读取接口仅允许读取调用方指定的普通文件，并将内容限制为 2 MiB；不存在或非普通文件返回 `NOT_FOUND`。系统文件、服务、状态和执行任务计数接口返回稳定空列表，待接入平台采集器后可透明替换。

## 设置

`/api/v2/config/global` 和 `/api/v2/core/settings/*` 的查询与更新统一合并 JSON 键值。未知键会保留，空键会忽略；敏感凭据不得写入设置或日志。
