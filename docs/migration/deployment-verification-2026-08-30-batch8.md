<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 双节点迁移验收记录（批次 8）

日期：2026-08-30  
制品：`workmesh-server` Linux amd64  
制品 SHA256：`00e07f7dbb4ed995446cee19d5d97ad6deeec268852cdfcaf10dad575367fc65`

## 本批变更

- `GET /api/v2/core/script/run` 增加 `WORKMESH_COMMAND_TOKEN` 鉴权、命令长度限制、30 秒超时、退出码和输出字段。
- `GET /api/v2/process/ws` 增加 RFC6455 16 位和 64 位 payload 长度编码，避免进程快照超过 125 字节时断流。
- 网站监控/WAF 统计接口使用统一 DTO 结构；监控全局和站点配置写入 `domains.json`。
- 隐藏功能清单与 870 条逐路由清单同步更新。

## 自动化验证

| 检查 | 结果 |
| --- | --- |
| `go test ./...` | 通过 |
| `go vet ./...` | 通过 |
| route-scan | 870/870，通过；扩展路由 62 条允许兼容 |
| hidden-function-scan | 95 个源码文件，init/middleware/i18n/log/cron 五类清单存在 |
| implementation-scan | implemented 476，partial 225，compatibility 169，pending 0，missing 0 |
| 双节点制品校验 | 主、次节点 SHA256 一致 |
| 双节点健康检查 | `/health`、`/ready` 均 HTTP 200 |
| 前端 MIME | 首页 `text/html`，入口 JS `text/javascript` |
| 节点管理 | 登录、添加、列表、删除通过，测试节点已清理 |
| AI Agent | 创建、列表、概览、删除通过，测试 Agent 已清理 |
| 隐藏入口 | xpack monitor、xpack WAF、swagger 可访问；`/public/favicon.ico` 无资源时返回 404 |

## 未完成项声明

`partial` 和 `compatibility` 不等同于完整复刻。真实访问日志采集、云备份 token 刷新、完整双向终端/SSE、CSRF/域名绑定/密码过期中间件、PHP/Node 运行时详情、文件回收站高级行为等仍需按逐路由清单逐项验收后才能发布为“完整功能”。

