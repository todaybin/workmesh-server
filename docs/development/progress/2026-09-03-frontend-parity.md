<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 前端页面与同源 API 复刻

状态：[x] 已完成（静态核对与构建门禁）

## 本次修改

- 对照 `apps/workmesh-node/frontend/src` 与 `web/src` 的 routers、views、api 文件清单，页面和动态 import 无缺失。
- API 客户端将绝对 `VITE_API_URL` 归一为当前 origin 下的 API 路径，保持浏览器不直连节点或远程服务。
- 应用图标、文件下载、文件分享下载/二维码和文件预览统一通过同源 URL 构造器，保留节点与业务查询参数。

## 验证证据

- `npx --yes --package node@20 --call 'node --version && npm run type-check'`：通过，Node `v20.20.2`。
- `npx --yes --package node@20 --call 'npm run build:pro'`：通过，Vite 生产构建完成。
- 原版/当前视图文件交叉扫描：缺失 `0`，额外 `0`。
- 原版/当前动态 import 扫描：两侧各 `98` 个，全部可解析。

## 未完成与边界

- 页面依赖的后端真实能力、Docker/OpenResty/ACME 和部署验收由后端及测试部署任务继续验证；前端不以静态构建代替真实回路。
