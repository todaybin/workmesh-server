# 应用路由结构整改

日期：2026-09-07

## 本次范围

- 仅修改 `node/api/apps_routes.go`，并新增 `node/api/apps_route_handlers.go`。
- 保留所有 `/api/v2/apps/*`、`/api/v2/custom/app/*` 和跨节点安装路由的路径、方法、请求解析、响应 envelope 与 SQLite 状态来源。
- 未修改 `/www/apps/1Panel`、前端代码、生产服务和 OpenResty 配置。

## 结构结果

- `RegisterAppRoutes` 现在只负责创建共享 `appRouteHandlers` 并按目录、已安装应用、扩展接口三组注册路由。
- 目录搜索、筛选分页、更新检查、标签整理和应用详情处理器集中在 `apps_route_handlers.go`。
- 原 `apps_routes.go` 从 432 行降至 140 行；新增处理器文件 455 行，单个函数均保持在 80 行以内。
- 所有新增和修改函数均有中文职责注释；未引入模拟数据或固定成功响应。

## 验证

```text
GOCACHE=/tmp/workmesh-app-routes-gocache GOWORK=off go test ./node/api -run 'Test(App|RemoteApp|PHPVersionApp|RuntimeCatalog|LegacyCompatibilityDoesNotOverrideAppRoutes|ApplyAppContainerStates|OpenRestyInstalledCheck)' -count=1
ok  github.com/todaybin/workmesh-server/node/api  0.995s

git diff --check -- node/api/apps_routes.go node/api/apps_route_handlers.go
通过
```

首次在受限沙箱运行定向测试时，Go 测试环境无法监听 `httptest` 本地端口；切换测试构建缓存并允许测试进程监听随机本地端口后，测试通过。未执行全量门禁，交由唯一集成测试负责人统一执行。
