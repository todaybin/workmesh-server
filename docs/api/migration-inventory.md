<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Core/Agent 全量迁移清单

本清单是迁移验收的索引，不允许用它删减旧功能。执行阶段需由脚本从旧项目的 router、controller、service、model、定时任务、CLI、WebSocket/SSE 和前端 API 生成机器可读快照，并将差异附在本目录。

## 控制面（Core）

`/api/v2/core/*` 下的认证、Session、MFA、Passkey、RBAC、用户组、权限、菜单、基础设置、域名、SSL、升级、应用商店、备忘录、操作日志、控制面备份、脚本库、命令历史、节点多机管理、Gateway 登录授权和同步接口。

## 执行面（Agent）

系统信息、进程和服务管理；系统命令与终端；网络、防火墙、Fail2ban、SSH 和诊断；Docker 容器、镜像、网络、卷和 Compose；应用安装、升级、回滚和备份；网站、OpenResty、PHP、SSL、ACME、CA、DNS、模板和 WAF；MySQL、PostgreSQL、MongoDB、Redis 等数据库；文件浏览、权限、上传下载、分片、编辑、压缩、解压和分享；计划任务及日志；备份账号、备份和恢复；告警和通知；AI、Ollama、MCP、GPU、TensorRT；CubeSandbox；在线开发、Worktree、运行时；部署清单、制品和异步任务。

## 协议与资源

必须保留所有 HTTP 方法、参数校验、响应 envelope、错误码、分页上限、WebSocket、SSE、流式输出和文件传输行为。新增 Gateway 与主次节点接口不得覆盖或改变旧 API。

## 完成条件

新旧快照差异只能包含已批准的新接口；每个差异必须有迁移提交、契约测试和中文 API 文档链接。
