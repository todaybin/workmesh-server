// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

# Core 认证、会话、分组与设置

本批将旧 Core 的认证与基础控制面接口替换为本机可运行实现：

- 认证：`/api/v2/core/auth/login`、`logout`、`current`、`captcha`、`welcome`、`setting`。
- 会话：登录成功后通过 HttpOnly `workmesh_session` Cookie 保存 24 小时会话；密码仅保存 SHA-256 摘要。
- 分组：`/api/v2/core/groups/del`、`search`、`update` 支持内存创建、查询、更新和删除，后续接入 Store 持久化。
- 设置：`/api/v2/core/settings/memo`、`search`、`search/base`、`menu/update`、`update` 支持快照查询和 JSON 键值更新。

默认管理员为 `admin/admin`，生产环境必须通过 `WORKMESH_ADMIN_NAME` 与 `WORKMESH_ADMIN_PASSWORD` 配置覆盖，并在持久化迁移完成后禁止使用默认凭据。密码、Session 和 API 密钥不得写入日志。

未迁移的 MFA、Passkey、外部 SSO 和安全入口路由仍保留明确的 `MIGRATION_PENDING` 响应，不能将占位响应视为功能完成。
