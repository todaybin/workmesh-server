# WorkMesh Server 统一架构

## 目标

`workmesh-server` 是一个同时承载 control 和 node 业务分区的单进程应用。浏览器只访问公网入口，服务端在同一进程内共享配置、认证、存储、路由注册表、调度器和生命周期。

## 运行时边界

```text
公网 HTTP(S)
  -> WorkMesh Server
     -> 安全中间件与 API v2
        -> control 业务分区
        -> node 业务分区
        -> NodeRelay（仅服务端透传）
     -> web/dist（同源静态资源）
     -> Unified SQLite Store
```

control 与 node 不再启动独立 HTTP 服务，也不各自创建 Store、Scheduler 或后台任务。每个领域的服务实例在 Bootstrap 阶段创建一次，通过依赖注入提供给路由和任务执行器。

## 配置与启动

配置优先级为 `WORKMESH_SERVER_CONFIG` 指定的 `config/server.json`，再由环境变量覆盖。监听地址、监听端口、`publicURL`、数据目录、超时和资源上限均来自配置，源码不写入生产端口。启动顺序如下：

1. 读取并校验配置，创建数据目录。
2. 打开统一 SQLite，设置 WAL、外键和 busy timeout，执行幂等 schema migration。
3. 扫描旧 JSON/SQLite 数据源，按摘要记录导入结果；导入失败时阻止就绪。
4. 恢复部署状态，创建一次性服务注册表和按需调度器。
5. 注册 control、node、relay 和静态资源路由。
6. 绑定 HTTP 监听器；只有迁移完成且依赖检查通过时 `/ready` 才返回 200。

`/health` 只反映进程存活，不访问数据库；`/ready` 反映迁移、存储和关键依赖状态，未就绪返回 503。

## 数据与更新

长期数据统一存储在 `${dataDir}/workmesh.db`。旧文件只作为一次性导入源保留，不删除、不覆盖。迁移记录包含版本、checksum、制品摘要、备份路径、状态和错误信息。

版本更新状态机为 `staged -> migrating -> active`。候选制品先在独立目录启动并执行迁移与健康检查，迁移失败进入 `migration_failed`，旧数据库摘要必须保持不变；只有 `/ready` 通过后才切换 systemd 指向新制品。

## 资源模型

无任务时不创建 Cron ticker；无证书时不创建证书扫描 goroutine；未配置 Gateway 时不启动心跳；缓存使用有界 TTL。长任务、SSE 和并发转换受配置中的 limits 限制。

## 前端和节点透传

前端 API、SSE、WebSocket 和文件请求全部使用当前页面 origin 下的 `/api/v2`，开发环境通过 Vite 代理转发，不向浏览器暴露节点地址。`operateNode`/`CurrentNode` 只由服务端 NodeRelay 解析；relay 对目标节点做白名单校验、签名、时间戳、nonce 和 role epoch 校验，并支持流式响应、Range 和 WebSocket 升级透传。

## 部署与回滚

部署到 Linux amd64 时先对远端做只读预检并备份旧服务、配置和数据库。候选版本使用临时端口验证 `/health`、`/ready`、登录、网站、分组、OpenResty、日志及流式接口，确认迁移成功后再切换 systemd。旧二进制和数据库始终保留；失败时停止候选并恢复旧 unit。
