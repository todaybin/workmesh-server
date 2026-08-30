<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 前端静态模块 MIME 修复

## 原因

旧兼容路由注册了 `GET /assets/*filepath`，会拦截真实前端资源并返回 JSON。浏览器加载 ES module 时收到 `application/json`，因此触发严格 MIME 类型错误。

## 修复

- 移除 `/assets` 的迁移占位处理。
- 由 WorkMesh Server 直接从 `web/dist` 提供 `/assets/{filepath...}` 静态文件。
- 增加 Go 回归测试，验证 JavaScript 返回 `text/javascript` 和真实文件内容。
- 更新契约扫描器，使 Go ServeMux 通配路径与旧路由清单保持一致。

## 线上验证

主节点 `61.184.12.165:9999` 和次节点 `162.14.96.198:9999` 均已更新制品：

- `/health`、`/ready`：HTTP 200。
- `/assets/js/index-oKb3ZpkU.js`：HTTP 200，`Content-Type: text/javascript; charset=utf-8`。
- `/assets/js/vendor-element-plus-DLuwRn2-.js`：HTTP 200，`Content-Type: text/javascript; charset=utf-8`。
- `/assets/css/style-BqLbKwD-.css`：HTTP 200，`Content-Type: text/css; charset=utf-8`。

本次 Linux amd64 制品 SHA256：

```text
1e6d1b25c3ffb5c291c16a9faf95ac6d2b78a0006c58a428d78370b57d9057c2
```
