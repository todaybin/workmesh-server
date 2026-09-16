<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# HTTP/WS 真实测试矩阵（2026-09-05）

状态：`[>]` 进行中。本文只记录已经实际发出请求或实际执行命令的结果；源码清单中的 `not-run` 不得改写为通过。

## 测试环境

| 项目 | 实际值 |
| --- | --- |
| 服务 | `workmesh-server.service`，`active/running` |
| 本地地址 | `http://127.0.0.1:9999` |
| 网站入口 | `Host: znmp.sopvip.com`，本机 OpenResty 80 端口 |
| 生产二进制 SHA-256 | `a3fc4d11476e23c7a55c29fdf13ad7975e06706f990f8bf8d841d620ee5f6222` |
| 部署备份 | `/opt/workmesh-server/backups/deploy-20260905T210300+0800-files-routes-menu-inventory` |
| 数据库 | `/opt/workmesh-server/data/workmesh.db`（SQLite，真实运行库） |
| WAF | `workmesh-openresty-waf`，`running/healthy` |

生产 SQLite 快照检查（2026-09-05）显示：已应用 12 条迁移（最新 `0012-runtime-task-state`），`websites=1`、`runtime_records=6`、`runtime_tasks=31`、`runtime_task_logs=18145`、`operation_logs=1995`、`login_logs=153`。这些是数据库中的真实记录计数，只证明数据源和迁移可读，不替代各页面的业务验收。

## 已执行的 HTTP 测试

请求体、密码和 Cookie 不在文档中保存；响应仅保留状态码和脱敏摘要。

| 状态 | 方法 | 路径 | 预期 | 实际 | 副作用/备注 |
| --- | --- | --- | ---: | ---: | --- |
| pass | GET | `/health` | 200 | 200 | `{"code":200,"data":{"status":"ok"}}` |
| pass | GET | `/ready` | 200 | 200 | `{"code":200,"data":{"status":"ready"}}` |
| pass | GET | `/api/v2/health` | 200 | 200 | v2 健康接口返回真实服务状态 |
| pass | GET | `/api/v2/core/auth/setting` | 200 | 200 | 返回 `mfa:false、passkey:false`；设置了清理公钥 Cookie |
| pass | GET | `/api/v2/core/auth/current` | 401 | 401 | 无会话被拒绝，`errCode=LOCAL_AUTH_REQUIRED` |
| pass | GET | `/api/v2/core/dashboard` | 401 | 401 | 无会话被拒绝，未返回模拟业务数据 |
| pass | POST | `/api/v2/core/auth/login` | 401 | 401 | 使用不存在的探针账号和错误密码；未修改现有管理员密码 |
| pass | GET | `/`（无安全入口） | 404 | 404 | 安全入口保护生效 |
| pass | GET | `/<security-entrance>` | 200 | 200 | 实际入口来自 SQLite；设置 `SecurityEntrance` Cookie 并返回前端 HTML |
| pass | GET | `/login`（带安全入口 Cookie） | 200 | 200 | 返回前端登录页 |

最新一批请求在当前生产配置未向测试 shell 暴露安全入口值的情况下执行：`node test/contract/http-smoke.mjs --base http://127.0.0.1:9999 --out docs/inventory/http-smoke-20260906.json`，结果为 `8/8 pass`，证据文件为 [`../../inventory/http-smoke-20260906.json`](../../inventory/http-smoke-20260906.json)。此前带安全入口参数的结果仅作为历史记录，不能当作当前结果。脚本只记录真实响应摘要，不修改 759 条路由和 389 条前端调用的 `not-run` 状态。

## 已执行的 WS 握手边界测试

在未携带登录会话或 `X-WorkMesh-Token` 的情况下，使用 HTTP/1.1 WebSocket Upgrade 请求验证四条前端实际使用的 WS 路由均拒绝未授权连接；这证明了认证边界，不等价于已完成登录后的 PTY、消息序列或断线释放验收。

| 状态 | 路径 | 实际状态 | 响应摘要 |
| --- | --- | ---: | --- |
| pass | `/api/v2/core/script/run` | 401 | `LOCAL_AUTH_REQUIRED` |
| pass | `/api/v2/hosts/terminal/local` | 401 | `STREAM_AUTH_REQUIRED` |
| pass | `/api/v2/hosts/terminal/container` | 401 | `STREAM_AUTH_REQUIRED` |
| pass | `/api/v2/hosts/terminal/ssh` | 401 | `STREAM_AUTH_REQUIRED` |

本批测试没有建立成功的 WebSocket 会话，也没有发送业务消息；登录后的 WS 测试仍保持 `not-run`。

前端合约执行器在 `WORKMESH_SCOPE=boundary` 下于 2026-09-05 23:19（北京时间）再次实际运行，7 条公开/未授权 HTTP 与 WS 边界全部通过；最新独立证据为 `docs/inventory/frontend-contract-evidence-20260905T151944737Z.json`。该证据只证明认证边界，不改变 389 条业务接口的 `not-run` 状态。

## 已执行的 OpenResty/站点测试

| 状态 | 命令/请求 | 实际结果 |
| --- | --- | --- |
| pass | `docker exec workmesh-openresty-waf nginx -t` | syntax is ok，test is successful |
| pass | `curl -H 'Host: znmp.sopvip.com' http://127.0.0.1/` | HTTP 200，返回站点 HTML |
| pass | WAF 容器健康检查 | `running/healthy` |
| pass | 无效 `proxy_pass http://;` 残留检查 | 已隔离旧配置并恢复 nginx 配置测试 |
| pass | stream 配置语法 | 生成 `listen 29001; proxy_pass 127.0.0.1:9;`，不再嵌套 `stream {}` |

## 尚未执行的真实测试

以下项目必须取得有效登录账号、节点凭据或明确的 Docker/证书测试资源后执行；不能用固定成功、空数组或 JSON 文件代替：

- 登录成功后的 Session/Cookie、Bearer/API Key 和 CSRF 闭环。
- 概览、应用商店、网站、AI、数据库、容器、系统、终端、计划任务、工具箱、高级功能、日志审计、面板设置全部页面和弹窗操作。
- 1Panel 基线 759 条路由的真实请求、参数、响应字段、错误分支和 SQLite 副作用。
- 前端扫描出的 385 条 HTTP 调用和 4 条 WS 调用的真实调用；包括同一路由 `type`、`operate`、`logType`、`source`、`scope` 变体。
- 终端本地/容器/SSH PTY、进程 WS、SSE、上传下载和流式响应的消息序列及断线释放。
- Go、Node、Python、Java、.NET、PHP 运行环境创建、详情、停止、启动、重启、日志、任务、更新、删除和同名重建；PHP FPM、扩展和 Supervisor 单独验收。
- 一键部署、运行环境、静态、反向代理、子网站、TCP/UDP 六类网站和全部网站设置。
- Let's Encrypt HTTP-01 具体子域名签发、安装、续期；HTTP-01 不支持 `*.cs.sopvip.com` 通配符证书，必须为具体前置子域名申请。
- 主站点/次站点角色、Gateway 注册心跳、跨节点任务转发和主次切换。

## 清单来源与状态规则

- 1Panel HTTP 路由：[`../../inventory/route-inventory-1panel.json`](../../inventory/route-inventory-1panel.json)，759 条，初始均为 `not-run`。
- 前端 HTTP/WS 调用：[`../../inventory/frontend-api-inventory.json`](../../inventory/frontend-api-inventory.json)，389 条，初始均为 `not-run`。
- 质量报告：[`../../inventory/quality-report.json`](../../inventory/quality-report.json)，当前仍为 `fail`。
- 只有真实请求、脱敏响应、状态码、持久化或外部副作用和失败分支证据同时具备时，才允许把条目标记为 `pass`；无法访问凭据或外部资源标记 `blocked`，不标记 `skipped` 逃避验收。
