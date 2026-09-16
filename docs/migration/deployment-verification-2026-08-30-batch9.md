<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-08-30 迁移批次九部署验收

> Historical record：本文记录历史部署批次。旧 `apps/workmesh-node` 命令仅保留作当时的执行证据，不得直接复用；当前路由、实现和隐藏能力扫描统一使用只读参考 `/www/apps/1Panel`。

## 本地制品

- 目标平台：Linux amd64
- 构建命令：`GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o .tmp/workmesh-server-linux-amd64 ./cmd/workmesh-server`
- SHA256：`CEDCC6C96CCE4852D5AF4178D10B169630A1EE2A08722E77CD40B23E8CBAA7B9`
- 本批提交：`1f85702`、`edcc573`、`9fa8a3b`、`98852c7`、`4a24f8f`、`b0ac514`、`364dde8`

## 公网健康检查

| 节点 | `/health` | `/ready` | 结果 |
| --- | --- | --- | --- |
| 主节点 `61.184.12.165:9999` | HTTP 200 | HTTP 200 | 通过 |
| 次节点 `162.14.96.198:9999` | HTTP 200 | HTTP 200 | 通过 |

## 制品替换状态

当前执行环境未提供可用的远程 SSH 授权材料，未执行二进制上传或 systemd 重启；上述健康检查验证的是线上现有服务，不代表其已运行本批 SHA256 制品。获得授权后应通过受控部署脚本原子替换，保留旧制品并重新执行双节点健康、路由和实现扫描。

## 本批功能

- AI 账户、模型发现、GPU 探测和 CubeSandbox 状态持久化。
- 文件收藏、回收站和上传记录持久化。
- 备份账号、记录、上传、恢复及 token 状态。
- 仪表盘网络、挂载点和硬件能力采集。
- 网站 xpack 监控/WAF 别名真实转发，移除通配兼容占位。

## 验证命令

```powershell
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go test ./..."
node scripts/with-dev-env.mjs -- powershell -NoProfile -Command "`$env:GOWORK='off'; Set-Location apps/workmesh-server; go vet ./..."
node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/route-scan.mjs check --legacy apps/workmesh-node --project apps/workmesh-server --manifest apps/workmesh-server/test/contract/routes.json
node scripts/with-dev-env.mjs -- node apps/workmesh-server/test/contract/hidden-function-scan.mjs --legacy apps/workmesh-node --project apps/workmesh-server
```
