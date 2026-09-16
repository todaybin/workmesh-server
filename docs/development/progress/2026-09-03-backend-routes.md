<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-09-03 后端 v2 路由补齐

## 本次完成

- [x] `POST /api/v2/core/script/sync` 使用 `WORKMESH_SCRIPT_REPO_URL` 读取有界远端 JSON，校验脚本字段并写入 SQLite；未配置远端时返回 503，远端或存储失败不伪造成功。
- [x] `POST /api/v2/apps/installed/sync` 逐安装记录探测真实 Docker 容器状态，部分失败返回 502 及失败项。
- [x] `POST /api/v2/custom/app/sync` 要求受控部署通过 `WORKMESH_CUSTOM_APP_ARCHIVE`/`WORKMESH_CUSTOM_APP_PACKAGE` 提供真实归档，执行大小校验、临时文件和原子替换；未配置返回 503。
- [x] `POST /api/v2/dashboard/system/restart/{operation}` 增加操作白名单、systemd 存在性和显式授权开关，使用参数化 `systemctl` 命令异步执行；无 systemd、服务未运行或未授权均返回明确错误。
- [x] 核心 groups/script 专用路由过滤 legacy fallback，未接入资源继续明确返回 501。
- [x] AI 执行状态接入公共 SQLite `ai_state`；旧 `ai.json` 首次导入后归档，重启从 SQLite 恢复。
- [x] 主机监控设置接入公共 SQLite `node_settings(host_operational)`；旧 `host-operational.json` 首次导入后归档。
- [x] 数据库管理旧 `database-admin.json` 导入成功后归档，后续请求只使用 SQLite 表。
- [x] 计划任务和分组路由从 legacy fallback 过滤器中移除，避免已实现 handler 被兼容占位注册覆盖。
- [x] 网站创建始终把主域名写入 `website_domains`，HTTP/HTTPS 默认端口分别为 80/443；历史空域名网站在启动时幂等回填。
- [x] `GET /api/v2/websites/domains/{websiteId}` 恢复原版数组响应，匹配前端 `res.data` 表格契约。
- [x] 安全入口设置在 `domains.json` 迁移后改从 SQLite `functional_domain_state` 注入全局中间件，避免重启后入口策略丢失。

## 验证

```text
GOWORK=off go test ./node/api -run 'TestScriptSyncUsesConfiguredRemoteAndSQLite|TestInstalledSyncAndCustomStoreRequireRealSources|TestDashboardRestartRequiresExplicitAuthorization' -count=1 -v
```

结果：3 个测试通过。完整 `server/node/api` 测试仍受其他领域测试环境约束，详见主状态页。

## 未完成/阻塞

- 脚本远端数据格式目前支持 JSON 数组或 `{scripts:[...]}`；原版 YAML+归档下载协议需在部署配置提供兼容转换源后再联调。
- 系统重启属于宿主机高风险副作用，默认关闭 `WORKMESH_ALLOW_RESTART`/`WORKMESH_ALLOW_SYSTEM_REBOOT`，正式验收需人工开启并记录 systemd 结果。
- AI、主机监控和数据库管理新增迁移回归测试已通过；外部 Gateway、ACME、SSH/终端和 KVM 依赖仍需真实环境验收。
