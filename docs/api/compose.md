<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Compose 编排接口

节点通过 Docker CLI 执行 Compose，所有参数按独立参数传递，不经过 shell 拼接：

- `POST /api/v2/containers/compose/search`：读取服务状态（等价 `docker compose -f <path> ps`）。
- `POST /api/v2/containers/compose/test`：校验并渲染配置（`config`）。
- `POST /api/v2/containers/compose/operate`：执行 `up/down/start/stop/restart/ps/config/pull`，请求体包含 `path`、`operation` 和可选 `services`。

文件路径和服务名由 Docker 自身校验；执行超时、退出码和标准输出统一使用节点命令响应结构。

容器资源接口同样通过 `/api/v2/containers/` 统一适配：`GET image|network|volume` 查询资源，`POST image/{pull,push,remove,search,tag,build,save,load}` 管理镜像，`POST network[/del]` 与 `volume[/del]` 创建或删除网络、卷，`GET stats/{id}` 和 `POST info|inspect` 查询容器详情。名称参数仅允许 Docker 标识字符，禁止 shell 元字符。
