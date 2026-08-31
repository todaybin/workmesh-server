<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-08-31 线上部署与验收

## 发布制品

| 制品 | SHA256 | 说明 |
| --- | --- | --- |
| Linux amd64 后端 | `c7d4a63cc4ea52bbac3a137d287957ed3d547c97941e344002087d8d94ee2ea7` | 当前提交构建，`-trimpath -ldflags "-s -w"` |
| `web/dist` 前端归档 | `2c955f506c01794989dc5c54c311025c03a2a5282e884fcdd8864ad0f84af4a4` | 当前前端源码构建，原子替换 |

## 部署目标

| 节点 | SSH | WorkMesh 目录 | systemd 单元 | 结果 |
| --- | --- | --- | --- | --- |
| 主节点 | `root@61.184.12.165:52834` | `/opt/workmesh-server` | `workmesh-server.service` | 已替换并 active |
| 次节点 | `root@162.14.96.198:22` | `/opt/workmesh-server-secondary` | `workmesh-server-secondary.service` | 已替换并 active |

次节点同机的 `workmesh-gateway.service`（`/www/wwwroot/work.zoomtk.com`）未修改。两台节点均在自身 `backups/release-*` 和 `backups/web-*` 下保存替换前制品，配置目录和数据目录未覆盖。

## 自动验收

| 检查 | 主节点 | 次节点 |
| --- | --- | --- |
| `/health` | HTTP 200 | HTTP 200 |
| `/ready` | HTTP 200 | HTTP 200 |
| `/` | HTTP 200，`text/html` | HTTP 200，`text/html` |
| 入口 JavaScript | HTTP 200，`text/javascript` | HTTP 200，`text/javascript` |
| 未授权 API | 401 | 401 |
| `admin/admin` 登录 | 200，会话 Cookie 有效 | 200，会话 Cookie 有效 |
| `/core/auth/current` | 200 | 200 |
| `/workmesh/gateway/status` | 200 | 200 |
| `/core/nodes/list` | 200 | 200 |
| `/dashboard/base/os` | 200 | 200 |
| `/dashboard/current/node` | 200 | 200 |
| `/containers/docker/status` | 200 | 200 |
| `/websites/list` | 200，当前为空 | 200，当前为空 |
| `/hosts/tool/status`（Bearer） | 200 | 200 |
| `/websites/search`（Bearer） | 200，未发现 `znmp.sopvip.com` | 200，未发现 `znmp.sopvip.com` |

公网从服务器侧执行 `curl -k`：

- `https://znmp.sopvip.com/`：HTTP 200，`text/html`。
- `https://work.zoomtk.com/`：HTTP 200，`text/html`。
- 明文 HTTP 访问 `znmp.sopvip.com` 会按站点配置 301 跳转 HTTPS。

## 授权与遗留风险

- 主节点返回 `configured=true`、`registration=registered`，但 Gateway 状态字段为 `error`，需要使用真实云端接口继续核对心跳和任务透传。
- 次节点返回 `configured=false`、`registration=pending`。未提供有效 Gateway 凭据，未伪造注册成功，也未写入或复制主节点密钥。
- 两节点本地网站列表为空，`znmp.sopvip.com` 尚未出现在 WorkMesh 节点站点数据中。该域名由独立普通应用站点提供，需确认站点数据导入/节点归属后再执行业务迁移，不能在本次发布中猜测写入。
- 本机 PowerShell Schannel 无法完成公网 TLS 握手；服务器侧 `curl -k` 已验证 HTTPS 返回 200，证书链和浏览器兼容性仍需在真实客户端复核。
- 本次未执行数据库迁移、Gateway 配置变更、容器破坏性操作或生产数据写入。

## 验证命令

部署和冒烟命令均通过 `node scripts/with-dev-env.mjs -- ...` 包装器执行。远端替换脚本在每台节点执行摘要校验、备份、systemd 重启和本机健康检查；失败路径会恢复二进制、前端目录并重新启动原单元。

## Gateway 绑定修复复验

- 后端修复制品 SHA256：`0a8da4dff0f3fe76bf3194dea278bf8102eb7b8ef9a0827b919c25871c31fc1e`。
- 修复第三方 Gateway 节点注册响应使用 `data.item.id` 时无法识别绑定标识的问题；该字段作为稳定节点绑定 ID 保存。
- 主节点已有绑定保持 `configured=true / registration=registered`。
- 次节点使用本地 `admin/admin` 会话和真实 Gateway 账号完成一次绑定：HTTP 200、`bound=true`、`nodeId=secondary-gateway-162`。
- 重启次节点后再次查询仍为 `configured=true / registration=registered`，确认绑定快照持久化；未复制主节点身份或密钥。
- 前端路由守卫、绑定页和设置页现会识别本地会话失效并跳转本地登录页，不再显示误导性的 Gateway 绑定错误。
