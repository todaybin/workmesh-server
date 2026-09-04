<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Gateway 适配边界

本目录只定义节点与云端 Gateway 的协议契约和适配接口。实现必须满足：

- 所有节点独立注册，注册凭证和私钥不进入日志或前端响应。
- 仅使用 HTTPS、请求签名、时间戳、nonce、幂等键和协议版本。
- WorkMesh 节点协议固定为 `v2`；不再提供 `/api/workmesh/v1` 路径或 v1 header 回退。
- Gateway 离线时，已注册节点的本机能力可按策略继续运行；云端任务不得绕过授权。
- `CapabilityRouter` 是本机和 Gateway 透传的共同入口，禁止复制两套沙盒执行逻辑。

TODO：补充 Gateway 版本化 OpenAPI、签名算法和错误码契约；由 Gateway 对接负责人实现 HTTP adapter 与重试队列。
