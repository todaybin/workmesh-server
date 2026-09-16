<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh 功能迁移逐路由清单

基线来源：只读参考 `/www/apps/1Panel/core` 与 `agent` 全源码，生成时间：2026-09-07T09:43:40.739Z。
共 759 条接口：implemented 759。

状态定义：`implemented`=已实现并有具体处理器，`partial`=具体处理器仍返回固定空数据或存在 TODO，`compatibility`=兼容占位，`pending`=迁移中，`missing`=未发现注册。

## ai

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/ai/accounts/providers` | node/api/ai_explicit_routes.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/ai/gpu/load` | node/api/ai_explicit_routes.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/ai/gpu/options` | node/api/ai_explicit_routes.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/ai/mcp/domain/get` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/accounts` | node/api/ai_explicit_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/ai/accounts/counts` | node/api/ai_explicit_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/accounts/delete` | node/api/ai_explicit_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/accounts/models` | node/api/ai_explicit_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/accounts/models/create` | node/api/ai_explicit_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/accounts/models/delete` | node/api/ai_explicit_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/accounts/models/discover` | node/api/ai_explicit_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/accounts/models/update` | node/api/ai_explicit_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/accounts/search` | node/api/ai_explicit_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/accounts/update` | node/api/ai_explicit_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/accounts/verify` | node/api/ai_explicit_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/agent/bind` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/agent/channels` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/agent/create` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/agent/delete` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/agent/list` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/agent/md/list` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/agent/md/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/agent/unbind` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/batch/install` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/batch/operate` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/batch/skill/install` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/batch/upgrade` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/delete` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/dingtalk/get` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/dingtalk/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/discord/get` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/discord/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/feishu/get` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/feishu/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/pairing/approve` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/qqbot/get` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/qqbot/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/telegram/get` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/telegram/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/wecom/get` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/wecom/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/weixin/get` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/channel/weixin/login` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/config-file/get` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/config-file/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/delete` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/delete/check` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/hermes/chat/sessions` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/hermes/chat/sessions/delete` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/hermes/chat/sessions/rename` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/model/get` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/model/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/other/get` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/other/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/overview` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/plugin/check` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/plugin/install` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/plugin/uninstall` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/plugin/upgrade` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/plugins/install` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/plugins/list` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/plugins/operate` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/plugins/search` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/remark` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/search` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/security/get` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/security/update` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/skills/install` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/skills/list` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/skills/search` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/skills/uninstall` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/skills/update` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/token/reset` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/website/bind` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/agents/website/unbind` | node/api/ai_routes_catalog.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/ai/domain/bind` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/domain/get` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/domain/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/gpu/search` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/mcp/domain/bind` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/mcp/domain/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/mcp/search` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/mcp/server` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/mcp/server/connection/test` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/mcp/server/del` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/mcp/server/detail` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/mcp/server/op` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/mcp/server/status/sync` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/mcp/server/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/ollama/close` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/ollama/model` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/ollama/model/del` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/ollama/model/load` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/ollama/model/recreate` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/ollama/model/search` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/ollama/model/sync` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/tensorrt/create` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/tensorrt/delete` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/tensorrt/operate` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/tensorrt/search` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/ai/tensorrt/update` | node/api/ai_routes_catalog.go | unknown | unknown | missing | - |

## alert

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/alert/clams/list` | node/api/functional_alerts.go | required | memory | missing | - |
| implemented | GET | `/api/v2/alert/disks/list` | node/api/functional_alerts.go | required | external | missing | - |
| implemented | POST | `/api/v2/alert/config/del` | node/api/functional_alerts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/alert/config/info` | node/api/functional_alerts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/alert/config/search` | node/api/functional_alerts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/alert/config/test` | node/api/functional_alerts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/alert/config/update` | node/api/functional_alerts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/alert/cronjob/list` | node/api/functional_alerts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/alert/del` | node/api/functional_alerts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/alert/logs/clean` | node/api/functional_alerts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/alert/logs/search` | node/api/functional_alerts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/alert/search` | node/api/functional_alerts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/alert/status` | node/api/functional_alerts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/alert/update` | node/api/functional_alerts.go | required | external | missing | - |

## apps

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/apps/:key` | node/api/apps_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/apps/checkupdate` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/apps/detail/:appId/:version/:type` | node/api/apps_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/apps/detail/node/:appKey/:version` | node/api/apps_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/apps/details/:id` | node/api/apps_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/apps/icon/:key` | node/api/apps_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/apps/ignored/detail` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/apps/installed/delete/check/:appInstallId` | node/api/apps_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/apps/installed/info/:appInstallId` | node/api/apps_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/apps/installed/list` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/apps/installed/params/:appInstallId` | node/api/apps_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/apps/services/:key` | node/api/apps_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/apps/tags` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/ignored/cancel` | node/api/apps_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/apps/install` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/check` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/conf` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/config/update` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/conninfo` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/ignore` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/loadport` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/op` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/params/update` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/port/change` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/search` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/sort/update` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/sync` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/installed/update/versions` | node/api/apps_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/apps/search` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/sync/local` | node/api/apps_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/apps/sync/remote` | node/api/apps_routes.go | unknown | external | missing | - |

## auth

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/core/auth/captcha` | node/api/legacy_routes_core.go | public | external | missing | - |
| implemented | GET | `/api/v2/core/auth/current` | node/api/legacy_routes_core.go | required | external | present | - |
| implemented | GET | `/api/v2/core/auth/passkey/list` | node/api/core_handlers.go | required | memory | present | - |
| implemented | GET | `/api/v2/core/auth/setting` | node/api/legacy_routes_core.go | required | external | missing | - |
| implemented | GET | `/api/v2/core/auth/welcome` | node/api/legacy_routes_core.go | public | external | missing | - |
| implemented | POST | `/api/v2/core/auth/api/generate` | node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/api/update` | node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/current/update` | node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/expired/reset` | node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/login` | node/api/legacy_routes_core.go | public | memory | present | - |
| implemented | POST | `/api/v2/core/auth/logout` | node/api/legacy_routes_core.go | required | external | present | - |
| implemented | POST | `/api/v2/core/auth/mfa` | node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/mfa/bind` | node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/mfa/close` | node/api/core_handlers.go | required | external | present | - |
| implemented | POST | `/api/v2/core/auth/mfalogin` | node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/passkey/begin` | node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/passkey/del` | node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/passkey/finish` | node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/passkey/register/begin` | node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/passkey/register/finish` | node/api/core_handlers.go | required | memory | present | - |

## backups

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/backups/check/:name` | node/api/functional_backup_accounts.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/backups/local` | node/api/functional_backup_accounts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/backups/options` | node/api/functional_backup_accounts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/core/backups/client/:clientType` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/backup` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/buckets` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/conn/check` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/del` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/record/del` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/record/description/update` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/record/download` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/record/search` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/record/search/bycronjob` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/record/size` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/recover` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/recover/byupload` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/refresh/token` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/search` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/search/files` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/backups/update` | node/api/functional_backup_accounts.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/backups/upload` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/core/backups/del` | node/api/functional_backup_accounts.go | required | external | missing | - |
| implemented | POST | `/api/v2/core/backups/refresh/token` | node/api/functional_backup_accounts.go | required | memory | missing | - |
| implemented | POST | `/api/v2/core/backups/update` | node/api/functional_backup_accounts.go | required | external | missing | - |

## commands

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/core/commands/del` | node/api/core_commands.go | required | external | present | - |
| implemented | POST | `/api/v2/core/commands/export` | node/api/core_commands.go | unknown | external | present | - |
| implemented | POST | `/api/v2/core/commands/import` | node/api/core_commands.go | unknown | external | present | - |
| implemented | POST | `/api/v2/core/commands/list` | node/api/core_commands.go | unknown | external | present | - |
| implemented | POST | `/api/v2/core/commands/search` | node/api/core_commands.go | unknown | external | present | - |
| implemented | POST | `/api/v2/core/commands/tree` | node/api/core_commands.go | unknown | external | present | - |
| implemented | POST | `/api/v2/core/commands/update` | node/api/core_commands.go | unknown | external | present | - |
| implemented | POST | `/api/v2/core/commands/upload` | node/api/core_commands.go | unknown | external | present | - |

## containers

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/containers/daemonjson` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/containers/daemonjson/file` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/containers/docker/status` | node/api/containers.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/containers/image` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/containers/image/all` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/containers/limit` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/containers/list/stats` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/containers/network` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/containers/repo` | node/api/containers.go | required | memory | missing | - |
| implemented | GET | `/api/v2/containers/search/log` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/containers/stats/:id` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/containers/status` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/containers/template` | node/api/containers.go | required | memory | missing | - |
| implemented | GET | `/api/v2/containers/volume` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/clean/log` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/commit` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/compose` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/clean/log` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/env` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/operate` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/pin` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/search` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/test` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/update` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/daemonjson/update` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/daemonjson/update/byfile` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/docker/operate` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/download/log` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/files/content` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/files/del` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/files/download` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/files/search` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/files/size` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/files/upload` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/image/build` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/image/load` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/image/pull` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/image/push` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/image/remove` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/image/save` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/image/search` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/image/tag` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/info` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/inspect` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/ipv6option/update` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/item/stats` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/list` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/list/byimage` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/logoption/update` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/network` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/network/del` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/network/search` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/operate` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/prune` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/rename` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/repo` | node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/repo/del` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/repo/search` | node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/repo/status` | node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/repo/update` | node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/search` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/template` | node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/template/batch` | node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/template/del` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/template/search` | node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/template/update` | node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/update` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/upgrade` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/users` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/volume` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/volume/del` | node/api/containers.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/containers/volume/search` | node/api/containers.go | unknown | unknown | missing | - |

## cronjobs

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/cronjobs/script/options` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/del` | node/api/cron_routes.go | unknown | database | missing | - |
| implemented | POST | `/api/v2/cronjobs/export` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/group/update` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/handle` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/import` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/load/info` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/next` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/records/clean` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/records/log` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/search` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/search/records` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/status` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/stop` | node/api/cron_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/update` | node/api/cron_routes.go | unknown | memory | missing | - |

## dashboard

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/dashboard/app/launcher` | node/api/legacy_routes_dashboard.go | required | external | present | - |
| implemented | GET | `/api/v2/dashboard/base/:ioOption/:netOption` | node/api/host_container_cron.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/dashboard/base/os` | node/api/host_container_cron.go | required | memory | missing | - |
| implemented | GET | `/api/v2/dashboard/current/:ioOption/:netOption` | node/api/host_container_cron.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/dashboard/current/node` | node/api/legacy_routes_dashboard.go | required | external | present | - |
| implemented | GET | `/api/v2/dashboard/current/top/cpu` | node/api/legacy_routes_dashboard.go | unknown | external | present | - |
| implemented | GET | `/api/v2/dashboard/current/top/mem` | node/api/legacy_routes_dashboard.go | unknown | external | present | - |
| implemented | GET | `/api/v2/dashboard/quick/option` | node/api/legacy_routes_dashboard.go | unknown | external | present | - |
| implemented | POST | `/api/v2/dashboard/app/launcher/option` | node/api/legacy_routes_dashboard.go | unknown | external | present | - |
| implemented | POST | `/api/v2/dashboard/app/launcher/show` | node/api/legacy_routes_dashboard.go | unknown | external | present | - |
| implemented | POST | `/api/v2/dashboard/quick/change` | node/api/legacy_routes_dashboard.go | unknown | external | present | - |
| implemented | POST | `/api/v2/dashboard/system/restart/:operation` | node/api/host_container_cron.go | unknown | external | missing | - |

## databases

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/databases/db/:name` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/databases/db/item/:type` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/databases/db/list/:type` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/databases/redis/check` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/databases/change/access` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/databases/change/password` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/databases/common/info` | node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/common/load/file` | node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/common/update/conf` | node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/db` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/databases/db/check` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/databases/db/del` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/databases/db/del/check` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/databases/db/search` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/databases/db/update` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/databases/del` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/databases/del/check` | node/api/database_routes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/databases/description/update` | node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/format/options` | node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/grants` | node/api/database_admin_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/databases/grants/del` | node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/grants/search` | node/api/database_admin_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/databases/grants/summary` | node/api/database_admin_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/databases/load` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/mongodb` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/bind` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/del` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/del/check` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/description` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/load` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/password` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/privileges` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/privileges/change` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/root/password` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/search` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/pg` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/pg/:database/load` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/pg/bind` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/pg/del` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/pg/del/check` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/pg/description` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/pg/password` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/pg/privileges` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/pg/search` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/redis/conf` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/redis/conf/update` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/redis/install/cli` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/redis/password` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/redis/persistence/conf` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/redis/persistence/update` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/redis/status` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/remote` | node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/search` | node/api/database_routes.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/databases/status` | node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/users` | node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/users/del` | node/api/database_admin_routes.go | unknown | unknown | present | - |
| implemented | POST | `/api/v2/databases/users/password` | node/api/database_admin_routes.go | unknown | unknown | present | - |
| implemented | POST | `/api/v2/databases/users/password/save` | node/api/database_admin_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/databases/users/search` | node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/users/update` | node/api/database_admin_routes.go | unknown | unknown | present | - |
| implemented | POST | `/api/v2/databases/variables` | node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/variables/update` | node/api/database_admin_routes.go | unknown | memory | present | - |

## files

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/files/download` | node/api/files_routes.go | required | external | present | - |
| implemented | GET | `/api/v2/files/recycle/status` | node/api/files_routes.go | unknown | external | present | - |
| implemented | GET | `/api/v2/files/share/check` | node/api/files_routes.go | required | external | present | - |
| implemented | GET | `/api/v2/files/share/download` | node/api/files_routes.go | required | external | present | - |
| implemented | GET | `/api/v2/files/share/info` | node/api/files_routes.go | required | external | present | - |
| implemented | GET | `/api/v2/files/share/qrcode` | node/api/files_routes.go | required | external | present | - |
| implemented | GET | `/api/v2/files/wget/process` | node/api/files_routes.go | required | external | present | - |
| implemented | GET | `/api/v2/files/wget/process/keys` | node/api/files_routes.go | required | external | present | - |
| implemented | POST | `/api/v2/files/ai-search` | node/api/files_routes.go | required | external | present | - |
| implemented | POST | `/api/v2/files/batch/check` | node/api/files_routes.go | required | external | present | - |
| implemented | POST | `/api/v2/files/batch/del` | node/api/files_routes.go | required | external | present | - |
| implemented | POST | `/api/v2/files/batch/role` | node/api/files_routes.go | required | external | present | - |
| implemented | POST | `/api/v2/files/check` | node/api/files_routes.go | required | external | present | - |
| implemented | POST | `/api/v2/files/chunkdownload` | node/api/files_routes.go | required | external | present | - |
| implemented | POST | `/api/v2/files/chunkupload` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/chunkupload/stop` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/compress` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/compress/stop` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/content` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/convert` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/convert/log` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/decompress` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/decompress/stop` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/del` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/depth/size` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/favorite` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/favorite/del` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/favorite/search` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/history/content` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/history/del` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/history/restore` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/history/search` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/mode` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/mount` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/move` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/move/stop` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/owner` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/preview` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/read/:type` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/recycle/clear` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/recycle/reduce` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/recycle/search` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/remark` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/remarks` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/rename` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/save` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/search` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/share/create` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/share/del` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/share/detail` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/share/search` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/size` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/tree` | node/api/files_routes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/files/upload` | node/api/files_routes.go | required | external | present | - |
| implemented | POST | `/api/v2/files/upload/search` | node/api/files_routes.go | unknown | unknown | present | - |
| implemented | POST | `/api/v2/files/user/group` | node/api/files_routes.go | unknown | unknown | present | - |
| implemented | POST | `/api/v2/files/wget` | node/api/files_routes.go | required | external | present | - |
| implemented | POST | `/api/v2/files/wget/stop` | node/api/files_routes.go | unknown | unknown | present | - |

## groups

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/core/groups/del` | node/api/core_resources.go | unknown | external | present | - |
| implemented | POST | `/api/v2/core/groups/search` | node/api/core_resources.go | unknown | external | present | - |
| implemented | POST | `/api/v2/core/groups/update` | node/api/core_resources.go | unknown | external | present | - |
| implemented | POST | `/api/v2/groups/del` | node/api/groups.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/groups/search` | node/api/groups.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/groups/update` | node/api/groups.go | unknown | memory | present | - |

## health

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/health/check` | cmd/workmesh-server/http_routes.go | required | memory | missing | - |

## hosts

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/hosts/components/:name` | node/api/host_container_cron.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/hosts/diagnostics/goroutines` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/hosts/diagnostics/summary` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/hosts/disks` | node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/firewall/docker/endpoints` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/hosts/firewall/docker/ports` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/hosts/firewall/rules/sync/task` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/hosts/firewall/settings` | node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/monitor/iooptions` | node/api/host_monitor_interfaces.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/monitor/netoptions` | node/api/host_monitor_interfaces.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/monitor/setting` | node/api/host_monitor_interfaces.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/terminal/container` | node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/terminal/local` | node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/terminal/ssh` | node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/tool/supervisor/process` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/del` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/diagnostics/profiles` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/disks/mount` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/disks/partition` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/disks/unmount` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/base` | node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/docker/operate` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/docker/policies/batch` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/docker/policies/delete/batch` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/docker/sync` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/filter/operate` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/forward/base` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/forward/enable` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/forward/operate` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/forward/search` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/operate` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/check` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/delete` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/native/detail` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/reorder` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/reset` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/search` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/sync` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/sync/preview` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/update` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/settings/operate` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/info` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/monitor/clean` | node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/monitor/search` | node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/monitor/setting/update` | node/api/host_monitor_interfaces.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/search` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/cert` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/cert/delete` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/cert/search` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/cert/sync` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/cert/update` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/file` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/file/update` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/log` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/log/clean` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/log/export` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/operate` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/search` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/update` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/test/byid` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/test/byinfo` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/tool/config/get` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/tool/config/set` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/tool/init` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/tool/operate` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/tool/status` | node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/tool/supervisor/process` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/tool/supervisor/process/file` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/tool/supervisor/process/file/get` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/tree` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/update` | node/api/hosts.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/hosts/update/group` | node/api/hosts.go | unknown | unknown | missing | - |

## images

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/images/*filename` | node/api/legacy_routes_misc.go | unknown | unknown | missing | - |

## logs

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/logs/system/files` | node/api/functional_logs.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/logs/system/services` | node/api/functional_logs.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/logs/system/status` | node/api/functional_logs.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/logs/tasks/executing/count` | node/api/functional_logs.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/core/logs/clean` | node/api/functional_logs.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/core/logs/login` | node/api/functional_logs.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/core/logs/operation` | node/api/functional_logs.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/logs/system/read` | node/api/functional_logs.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/logs/tasks/read` | node/api/functional_logs.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/logs/tasks/search` | node/api/functional_logs.go | unknown | external | missing | - |

## openresty

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/openresty/https` | node/api/legacy_routes_openresty.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/openresty/modules` | node/api/legacy_routes_openresty.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/openresty/status` | node/api/legacy_routes_openresty.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/openresty/build` | node/api/legacy_routes_openresty.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/openresty/file` | node/api/legacy_routes_openresty.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/openresty/https` | node/api/legacy_routes_openresty.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/openresty/modules/update` | node/api/legacy_routes_openresty.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/openresty/scope` | node/api/legacy_routes_openresty.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/openresty/update` | node/api/legacy_routes_openresty.go | unknown | memory | missing | - |

## process

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/process/:pid` | node/api/process.go | unknown | unknown | present | - |
| implemented | GET | `/api/v2/process/ws` | node/api/legacy_routes_misc.go | required | memory | present | - |
| implemented | POST | `/api/v2/process/listening` | node/api/process.go | required | memory | present | - |
| implemented | POST | `/api/v2/process/stop` | node/api/process.go | required | memory | present | - |

## runtimes

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/runtimes/:id` | node/api/legacy_routes_runtimes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/runtimes/installed/delete/check/:id` | node/api/legacy_routes_runtimes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/runtimes/php/:id/extensions` | node/api/runtime_php_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/runtimes/php/config/:id` | node/api/runtime_php_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/runtimes/php/container/:id` | node/api/legacy_routes_runtimes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/runtimes/php/fpm/config/:id` | node/api/legacy_routes_runtimes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/runtimes/php/fpm/status/:id` | node/api/legacy_routes_runtimes.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/runtimes/supervisor/process/:id` | node/api/legacy_routes_runtimes.go | unknown | unknown | present | - |
| implemented | POST | `/api/v2/runtimes/del` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/runtimes/node/modules` | node/api/legacy_routes_runtimes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/runtimes/node/modules/operate` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/runtimes/node/package` | node/api/legacy_routes_runtimes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/runtimes/operate` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/runtimes/php/config` | node/api/legacy_routes_runtimes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/runtimes/php/container/update` | node/api/legacy_routes_runtimes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/runtimes/php/extensions` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/runtimes/php/extensions/del` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/runtimes/php/extensions/install` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/runtimes/php/extensions/search` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/runtimes/php/extensions/uninstall` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/runtimes/php/extensions/update` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/runtimes/php/file` | node/api/legacy_routes_runtimes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/runtimes/php/fpm/config` | node/api/legacy_routes_runtimes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/runtimes/php/update` | node/api/legacy_routes_runtimes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/runtimes/remark` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/runtimes/search` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/runtimes/supervisor/process` | node/api/legacy_routes_runtimes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/runtimes/supervisor/process/file` | node/api/legacy_routes_runtimes.go | unknown | external | present | - |
| implemented | POST | `/api/v2/runtimes/sync` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/runtimes/update` | node/api/legacy_routes_runtimes.go | unknown | external | missing | - |

## script

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/core/script/run` | node/api/core_resources.go | required | external | present | - |
| implemented | POST | `/api/v2/core/script/del` | node/api/core_resources.go | unknown | external | present | - |
| implemented | POST | `/api/v2/core/script/search` | node/api/core_resources.go | unknown | external | present | - |
| implemented | POST | `/api/v2/core/script/sync` | node/api/core_resources.go | unknown | external | present | - |
| implemented | POST | `/api/v2/core/script/update` | node/api/core_resources.go | unknown | external | present | - |

## settings

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/core/settings/apps/store/config` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | GET | `/api/v2/core/settings/interface` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | GET | `/api/v2/core/settings/memo` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | GET | `/api/v2/core/settings/search/available` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | GET | `/api/v2/core/settings/ssl/info` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | GET | `/api/v2/core/settings/upgrade` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | GET | `/api/v2/core/settings/upgrade/releases` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | GET | `/api/v2/settings/basedir` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/settings/search/available` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/settings/snapshot/load` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/settings/ssh/conn` | node/api/legacy_routes_settings.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/settings/website/dir` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/apps/store/update` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/bind/update` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/memo` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/menu/default` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/menu/update` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/port/update` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/proxy/update` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/search` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/search/base` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/ssl/download` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/ssl/reload` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/ssl/update` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/terminal/search` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/terminal/update` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/update` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/upgrade` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | POST | `/api/v2/core/settings/upgrade/notes` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/description/save` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/file-history/search` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/file-history/update` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/files/ai/search` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/files/ai/update` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/search` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/snapshot` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/snapshot/del` | node/api/functional_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/snapshot/description/update` | node/api/functional_settings.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/settings/snapshot/import` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | POST | `/api/v2/settings/snapshot/recover` | node/api/functional_settings.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/settings/snapshot/recreate` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | POST | `/api/v2/settings/snapshot/rollback` | node/api/functional_settings.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/settings/snapshot/search` | node/api/functional_settings.go | required | memory | missing | - |
| implemented | POST | `/api/v2/settings/ssh` | node/api/legacy_routes_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/ssh/check` | node/api/legacy_routes_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/ssh/check/info` | node/api/legacy_routes_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/ssh/default` | node/api/legacy_routes_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/terminal/ai/search` | node/api/legacy_routes_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/terminal/ai/update` | node/api/legacy_routes_settings.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/settings/update` | node/api/functional_settings.go | unknown | memory | missing | - |

## static

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/static/*filename` | node/api/legacy_routes_misc.go | unknown | unknown | missing | - |

## toolbox

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/toolbox/device/users` | node/api/legacy_routes_toolbox.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/toolbox/device/zone/options` | node/api/legacy_routes_toolbox.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/toolbox/fail2ban/base` | node/api/legacy_routes_toolbox.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/toolbox/fail2ban/load/conf` | node/api/legacy_routes_toolbox.go | unknown | external | missing | - |
| implemented | GET | `/api/v2/toolbox/ftp/base` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clam` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clam/base` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clam/del` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clam/file/search` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clam/file/update` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clam/handle` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clam/operate` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clam/record/clean` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clam/record/search` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clam/search` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clam/status/update` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clam/update` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/clean` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/device/base` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/device/check/dns` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/device/conf` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/device/update/byconf` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/device/update/conf` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/device/update/host` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/device/update/passwd` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/device/update/swap` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/fail2ban/operate` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/fail2ban/operate/sshd` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/fail2ban/search` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/fail2ban/update` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/fail2ban/update/byconf` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/ftp` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/ftp/del` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/ftp/log/search` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/ftp/operate` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/ftp/search` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/ftp/sync` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/ftp/update` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/toolbox/scan` | node/api/legacy_routes_toolbox.go | unknown | memory | missing | - |

## unknown

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/` | cmd/workmesh-server/http_routes.go | unknown | database | missing | - |
| implemented | GET | `/assets/*filepath` | cmd/workmesh-server/http_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/favicon.ico` | cmd/workmesh-server/http_routes.go | unknown | external | missing | - |
| implemented | GET | `/favicon.ico/*filepath` | cmd/workmesh-server/http_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/public/*filepath` | cmd/workmesh-server/http_routes.go | unknown | unknown | missing | - |
| implemented | GET | `/swagger/*any` | cmd/workmesh-server/http_routes.go | unknown | unknown | missing | - |
| implemented | HEAD | `/assets/*filepath` | cmd/workmesh-server/http_routes.go | unknown | unknown | missing | - |
| implemented | HEAD | `/favicon.ico` | cmd/workmesh-server/http_routes.go | unknown | external | missing | - |
| implemented | HEAD | `/favicon.ico/*filepath` | cmd/workmesh-server/http_routes.go | unknown | unknown | missing | - |
| implemented | HEAD | `/public/*filepath` | cmd/workmesh-server/http_routes.go | unknown | unknown | missing | - |

## websites

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/websites/:id` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/websites/:id/config/:type` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/websites/:id/https` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/websites/:id/lbs` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/websites/ca/:id` | node/api/legacy_routes_websites.go | unknown | unknown | present | - |
| implemented | GET | `/api/v2/websites/cors/:id` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/websites/databases` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | GET | `/api/v2/websites/default/html/:type` | node/api/legacy_routes_websites.go | required | unknown | missing | - |
| implemented | GET | `/api/v2/websites/domains/:websiteId` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/websites/list` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/websites/proxy/config/:id` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/websites/realip/config/:id` | node/api/legacy_routes_websites.go | required | unknown | missing | - |
| implemented | GET | `/api/v2/websites/resource/:id` | node/api/legacy_routes_websites.go | required | unknown | missing | - |
| implemented | GET | `/api/v2/websites/rewrite/custom` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | GET | `/api/v2/websites/ssl/:id` | node/api/ssl.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/websites/ssl/website/:websiteId` | node/api/ssl.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/websites/:id/https` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/websites/acme/del` | node/api/legacy_routes_websites.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/acme/search` | node/api/legacy_routes_websites.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/acme/update` | node/api/legacy_routes_websites.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/auths` | node/api/legacy_routes_websites.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/websites/auths/path` | node/api/legacy_routes_websites.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/websites/auths/path/update` | node/api/legacy_routes_websites.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/websites/auths/update` | node/api/legacy_routes_websites.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/websites/batch/group` | node/api/legacy_routes_websites.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/websites/batch/operate` | node/api/legacy_routes_websites.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/websites/batch/ssl` | node/api/legacy_routes_websites.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/websites/ca/del` | node/api/legacy_routes_websites.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/ca/download` | node/api/legacy_routes_websites.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/ca/obtain` | node/api/legacy_routes_websites.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/ca/renew` | node/api/legacy_routes_websites.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/ca/search` | node/api/legacy_routes_websites.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/check` | node/api/legacy_routes_websites.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/config` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/config/update` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/cors/update` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/crosssite` | node/api/legacy_routes_websites.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/websites/databases` | node/api/legacy_routes_websites.go | required | unknown | missing | - |
| implemented | POST | `/api/v2/websites/default/html/update` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/default/server` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/del` | node/api/legacy_routes_websites.go | required | database | missing | - |
| implemented | POST | `/api/v2/websites/dir` | node/api/legacy_routes_websites.go | required | external | missing | - |
| implemented | POST | `/api/v2/websites/dir/permission` | node/api/legacy_routes_websites.go | required | external | missing | - |
| implemented | POST | `/api/v2/websites/dir/update` | node/api/legacy_routes_websites.go | required | external | missing | - |
| implemented | POST | `/api/v2/websites/dns/del` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/dns/search` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/dns/update` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/domains` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/domains/del` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/domains/update` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/exec/composer` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/websites/group/change` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/websites/lbs/create` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/lbs/del` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/websites/lbs/file` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/lbs/update` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/leech` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/leech/update` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/log/operate` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/websites/log/search` | node/api/legacy_routes_websites.go | unknown | unknown | missing | - |
| implemented | POST | `/api/v2/websites/nginx/update` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/operate` | node/api/legacy_routes_websites.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/options` | node/api/legacy_routes_websites.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/php/version` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/proxies` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/proxies/delete` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/proxies/file` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/proxies/status` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/proxies/update` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/proxy/clear` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/proxy/config` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/realip/config` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/redirect` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/redirect/file` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/redirect/update` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/rewrite` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/rewrite/custom` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/rewrite/update` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/search` | node/api/legacy_routes_websites.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/websites/ssl/del` | node/api/ssl.go | unknown | database | missing | - |
| implemented | POST | `/api/v2/websites/ssl/download` | node/api/ssl.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/websites/ssl/import` | node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/list` | node/api/ssl.go | unknown | database | missing | - |
| implemented | POST | `/api/v2/websites/ssl/obtain` | node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/push` | node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/resolve` | node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/search` | node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/update` | node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/upload` | node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/upload/file` | node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/stream/update` | node/api/legacy_routes_websites.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/templates/del` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/templates/get` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/templates/outputs` | node/api/legacy_routes_websites.go | required | memory | missing | - |
| implemented | POST | `/api/v2/websites/templates/outputs/del` | node/api/legacy_routes_websites.go | required | database | missing | - |
| implemented | POST | `/api/v2/websites/templates/outputs/get` | node/api/legacy_routes_websites.go | required | database | missing | - |
| implemented | POST | `/api/v2/websites/templates/outputs/search` | node/api/legacy_routes_websites.go | required | database | missing | - |
| implemented | POST | `/api/v2/websites/templates/preview` | node/api/legacy_routes_websites.go | required | database | missing | - |
| implemented | POST | `/api/v2/websites/templates/search` | node/api/legacy_routes_websites.go | required | database | missing | - |
| implemented | POST | `/api/v2/websites/templates/update` | node/api/legacy_routes_websites.go | required | database | missing | - |
| implemented | POST | `/api/v2/websites/templates/upload` | node/api/legacy_routes_websites.go | required | database | present | - |
| implemented | POST | `/api/v2/websites/update` | node/api/legacy_routes_websites.go | unknown | database | missing | - |

## 国际化完整性批次

| 功能 | 来源 | 新实现 | 覆盖 | 状态 |
| --- | --- | --- | --- | --- |
| Core/Agent 后端语言包与前端语言入口 | 只读参考 `/www/apps/1Panel/core/i18n`、`/www/apps/1Panel/agent/i18n`、`/www/apps/1Panel/frontend` | `i18n/i18n.go`、`i18n/lang/*.yaml`、`web/src/lang` 与各页面入口 | 语言键和菜单入口通过 `i18n-scan.mjs` | implemented |

