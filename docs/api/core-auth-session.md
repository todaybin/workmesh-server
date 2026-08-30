// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

# Core 认证、会话、分组与设置

本批将旧 Core 的认证、基础控制面和节点管理接口替换为本机可运行实现：

- 认证：`/api/v2/core/auth/login`、`logout`、`current`、`captcha`、`welcome`、`setting`；登录响应兼容旧前端的扁平 `name/role/token` 字段。
- 会话：登录成功后通过 HttpOnly `workmesh_session` Cookie 保存 24 小时会话；同时支持 `Authorization: Bearer`、`X-WorkMesh-Token` 和 `X-API-Key`；密码仅保存 SHA-256 摘要。
- 用户与 API 凭证：`/api/v2/core/auth/current/update` 支持修改名称/密码，`auth/api/generate` 生成 API Key，`auth/api/update` 更新 API 开关、白名单和有效期。
- 可选身份源：LDAP、OIDC、SAML2 状态接口返回 `enabled: false`；开始/完成接口在未启用时返回明确错误，不伪装为登录成功。
- MFA 与 Passkey：接口保持契约可调用，默认返回 disabled/空凭证；接入真实提供商前不得宣称已启用多因素认证。
- 分组：`/api/v2/core/groups/del`、`search`、`update` 支持内存创建、查询、更新和删除，后续接入 Store 持久化。
- 设置：`/api/v2/core/settings/memo`、`search`、`search/base`、`terminal/search`、`menu/update`、`proxy/update`、`bind/update`、`port/update`、`ssl/*`、`apps/store/*`、`upgrade/*`、`update` 返回完整前端字段；状态持久化到 `WORKMESH_DATA_DIR/domains.json`，`SettingUpdate` 的 PascalCase 键会转换为 lowerCamelCase。
- 节点注册：`core/nodes/list`、`simple/all`、`add`、`update`、`del`、`delete` 以及 `core/xpack/nodes/*` 兼容别名支持节点增删改查、收藏和关键字筛选；节点状态持久化到 `WORKMESH_DATA_DIR/nodes.json`，当前节点不可删除。

默认管理员为 `admin/admin`。生产环境必须在部署前替换默认凭据；密码、Session 和 API 密钥不得写入日志。

验收命令：`node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./..."`。测试覆盖认证会话/API Key、设置键值更新、节点注册/收藏/删除。
