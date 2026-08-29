<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 进程与运行时接口（首批）

- `GET /api/v2/process/{pid}`：读取本机进程命令行和基础信息。
- `POST /api/v2/process/listening`：调用 `ss` 或 `netstat` 列出监听端口。
- `POST /api/v2/process/stop`：按 PID 终止进程，生产环境必须配置 `WORKMESH_COMMAND_TOKEN` 并在 `X-WorkMesh-Token` 中传入。

接口不拼接 shell 命令；进程读取失败返回结构化 `ERR`。WebSocket 进程流和运行时安装、PHP 扩展等高权限能力继续保留迁移占位，待接入独立任务队列后实现。
