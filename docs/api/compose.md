<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Compose 编排接口

节点通过 Docker CLI 执行 Compose，所有参数按独立参数传递，不经过 shell 拼接：

- `POST /api/v2/containers/compose/search`：读取服务状态（等价 `docker compose -f <path> ps`）。
- `POST /api/v2/containers/compose/test`：校验并渲染配置（`config`）。
- `POST /api/v2/containers/compose/operate`：执行 `up/down/start/stop/restart/ps/config/pull`，请求体包含 `path`、`operation` 和可选 `services`。

文件路径和服务名由 Docker 自身校验；执行超时、退出码和标准输出统一使用节点命令响应结构。
