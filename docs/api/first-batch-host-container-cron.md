# 首批主机、容器和计划任务迁移

旧 Agent 在 `app/api/v2` 中盘点到 77 条相关路由：系统组件 1 条、主机 9 条、容器与 Docker 52 条、计划任务 15 条（按旧源码注释中的 HTTP 方法和路径去重）。

本批已提供真实适配的接口：

- `POST /api/v2/system/command`：不经过 shell 拼接的程序执行，要求 `X-WorkMesh-Token` 与 `WORKMESH_COMMAND_TOKEN` 匹配，超时上限 5 分钟，输出上限 1 MiB。
- `GET /api/v2/hosts/components/{name}`：组件存在性检查。
- `GET /api/v2/hosts/system/info`：主机基础信息。
- `GET /api/v2/containers/docker/status`、`GET /api/v2/containers/list`、`POST /api/v2/containers/operate`：Docker CLI 状态、列表和生命周期白名单操作。
- `POST/GET /api/v2/cronjobs`、`POST /api/v2/cronjobs/search`、`POST /api/v2/cronjobs/handle`、`POST /api/v2/cronjobs/del`：计划任务内存适配。

其余旧路径全部注册并返回 `ERR/MIGRATION_PENDING`，避免静默 404。后续迁移必须以 `test/contract/routes.json` 的 831 条全量清单为验收基线，将占位实现替换为持久化模型、权限审计和流式传输。
