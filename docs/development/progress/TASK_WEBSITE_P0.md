<!-- SPDX-License-Identifier: GPL-3.0-only -->
# WEB-P0 当前任务

只修改后端网站/OpenResty相关代码和定向测试，不修改 apps/1Panel、前端或生产资源。

目标：检查并补齐静态站点、域名、HTTPS、OpenResty 配置处理器的真实 SQLite/配置文件行为；优先修复明显固定成功、空实现或错误 envelope。每次只改互不冲突的小范围文件，新增方法写中文注释，单文件不要继续膨胀。

验收：仅运行 `GOWORK=off go test ./node/api -run 'Test(Website|SSL)' -count=1` 及相关服务定向测试；记录结果到本文件末尾。禁止生产 Docker、ACME 签发、reload、数据库写入。
