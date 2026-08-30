<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-08-30 运行验收批次十

## 本地服务

- 启动方式：`WORKMESH_SERVER_ADDR=127.0.0.1:9999 WORKMESH_DATA_DIR=.tmp/local-run go run ./cmd/workmesh-server`
- `/health`：HTTP 200，返回 `code=200/data.status=ok`。
- `/ready`：HTTP 200，返回 `code=200/data.status=ready`。
- `/`：HTTP 200，`Content-Type: text/html; charset=utf-8`。
- `/assets/js/index-C-bqGlVd.js`：HTTP 200，`Content-Type: text/javascript; charset=utf-8`，确认模块脚本不再收到 JSON MIME。

## 登录与部署节点

- `POST /api/v2/core/auth/login` 使用本地测试管理员会话成功，返回 HttpOnly `workmesh_session` Cookie。
- 带会话访问 `GET /api/v2/core/auth/current` 返回当前用户对象；无会话请求返回 HTTP 401。
- `POST /api/v2/core/nodes/add` 新增 `secondary` 节点成功，节点地址通过 HTTP(S) URL 校验并写入 `nodes.json`。
- `GET /api/v2/core/nodes/list` 返回本机主节点和新增次节点；使用 `nodeId` 删除测试节点成功。
- 非法地址、重复节点和删除当前节点均返回明确错误，未产生持久化副作用。

## 公网节点健康

| 节点 | `/health` | `/ready` | 结论 |
| --- | --- | --- | --- |
| `61.184.12.165:9999` | HTTP 200 | HTTP 200 | 现有线上服务健康；未证明已替换最新制品 |
| `162.14.96.198:9999` | HTTP 200 | HTTP 200 | 现有线上服务健康；未证明已替换最新制品 |

## 受控部署预检

通过 `scripts/remote-staging-preflight.ps1 -SkipUpload` 执行，SSH 可达但返回阻断：

- 缺少 `/dev/kvm`，无法验收 CubeSandbox/MicroVM。
- 主机内存约 4 GiB，低于部署脚本要求的 8 GiB。

因此本批次未上传或替换远程二进制，未执行 systemd 重启。不能将公网健康结果解释为最新源码已部署。

## 验证命令

```powershell
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./..."
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go vet ./..."
node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/route-scan.mjs check --legacy apps/workmesh-node --project apps/workmesh-server --manifest apps/workmesh-server/test/contract/routes.json
node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/hidden-function-scan.mjs --legacy apps/workmesh-node --project apps/workmesh-server
```

结果：以上四项均通过；实现扫描当前为 `implemented 717`、`partial 76`、`pending 78`。尚未达到“所有功能完成”门槛。
