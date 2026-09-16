<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 前端页面与同源 API 复刻

> Historical record：本文记录 2026 年 9 月 3 日的迁移阶段。当前前端接口、菜单和页面审计必须读取只读参考 `/www/apps/1Panel/frontend`，WorkMesh 工作副本仅为 `/www/apps/workmesh-server/web`。

状态：[x] 已完成（静态核对与构建门禁）

## 本次修改

- 当前前端唯一实现目录是 `/www/apps/workmesh-server/web/`；页面、动态 import 和 API 清单以该目录为准，业务行为只对照只读参考 `/www/apps/1Panel/frontend`。
- API 客户端将绝对 `VITE_API_URL` 归一为当前 origin 下的 API 路径，保持浏览器不直连节点或远程服务。
- 应用图标、文件下载、文件分享下载/二维码和文件预览统一通过同源 URL 构造器，保留节点与业务查询参数。

## 验证证据

- `npx --yes --package node@20 --call 'node --version && npm run type-check'`：通过，Node `v20.20.2`。
- `npx --yes --package node@20 --call 'npm run build:pro'`：通过，Vite 生产构建完成。
- 原版/当前视图文件交叉扫描：缺失 `0`，额外 `0`。
- 原版/当前动态 import 扫描：两侧各 `98` 个，全部可解析。

## 未完成与边界

- 页面依赖的后端真实能力、Docker/OpenResty/ACME 和部署验收由后端及测试部署任务继续验证；前端不以静态构建代替真实回路。
