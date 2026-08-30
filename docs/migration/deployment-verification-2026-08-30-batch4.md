<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-08-30 双节点部署验收

## 部署结果

后端制品 `0AB60D16FCC7DCDC0588D738488AA774CA5BEB3F65B0C7EA6D08B9CFF777FE88` 已部署到：

| 节点 | 目录 | 服务 | 结果 |
| --- | --- | --- | --- |
| 主节点 `61.184.12.165:52834` | `/opt/workmesh-server` | `workmesh-server.service` | `active` |
| 次节点 `162.14.96.198:22` | `/opt/workmesh-server-secondary` | `workmesh-server-secondary.service` | `active` |

两台节点均保留替换前二进制备份和前端目录备份，未覆盖配置、数据或上传资源。

## 前后端验收

- 主节点 `/`：HTTP 200，`text/html`。
- 主节点 `/assets/js/index-BvHLZz3m.js`：HTTP 200，`text/javascript`。
- 主节点 `/assets/css/style-BqLbKwD-.css`：HTTP 200，`text/css`。
- 主次节点 `/health`、`/ready`：HTTP 200，JSON `code=200`。
- Vite 生产构建：7656 个模块，构建成功。
- 次节点公网 `:9999` 可在 SSH 本机访问，但主节点到 `162.14.96.198:9999` 连接超时；该端口仍需在云安全组放行后才能验收跨机心跳/同步。
- `/api/v2/workmesh/gateway/status` 当前返回 `registration=pending`；远端 `server.env` 仅配置 Gateway URL，缺少 Gateway ID/Secret 或账号凭据，主动注册返回 HTTP 502，网关授权尚未完成。

## 节点管理验收

主节点使用 `admin/admin` 登录后完成真实接口链路：

1. `POST /api/v2/core/nodes/add` 添加测试次节点。
2. `POST /api/v2/core/nodes/list` 查询并确认节点存在。
3. `POST /api/v2/core/nodes/del` 删除测试节点并返回 `code=200`。

## 自动化验证

```text
go test ./...       通过
go vet ./...        通过
route-scan          831/831 通过
implementation-scan pending=0, missing=0
```

`partial` 和 `compatibility` 接口仍按逐路由清单标注，不能仅凭 HTTP 200 视为完成真实副作用迁移。
