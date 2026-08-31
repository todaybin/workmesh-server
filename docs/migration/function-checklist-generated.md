<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh 功能迁移逐路由清单

基线来源：旧 `apps/workmesh-node/core` 与 `agent` 全源码，生成时间：2026-08-31T02:48:59.216Z。
共 871 条接口：implemented 871。

状态定义：`implemented`=已实现并有具体处理器，`partial`=具体处理器仍返回固定空数据或存在 TODO，`compatibility`=兼容占位，`pending`=迁移中，`missing`=未发现注册。

## access-lists

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/access-lists` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/access-lists` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |

## ai

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/ai/accounts/providers` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/ai/gpu/load` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/ai/gpu/options` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/ai/mcp/domain/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/accounts` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/accounts/counts` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/accounts/delete` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/accounts/models` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/accounts/models/create` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/accounts/models/delete` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/accounts/models/discover` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/accounts/models/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/accounts/search` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/accounts/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/accounts/verify` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/agent/bind` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/agent/channels` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/agent/create` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/agent/delete` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/agent/list` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/agent/md/list` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/agent/md/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/agent/unbind` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/batch/install` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/batch/operate` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/batch/skill/install` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/batch/upgrade` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/delete` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/dingtalk/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/dingtalk/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/discord/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/discord/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/feishu/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/feishu/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/pairing/approve` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/qqbot/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/qqbot/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/telegram/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/telegram/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/wecom/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/wecom/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/weixin/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/channel/weixin/login` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/config-file/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/config-file/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/delete` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/delete/check` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/hermes/chat/sessions` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/hermes/chat/sessions/delete` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/hermes/chat/sessions/rename` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/model/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/model/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/other/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/other/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/overview` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/plugin/check` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/plugin/install` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/plugin/uninstall` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/plugin/upgrade` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/plugins/install` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/plugins/list` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/plugins/operate` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/plugins/search` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/remark` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/search` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/security/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/security/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/skills/install` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/skills/list` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/skills/search` | apps/workmesh-server/node/api/ai_execution.go | required | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/skills/uninstall` | apps/workmesh-server/node/api/ai_execution.go | required | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/skills/update` | apps/workmesh-server/node/api/ai_execution.go | required | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/token/reset` | apps/workmesh-server/node/api/ai_execution.go | required | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/website/bind` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/agents/website/unbind` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/domain/bind` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/domain/get` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/domain/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/gpu/search` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/mcp/domain/bind` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/mcp/domain/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/mcp/search` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/mcp/server` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/mcp/server/connection/test` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/mcp/server/del` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/mcp/server/detail` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/mcp/server/op` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/mcp/server/status/sync` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/mcp/server/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/ollama/close` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/ollama/model` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/ollama/model/del` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/ollama/model/load` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/ollama/model/recreate` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/ollama/model/search` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/ollama/model/sync` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/tensorrt/create` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/tensorrt/delete` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/tensorrt/operate` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/tensorrt/search` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/ai/tensorrt/update` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |

## alert

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/alert/clams/list` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | GET | `/api/v2/alert/disks/list` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/alert/config/del` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/alert/config/info` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/alert/config/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/alert/config/test` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/alert/config/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/alert/cronjob/list` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/alert/del` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/alert/logs/clean` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/alert/logs/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/alert/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/alert/status` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/alert/update` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |

## apps

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/apps/:key` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/apps/checkupdate` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/apps/detail/:appId/:version/:type` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/apps/detail/node/:appKey/:version` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/apps/details/:id` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/apps/icon/:key` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/apps/ignored/detail` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/apps/installed/delete/check/:appInstallId` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/apps/installed/info/:appInstallId` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/apps/installed/list` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/apps/installed/params/:appInstallId` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/apps/services/:key` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/apps/tags` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/ignored/cancel` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/install` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/check` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/conf` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/config/update` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/conninfo` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/ignore` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/loadport` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/op` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/params/update` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/port/change` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/search` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/sort/update` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/sync` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/installed/update/versions` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/search` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/sync/local` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/apps/sync/remote` | apps/workmesh-server/node/api/apps.go | unknown | memory | present | - |

## attack

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/attack/stat` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |

## auth

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/core/auth/captcha` | apps/workmesh-server/node/api/legacy_routes.go | public | memory | missing | - |
| implemented | GET | `/api/v2/core/auth/current` | apps/workmesh-server/node/api/router.go | required | unknown | present | - |
| implemented | GET | `/api/v2/core/auth/passkey/list` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |
| implemented | GET | `/api/v2/core/auth/setting` | apps/workmesh-server/node/api/legacy_routes.go | required | memory | missing | - |
| implemented | GET | `/api/v2/core/auth/welcome` | apps/workmesh-server/node/api/legacy_routes.go | public | memory | missing | - |
| implemented | POST | `/api/v2/core/auth/api/generate` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/api/update` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/current/update` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/expired/reset` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/login` | apps/workmesh-server/node/api/router.go | public | memory | present | - |
| implemented | POST | `/api/v2/core/auth/logout` | apps/workmesh-server/node/api/router.go | required | unknown | present | - |
| implemented | POST | `/api/v2/core/auth/mfa` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/mfa/bind` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/mfa/close` | apps/workmesh-server/node/api/core_handlers.go | required | external | present | - |
| implemented | POST | `/api/v2/core/auth/mfalogin` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/passkey/begin` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/passkey/del` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/passkey/finish` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/passkey/register/begin` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/auth/passkey/register/finish` | apps/workmesh-server/node/api/core_handlers.go | required | memory | present | - |

## backups

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/backups/check/:name` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/backups/local` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/backups/options` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/core/backups/client/:clientType` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/backups/backup` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/backups/buckets` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/backups/conn/check` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/backups/del` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/backups/record/del` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/backups/record/description/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/backups/record/download` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/backups/record/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/backups/record/search/bycronjob` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/backups/record/size` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/backups/recover` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/backups/recover/byupload` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/backups/refresh/token` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/backups/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/backups/search/files` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/backups/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/backups/upload` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/backups` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/backups/del` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/backups/refresh/token` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/backups/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |

## block

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/block/search` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |

## commands

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/core/commands/del` | apps/workmesh-server/node/api/core_commands.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/commands/export` | apps/workmesh-server/node/api/core_commands.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/commands/import` | apps/workmesh-server/node/api/core_commands.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/commands/list` | apps/workmesh-server/node/api/core_commands.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/commands/search` | apps/workmesh-server/node/api/core_commands.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/commands/tree` | apps/workmesh-server/node/api/core_commands.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/commands/update` | apps/workmesh-server/node/api/core_commands.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/commands/upload` | apps/workmesh-server/node/api/core_commands.go | unknown | memory | present | - |

## config

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/config/global` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/config/global` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/config/site` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/config/site/update` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |

## containers

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/containers/daemonjson` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/containers/daemonjson/file` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/containers/docker/status` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/containers/image` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/containers/image/all` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/containers/limit` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/containers/list/stats` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/containers/network` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/containers/repo` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | GET | `/api/v2/containers/search/log` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/containers/stats/:id` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/containers/status` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | GET | `/api/v2/containers/template` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | GET | `/api/v2/containers/volume` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/clean/log` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/commit` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/clean/log` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/env` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/operate` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/pin` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/search` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/test` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/compose/update` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/daemonjson/update` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/daemonjson/update/byfile` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/docker/operate` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/download/log` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/files/content` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/files/del` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/files/download` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/files/search` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/files/size` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/files/upload` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/image/build` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/image/load` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/image/pull` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/image/push` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/image/remove` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/image/save` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/image/search` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/image/tag` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/info` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/inspect` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/ipv6option/update` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/item/stats` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/list` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/list/byimage` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/logoption/update` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/network` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/network/del` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/network/search` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/operate` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/prune` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/rename` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/repo` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/repo/del` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/repo/search` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/repo/status` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/repo/update` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/search` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/template` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/template/batch` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/template/del` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/template/search` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/template/update` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/update` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/upgrade` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/users` | apps/workmesh-server/node/api/containers.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/containers/volume` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/volume/del` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |
| implemented | POST | `/api/v2/containers/volume/search` | apps/workmesh-server/node/api/containers.go | required | memory | missing | - |

## cronjobs

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/cronjobs/script/options` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/del` | apps/workmesh-server/node/api/host_container_cron.go | unknown | database | missing | - |
| implemented | POST | `/api/v2/cronjobs/export` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/group/update` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/handle` | apps/workmesh-server/node/api/host_container_cron.go | unknown | database | missing | - |
| implemented | POST | `/api/v2/cronjobs/import` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/load/info` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/next` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/records/clean` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/records/log` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/search` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/search/records` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/status` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/stop` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/cronjobs/update` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |

## cubesandbox

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/cubesandbox/health` | apps/workmesh-server/node/api/ai_execution.go | public | memory | present | - |
| implemented | GET | `/api/v2/cubesandbox/status` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/cubesandbox/reconcile` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/cubesandbox/start` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/cubesandbox/stop` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |

## dashboard

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/dashboard/app/launcher` | apps/workmesh-server/node/api/router.go | required | unknown | present | - |
| implemented | GET | `/api/v2/dashboard/base/:ioOption/:netOption` | apps/workmesh-server/node/api/host_container_cron.go | required | external | missing | - |
| implemented | GET | `/api/v2/dashboard/base/os` | apps/workmesh-server/node/api/host_container_cron.go | required | memory | missing | - |
| implemented | GET | `/api/v2/dashboard/current/:ioOption/:netOption` | apps/workmesh-server/node/api/host_container_cron.go | required | external | missing | - |
| implemented | GET | `/api/v2/dashboard/current/node` | apps/workmesh-server/node/api/router.go | required | unknown | present | - |
| implemented | GET | `/api/v2/dashboard/current/top/cpu` | apps/workmesh-server/node/api/router.go | unknown | unknown | present | - |
| implemented | GET | `/api/v2/dashboard/current/top/mem` | apps/workmesh-server/node/api/router.go | unknown | unknown | present | - |
| implemented | GET | `/api/v2/dashboard/quick/option` | apps/workmesh-server/node/api/router.go | unknown | unknown | present | - |
| implemented | POST | `/api/v2/dashboard/app/launcher/option` | apps/workmesh-server/node/api/router.go | unknown | unknown | present | - |
| implemented | POST | `/api/v2/dashboard/app/launcher/show` | apps/workmesh-server/node/api/router.go | unknown | unknown | present | - |
| implemented | POST | `/api/v2/dashboard/quick/change` | apps/workmesh-server/node/api/router.go | unknown | unknown | present | - |
| implemented | POST | `/api/v2/dashboard/system/restart/:operation` | apps/workmesh-server/node/api/host_container_cron.go | unknown | memory | missing | - |

## databases

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/databases/db/:name` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/databases/db/item/:type` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/databases/db/list/:type` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/databases/redis/check` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/change/access` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/change/password` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/common/info` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/common/load/file` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/common/update/conf` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/db` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/db/check` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/db/del` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/db/del/check` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/db/search` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/db/update` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/del` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/del/check` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/description/update` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/format/options` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/grants` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/grants/del` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/grants/search` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/grants/summary` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/load` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/mongodb` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/bind` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/del` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/del/check` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/description` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/load` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/password` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/privileges` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/privileges/change` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/root/password` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/mongodb/search` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/pg` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/pg/:database/load` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/pg/bind` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/pg/del` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/pg/del/check` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/pg/description` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/pg/password` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/pg/privileges` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/pg/search` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/redis/conf` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/redis/conf/update` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/redis/install/cli` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/redis/password` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/redis/persistence/conf` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/redis/persistence/update` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/redis/status` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/remote` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/search` | apps/workmesh-server/node/api/database_routes.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/databases/status` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/users` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/users/del` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/users/password` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/users/password/save` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/users/search` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/users/update` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/variables` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/databases/variables/update` | apps/workmesh-server/node/api/database_admin_routes.go | unknown | memory | present | - |

## deployment

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/deployment/status` | apps/workmesh-server/node/api/deployment_runtime.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/deployment/execute` | apps/workmesh-server/node/api/deployment_runtime.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/deployment/rollback` | apps/workmesh-server/node/api/deployment_runtime.go | unknown | memory | present | - |

## deployment-artifact

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/deployment-artifact/activate` | apps/workmesh-server/node/api/deployment_runtime.go | unknown | memory | present | - |

## deployment-manifest

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/deployment-manifest/verify` | apps/workmesh-server/node/api/deployment_runtime.go | unknown | memory | present | - |

## files

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/files/download` | apps/workmesh-server/node/api/files_routes.go | required | memory | present | - |
| implemented | GET | `/api/v2/files/recycle/status` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/files/share/check` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/files/share/download` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/files/share/info` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/files/share/qrcode` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/files/wget/process` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/files/wget/process/keys` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/ai-search` | apps/workmesh-server/node/api/files_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/files/batch/check` | apps/workmesh-server/node/api/files_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/files/batch/del` | apps/workmesh-server/node/api/files_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/files/batch/role` | apps/workmesh-server/node/api/files_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/files/check` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/chunkdownload` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/chunkupload` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/chunkupload/stop` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/compress` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/compress/stop` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/content` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/convert` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/convert/log` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/decompress` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/decompress/stop` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/del` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/depth/size` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/favorite` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/favorite/del` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/favorite/search` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/history/content` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/history/del` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/history/restore` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/history/search` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/mode` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/mount` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/move` | apps/workmesh-server/node/api/files_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/files/move/stop` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/owner` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/preview` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/read/:type` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/recycle/clear` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/recycle/reduce` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/recycle/search` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/remark` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/remarks` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/rename` | apps/workmesh-server/node/api/files_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/files/save` | apps/workmesh-server/node/api/files_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/files/search` | apps/workmesh-server/node/api/files_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/files/share/create` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/share/del` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/share/detail` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/share/search` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/size` | apps/workmesh-server/node/api/files_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/files/tree` | apps/workmesh-server/node/api/files_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/files/upload` | apps/workmesh-server/node/api/files_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/files/upload/search` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/user/group` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/wget` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/files/wget/stop` | apps/workmesh-server/node/api/files_routes.go | unknown | memory | present | - |

## global

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/global` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |

## groups

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/core/groups/del` | apps/workmesh-server/node/api/core_resources.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/groups/search` | apps/workmesh-server/node/api/core_resources.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/groups/update` | apps/workmesh-server/node/api/core_resources.go | required | memory | present | - |
| implemented | POST | `/api/v2/groups/del` | apps/workmesh-server/node/api/core_resources.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/groups/search` | apps/workmesh-server/node/api/core_resources.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/groups/update` | apps/workmesh-server/node/api/core_resources.go | unknown | memory | present | - |

## health

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/health/check` | apps/workmesh-server/cmd/workmesh-server/main.go | required | memory | present | - |

## hosts

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/hosts/components/:name` | apps/workmesh-server/node/api/hosts.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/hosts/diagnostics/goroutines` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/diagnostics/summary` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/disks` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/firewall/docker/endpoints` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/firewall/docker/ports` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/firewall/rules/sync/task` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/firewall/settings` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/monitor/iooptions` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/monitor/netoptions` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/monitor/setting` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | GET | `/api/v2/hosts/terminal/container` | apps/workmesh-server/node/api/hosts.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/hosts/terminal/local` | apps/workmesh-server/node/api/hosts.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/hosts/terminal/ssh` | apps/workmesh-server/node/api/hosts.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/hosts/tool/supervisor/process` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/del` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/diagnostics/profiles` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/disks/mount` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/disks/partition` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/disks/unmount` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/base` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/docker/operate` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/docker/policies/batch` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/docker/policies/delete/batch` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/docker/sync` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/filter/operate` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/forward/base` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/forward/enable` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/forward/operate` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/forward/search` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/operate` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/check` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/delete` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/native/detail` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/reorder` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/reset` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/search` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/sync` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/sync/preview` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/rules/update` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/firewall/settings/operate` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/info` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/monitor/clean` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/monitor/search` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/monitor/setting/update` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/search` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/cert` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/cert/delete` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/cert/search` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/cert/sync` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/cert/update` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/file` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/file/update` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/log` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/log/clean` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/log/export` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/operate` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/search` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/ssh/update` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/test/byid` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/test/byinfo` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/tool/config/get` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/tool/config/set` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/tool/init` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/tool/operate` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/tool/status` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/tool/supervisor/process` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/tool/supervisor/process/file` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/tool/supervisor/process/file/get` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/tree` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/update` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/hosts/update/group` | apps/workmesh-server/node/api/hosts.go | unknown | memory | missing | - |

## images

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/images/*filename` | apps/workmesh-server/cmd/workmesh-server/main.go | unknown | memory | present | - |

## log

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/log/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |

## logs

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/logs/system/files` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/logs/system/services` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/logs/system/status` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/logs/tasks/executing/count` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/logs/clean` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/logs/login` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/logs/operation` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/logs/clear` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/logs/detail` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/logs/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/logs/stat` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/logs/system/read` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/logs/tasks/read` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/logs/tasks/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |

## openresty

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/openresty/https` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/openresty/modules` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/openresty/status` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/openresty/build` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/openresty/file` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/openresty/https` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/openresty/modules/update` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/openresty/scope` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/openresty/update` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |

## process

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/process/:pid` | apps/workmesh-server/node/api/process.go | unknown | unknown | present | - |
| implemented | GET | `/api/v2/process/ws` | apps/workmesh-server/node/api/process.go | required | memory | present | - |
| implemented | POST | `/api/v2/process/listening` | apps/workmesh-server/node/api/process.go | required | memory | present | - |
| implemented | POST | `/api/v2/process/stop` | apps/workmesh-server/node/api/process.go | required | memory | present | - |

## qps

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/qps` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |

## rank

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/rank` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |

## relation

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/relation/stat` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |

## rules

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/rules` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/rules/delete` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |

## runtimes

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/runtimes/:id` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/runtimes/installed/delete/check/:id` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/runtimes/php/:id/extensions` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | unknown | present | - |
| implemented | GET | `/api/v2/runtimes/php/config/:id` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | unknown | present | - |
| implemented | GET | `/api/v2/runtimes/php/container/:id` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/runtimes/php/fpm/config/:id` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/runtimes/php/fpm/status/:id` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/runtimes/supervisor/process/:id` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/del` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/node/modules` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/node/modules/operate` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/node/package` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/operate` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/php/config` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/php/container/update` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/php/extensions` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/php/extensions/del` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/php/extensions/install` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/php/extensions/search` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/php/extensions/uninstall` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/php/extensions/update` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/php/file` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/php/fpm/config` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/php/update` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/remark` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/search` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/supervisor/process` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/supervisor/process/file` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/sync` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/runtimes/update` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |

## script

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/core/script/run` | apps/workmesh-server/node/api/core_resources.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/script/del` | apps/workmesh-server/node/api/core_resources.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/script/search` | apps/workmesh-server/node/api/core_resources.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/script/sync` | apps/workmesh-server/node/api/core_resources.go | required | memory | present | - |
| implemented | POST | `/api/v2/core/script/update` | apps/workmesh-server/node/api/core_resources.go | required | memory | present | - |

## settings

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/core/settings/apps/store/config` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/core/settings/interface` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/core/settings/memo` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/core/settings/search/available` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/core/settings/ssl/info` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/core/settings/upgrade` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/core/settings/upgrade/releases` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/settings/basedir` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/settings/search/available` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/settings/snapshot/load` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/settings/ssh/conn` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/settings/website/dir` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/apps/store/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/bind/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/memo` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/menu/default` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/menu/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/port/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/proxy/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/search/base` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/ssl/download` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/ssl/reload` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/ssl/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/terminal/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/terminal/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/upgrade` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/core/settings/upgrade/notes` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/description/save` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/file-history/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/file-history/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/files/ai/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/files/ai/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/search` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/snapshot` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/snapshot/del` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/snapshot/description/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/snapshot/import` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/settings/snapshot/recover` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/snapshot/recreate` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/settings/snapshot/rollback` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/snapshot/search` | apps/workmesh-server/node/api/functional_domains.go | required | memory | present | - |
| implemented | POST | `/api/v2/settings/ssh` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/ssh/check` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/ssh/check/info` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/ssh/default` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/terminal/ai/search` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/terminal/ai/update` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/settings/update` | apps/workmesh-server/node/api/functional_domains.go | unknown | memory | present | - |

## sites

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/sites` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/sites/:id/rules` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/sites` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |

## standard-rules

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/standard-rules` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |

## stat

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/stat` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |

## static

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/static/*filename` | apps/workmesh-server/cmd/workmesh-server/main.go | unknown | memory | present | - |

## status

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/status` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |

## test

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/test` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |

## toolbox

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/toolbox/device/users` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/toolbox/device/zone/options` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/toolbox/fail2ban/base` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/toolbox/fail2ban/load/conf` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/toolbox/ftp/base` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clam` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clam/base` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clam/del` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clam/file/search` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clam/file/update` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clam/handle` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clam/operate` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clam/record/clean` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clam/record/search` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clam/search` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clam/status/update` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clam/update` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/clean` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/device/base` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/device/check/dns` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/device/conf` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/device/update/byconf` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/device/update/conf` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/device/update/host` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/device/update/passwd` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/device/update/swap` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/fail2ban/operate` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/fail2ban/operate/sshd` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/fail2ban/search` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/fail2ban/update` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/fail2ban/update/byconf` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/ftp` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/ftp/del` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/ftp/log/search` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/ftp/operate` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/ftp/search` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/ftp/sync` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/ftp/update` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/toolbox/scan` | apps/workmesh-server/node/api/runtime_toolbox.go | unknown | memory | present | - |

## trend

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/trend` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |

## unknown

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/` | apps/workmesh-server/cmd/workmesh-server/main.go | required | external | present | - |
| implemented | GET | `/assets/*filepath` | apps/workmesh-server/cmd/workmesh-server/main.go | unknown | unknown | present | - |
| implemented | GET | `/favicon.ico` | apps/workmesh-server/cmd/workmesh-server/main.go | unknown | memory | present | - |
| implemented | GET | `/favicon.ico/*filepath` | apps/workmesh-server/cmd/workmesh-server/main.go | unknown | unknown | present | - |
| implemented | GET | `/public/*filepath` | apps/workmesh-server/cmd/workmesh-server/main.go | unknown | unknown | present | - |
| implemented | GET | `/swagger/*any` | apps/workmesh-server/cmd/workmesh-server/main.go | unknown | unknown | present | - |
| implemented | HEAD | `/assets/*filepath` | apps/workmesh-server/cmd/workmesh-server/main.go | unknown | unknown | present | - |
| implemented | HEAD | `/favicon.ico` | apps/workmesh-server/cmd/workmesh-server/main.go | unknown | memory | present | - |
| implemented | HEAD | `/favicon.ico/*filepath` | apps/workmesh-server/cmd/workmesh-server/main.go | unknown | unknown | present | - |
| implemented | HEAD | `/public/*filepath` | apps/workmesh-server/cmd/workmesh-server/main.go | unknown | unknown | present | - |

## visitors

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | POST | `/api/v2/visitors` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/visitors/loc` | apps/workmesh-server/node/api/analytics.go | unknown | memory | present | - |

## websites

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/websites/:id` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/:id/config/:type` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/:id/https` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/:id/lbs` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/ca/:id` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/cors/:id` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/databases` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | GET | `/api/v2/websites/default/html/:type` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | GET | `/api/v2/websites/domains/:websiteId` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/list` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/monitor/config/global` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/proxy/config/:id` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/realip/config/:id` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/resource/:id` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/rewrite/custom` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | GET | `/api/v2/websites/ssl/:id` | apps/workmesh-server/node/api/ssl.go | unknown | unknown | present | - |
| implemented | GET | `/api/v2/websites/ssl/website/:websiteId` | apps/workmesh-server/node/api/ssl.go | unknown | unknown | missing | - |
| implemented | GET | `/api/v2/websites/waf/access-lists` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/waf/sites` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/waf/sites/:id/rules` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/waf/standard-rules` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | GET | `/api/v2/websites/waf/status` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites` | apps/workmesh-server/node/api/ssl.go | required | database | present | - |
| implemented | POST | `/api/v2/websites/:id/https` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/acme/del` | apps/workmesh-server/node/api/website_cert_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/acme/search` | apps/workmesh-server/node/api/website_cert_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/acme/update` | apps/workmesh-server/node/api/website_cert_routes.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/auths` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/auths/path` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/auths/path/update` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/auths/update` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/batch/group` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/batch/operate` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/batch/ssl` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/ca/del` | apps/workmesh-server/node/api/website_cert_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/ca/download` | apps/workmesh-server/node/api/website_cert_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/ca/obtain` | apps/workmesh-server/node/api/website_cert_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/ca/renew` | apps/workmesh-server/node/api/website_cert_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/ca/search` | apps/workmesh-server/node/api/website_cert_routes.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/check` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/config` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/config/update` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/cors/update` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/crosssite` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/databases` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/default/html/update` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/default/server` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/del` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/dir` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/dir/permission` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/dir/update` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/dns/del` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/dns/search` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/dns/update` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/domains` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/domains/del` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/domains/update` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/exec/composer` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/group/change` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/lbs/create` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/lbs/del` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/lbs/file` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/lbs/update` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/leech` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/leech/update` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/log/operate` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/log/search` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/config/global` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/config/site` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/config/site/update` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/logs/clear` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/logs/detail` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/logs/search` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/logs/stat` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/qps` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/rank` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/stat` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/trend` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/visitors` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/visitors/loc` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/monitor/websites` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/nginx/update` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/operate` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/options` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/php/version` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/proxies` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/proxies/delete` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/proxies/file` | apps/workmesh-server/node/api/website_extensions.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/proxies/status` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/proxies/update` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/proxy/clear` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/proxy/config` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/realip/config` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/redirect` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/redirect/file` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/redirect/update` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/rewrite` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/rewrite/custom` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/rewrite/update` | apps/workmesh-server/node/api/website.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/search` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/ssl/del` | apps/workmesh-server/node/api/ssl.go | unknown | database | missing | - |
| implemented | POST | `/api/v2/websites/ssl/download` | apps/workmesh-server/node/api/ssl.go | unknown | external | missing | - |
| implemented | POST | `/api/v2/websites/ssl/import` | apps/workmesh-server/node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/list` | apps/workmesh-server/node/api/ssl.go | unknown | database | missing | - |
| implemented | POST | `/api/v2/websites/ssl/obtain` | apps/workmesh-server/node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/push` | apps/workmesh-server/node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/resolve` | apps/workmesh-server/node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/search` | apps/workmesh-server/node/api/ssl.go | unknown | database | missing | - |
| implemented | POST | `/api/v2/websites/ssl/update` | apps/workmesh-server/node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/upload` | apps/workmesh-server/node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/ssl/upload/file` | apps/workmesh-server/node/api/ssl.go | unknown | memory | missing | - |
| implemented | POST | `/api/v2/websites/stream/update` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/templates/del` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/templates/get` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/templates/outputs` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/templates/outputs/del` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/templates/outputs/get` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/templates/outputs/search` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/templates/preview` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/templates/search` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/templates/update` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/templates/upload` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/update` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/waf/access-lists` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/waf/attack/stat` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/waf/block/search` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/waf/global` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/waf/log/search` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/waf/relation/stat` | apps/workmesh-server/node/api/website_extensions.go | required | memory | present | - |
| implemented | POST | `/api/v2/websites/waf/rules` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/waf/rules/delete` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/waf/sites` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/websites/waf/test` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |

## workmesh

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/workmesh/gateway/status` | apps/workmesh-server/control/api/gateway.go | required | memory | present | - |
| implemented | POST | `/api/v2/workmesh/gateway/login` | apps/workmesh-server/control/api/gateway.go | required | memory | present | - |
| implemented | POST | `/api/v2/workmesh/gateway/register` | apps/workmesh-server/control/api/gateway.go | required | memory | present | - |
| implemented | POST | `/api/v2/workmesh/gateway/unbind` | apps/workmesh-server/control/api/gateway.go | required | memory | present | - |
| implemented | POST | `/api/v2/workmesh/tasks/cancel` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/workmesh/tasks/collect` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/workmesh/tasks/create` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/workmesh/tasks/destroy` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/workmesh/tasks/exec` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/workmesh/tasks/start` | apps/workmesh-server/node/api/ai_execution.go | unknown | memory | present | - |

## xpack

| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| implemented | GET | `/api/v2/xpack/monitor/config/global` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | GET | `/api/v2/xpack/monitor/status` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | GET | `/api/v2/xpack/waf/access-lists` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | GET | `/api/v2/xpack/waf/sites` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | GET | `/api/v2/xpack/waf/sites/:id/rules` | apps/workmesh-server/node/api/website.go | unknown | unknown | present | - |
| implemented | GET | `/api/v2/xpack/waf/standard-rules` | apps/workmesh-server/node/api/website.go | unknown | unknown | present | - |
| implemented | GET | `/api/v2/xpack/waf/status` | apps/workmesh-server/node/api/website.go | unknown | unknown | present | - |
| implemented | POST | `/api/v2/xpack/monitor/config/global` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/monitor/config/site` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/monitor/config/site/update` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/monitor/logs/clear` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/monitor/logs/detail` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/monitor/logs/search` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/monitor/logs/stat` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/monitor/qps` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/monitor/rank` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/monitor/stat` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/monitor/trend` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/monitor/visitors` | apps/workmesh-server/node/api/website.go | unknown | unknown | present | - |
| implemented | POST | `/api/v2/xpack/monitor/visitors/loc` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/monitor/websites` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/waf/access-lists` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/waf/attack/stat` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/xpack/waf/block/search` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/xpack/waf/global` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/waf/log/search` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/xpack/waf/relation/stat` | apps/workmesh-server/node/api/website.go | unknown | memory | present | - |
| implemented | POST | `/api/v2/xpack/waf/rules` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/waf/rules/delete` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/waf/sites` | apps/workmesh-server/node/api/website.go | unknown | external | present | - |
| implemented | POST | `/api/v2/xpack/waf/test` | apps/workmesh-server/node/api/website.go | unknown | unknown | present | - |

## 国际化完整性批次

| 功能 | 来源 | 新实现 | 覆盖 | 状态 |
| --- | --- | --- | --- | --- |
| Core/Agent 后端语言包与前端语言入口 | `apps/workmesh-node/core/i18n`、`apps/workmesh-node/agent/i18n`、旧 frontend | `i18n/i18n.go`、`i18n/lang/*.yaml`、`web/src/lang` 与各页面入口 | 12 种语言；后端每种 1037 键；前端键结构和菜单入口通过 `i18n-scan.mjs` | implemented |

