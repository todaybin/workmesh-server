<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# AI 与应用接口迁移

本页记录 Agent 执行面中 AI 和应用域的实现边界。接口统一返回 `{ code: 200, data: ... }`；错误返回 `code: "ERR"` 和 `details.errCode`。状态文件位于 `WORKMESH_DATA_DIR/ai.json`、`WORKMESH_DATA_DIR/apps.json`，采用临时文件替换，服务重启后可恢复。

## AI

| 路由 | 方法 | 行为 | 状态来源 |
| --- | --- | --- | --- |
| `/api/v2/ai/ollama/model` | POST | 按名称创建或更新 Ollama 模型记录 | `ai.json` 的 `ollama` |
| `/api/v2/ai/ollama/model/recreate` | POST | 重新登记模型并置为运行状态 | `ai.json` 的 `ollama` |
| `/api/v2/ai/ollama/model/search` | POST | 支持 `name/info` 过滤，返回分页 envelope | `ai.json` |
| `/api/v2/ai/ollama/model/load` | POST | 查询模型详情 | `ai.json` |
| `/api/v2/ai/ollama/model/sync` | POST | 返回本机已登记模型列表 | `ai.json` |
| `/api/v2/ai/ollama/model/del`、`/close` | POST | 按 `id/ids/name` 删除或停止模型 | `ai.json` |
| `/api/v2/ai/domain/*`、`/mcp/domain/*` | GET/POST | 绑定、更新和读取域名配置 | `ai.json` 的 `domains` |
| `/api/v2/ai/gpu/load`、`/gpu/options`、`/gpu/search` | GET/POST | 返回 CPU/GPU/NPU/XPU 能力和监控结构；无驱动时明确 `available=false` | 运行时探测 |
| `/api/v2/ai/mcp/search`、`server`、`server/detail`、`server/update`、`server/del`、`server/op` | POST | MCP 服务 CRUD 与状态操作 | `ai.json` 的 `mcp` |
| `/api/v2/ai/mcp/server/status/sync`、`connection/test` | POST | 返回登记服务状态；连接测试只校验 endpoint/protocol，不伪造外部连接 | 本地状态/参数校验 |
| `/api/v2/ai/tensorrt/*` | POST | TensorRT-LLM CRUD、搜索和启停操作 | `ai.json` 的 `tensorrt` |
| `/api/v2/ai/accounts/*` | GET/POST | Provider 列表、账号 CRUD、分页、计数、模型维护、发现和验证 | `ai.json` 的 `accounts` |
| `/api/v2/ai/agents/*` | POST | Agent 登记、批量操作、搜索、删除、Token 重置、备注、网站绑定、模型配置和概览 | `ai.json` 的 `agents/configs` |
| `/api/v2/ai/agents/hermes/chat/sessions*` | POST | 会话查询、重命名和删除 | `ai.json` 的 `sessions` |
| `/api/v2/ai/agents/agent/*` | POST | 角色创建/删除/绑定、渠道查询、Markdown 文件更新 | `ai.json` 的 `configs` |
| `/api/v2/ai/agents/channel/*` | POST | 飞书、Telegram、Discord、企业微信、钉钉、微信和 QQ Bot 配置；敏感字段脱敏 | `ai.json` 的 `configs` |
| `/api/v2/ai/agents/plugin*`、`plugins/*` | POST | 插件安装、升级、卸载、检查、搜索和操作登记 | `ai.json` 的 `plugins` |
| `/api/v2/ai/agents/skills/*` | POST | Skill 搜索、安装、更新和卸载登记 | `ai.json` 的 `skills` |

账号和渠道响应中的 `apiKey`、`token`、`password`、`secret`、`botToken`、`appSecret` 统一返回 `******`；原值仅用于本地状态保存，禁止写入日志。

## 应用目录与安装

| 路由 | 方法 | 行为 | 状态来源 |
| --- | --- | --- | --- |
| `/api/v2/apps/search` | POST | 按 `name/key/tags` 分页搜索应用目录；目录为空时懒加载原 1Panel 应用商店清单 | `apps.json` 的 `catalog` |
| `/api/v2/apps/sync/local`、`sync/remote` | POST | 重新加载本地目录或登记同步任务，返回统一列表 | `apps.json` |
| `/api/v2/apps/:key`、`detail/*`、`details/*` | GET | 返回应用详情、版本和参数结构；找不到时 `available=false` | catalog/installed |
| `/api/v2/apps/tags`、`checkupdate`、`services/:key` | GET | 返回目录标签、更新状态和服务状态 | catalog/installed |
| `/api/v2/apps/install` | POST | 按应用 ID/key 幂等安装并保存配置 | `apps.json` 的 `apps` |
| `/api/v2/apps/installed/list`、`installed/search` | GET/POST | 已安装应用列表和分页搜索 | `apps.json` |
| `/api/v2/apps/installed/info/*`、`params/*`、`delete/check/*` | GET | 读取安装信息、参数和删除依赖检查 | `apps.json` |
| `/api/v2/apps/installed/op` | POST | 启动、停止、重启、卸载（删除）安装记录 | `apps.json` |
| `/api/v2/apps/installed/port/change`、`params/update`、`config/update` | POST | 更新端口、安装参数及 Web UI 配置 | `apps.json` |
| `/api/v2/apps/installed/loadport`、`conninfo`、`conf` | POST | 返回端口、连接信息和默认 Compose 参数 | `apps.json` |
| `/api/v2/apps/installed/sort/update`、`update/versions`、`sync` | POST | 更新排序、查询可升级版本和重新同步状态 | `apps.json` |
| `/api/v2/apps/installed/ignore`、`/api/v2/apps/ignored/detail`、`ignored/cancel` | GET/POST | 忽略、查看和取消应用升级忽略项 | `apps.json` 的 `ignored` |
| `/api/v2/apps/icon/:key` | GET | 返回真实 `image/png`，无图标时使用透明 1x1 PNG | 静态响应 |
| `/api/v2/custom/app/config`、`custom/app/sync` | GET/POST | 自定义商店配置和同步任务 | `apps.json` |
| `/api/v2/core/xpack/sync/app/install` | POST | 多节点安装兼容入口，复用安装幂等逻辑 | `apps.json` |

安装写入遵循“同一 ID/key 更新、不同应用追加”的幂等规则；响应保留旧前端字段 `appKey`、`appName`、`appStatus`、`ready`、`total`、`canUpdate` 等，避免页面转换层丢字段。

未设置 `WORKMESH_APP_CATALOG` 时，Server 使用原应用商店仓库获取清单：默认地址为 `https://apps-assets.fit2cloud.com/{mode}/1panel.json.zip`，其中 `{mode}` 默认为 `stable`。可使用 `WORKMESH_APP_REPO_URL`、`WORKMESH_APP_REPO_MODE` 和 `WORKMESH_APP_REPO_EDITION=intl` 覆盖仓库配置。清单成功后缓存到 `WORKMESH_DATA_DIR/apps.json`；远程暂时不可用时继续使用上次缓存。

## 验收

```powershell
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./node/api -run 'AI|App'"
node apps/workmesh-server/test/contract/implementation-scan.mjs --legacy apps/workmesh-node --project apps/workmesh-server --out apps/workmesh-server/.tmp/implementation-status.json
```

验收必须检查：创建/更新后重新建立路由实例仍能读取状态；敏感字段不回显；应用图标的 Content-Type 为 `image/png`；无 GPU/KVM 主机返回明确降级状态而非 501。
