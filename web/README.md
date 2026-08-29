<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh Server Web

这是 `workmesh-server` 的独立 Vue 3 + Vite 前端入口。已完整迁入原前端的 854 个源码文件（包括全部页面、路由、API 模块、组件、语言包和图标资源）。Gateway 状态、登录/注册和解绑继续通过 `src/api/modules/workmesh.ts`、`src/api/modules/auth.ts` 对接；`src/api/gateway.ts` 提供类型安全的标准能力路由适配。所有业务执行仍由 Go 服务端负责。

## 开发

```bash
npm install
npm run dev
```

生产构建：

```bash
npm run build
```

接口基础路径固定为 `/api/v2`。服务端需要提供：

- `GET /api/v2/workmesh/gateway/status`
- `POST /api/v2/workmesh/gateway/login`
- `POST /api/v2/workmesh/gateway/capabilities/route`

响应可以使用数字 `200` 成功 envelope（`{ code: 200, data: ... }`）或直接返回数据对象。前端不会伪造成功状态；接口未接入时会显示可验证的错误信息。
