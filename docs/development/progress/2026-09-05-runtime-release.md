# 运行时契约、并发与结构整改

状态：[>] 进行中；候选版本已部署生产并完成基础健康检查，真实业务生命周期仍未全部验收。

## 原系统证据纠正

- `1Panel/agent/app/api/v2/helper/helper.go` 的 Success 返回 `code:200,message:"success"`。
- `1Panel/agent/app/dto/common_res.go` 的 `data` 无 omitempty，因此原 Success **包含 data:null**，并非省略 data 字段。此前计划描述需纠正。
- 原 runtime update/operate/sync/remark/PHP container update 使用 Success；原 frontend operate 使用大写 `ID` 和 `operate=up|down|restart`。

## 本轮实现

- [x] update/operate/sync/remark/PHP container update 改为原 Success envelope；原接口操作名优先，兼容已有 start/stop。
- [x] 操作类型白名单，避免任意 Docker 子命令透传；无 Compose 时 down 映射 Docker stop，不再调用不存在的 docker down。
- [x] 添加按资源 ID 的可回收互斥锁；create/update/operate/delete/remark/PHP container update 串行化同资源外部操作。
- [x] 安装任务获取资源锁并检查 taskID，避免创建后删除/重建启动旧任务。
- [x] 更新前复制 params，避免失败更新污染原记录；同步依据 UpdatedAt 拒绝过期快照。
- [>] 补充契约与并发测试；按业务职责拆分大文件。
- [x] 第一批职责拆分：验证/存储/HTTP、运行时路由/子路由、执行器、任务、记录校验、PHP、Node、工具箱分别归档；主文件由 4106 行降至 84 行，新增源码文件均不超过 500 行。
- [x] 生产候选部署：二进制 SHA256=`89597a1e2a67818c35a92bfb2e567705b6e55d854ec36bca29476aa4bb384e01`；备份目录 `/opt/workmesh-server/backups/deploy-20260905T235900+0800-website-split`；服务、`/health`、`/ready`、WAF `nginx -t` 和 `znmp.sopvip.com` 入口均正常。该版本包含应用安装后台任务的共享 `Config` map 竞态修复、SQLite 已初始化时跳过旧 JSON 回读和网站 API 职责拆分；备份包含旧二进制、配置和 SQLite 在线备份。

## 本轮验证

命令环境：`GOWORK=off GOCACHE=/www/apps/workmesh-server/.cache/go-build TMPDIR=/www/workspaces/workmesh-release-20260905-qxdhI5/.tmp`。

- [x] `go test ./node/api -run 'TestRuntimeLifecycleRoutes|TestRuntimeContainerStatusRequiresRunning' -count=1`：通过，2026-09-05；使用真实临时 SQLite，Docker 为单元测试命令执行器，不代表真实 Docker 验收。
- [x] `go test ./node/api -run '^TestRuntimeLifecycleRoutes$' -count=1`：通过，2026-09-05；拆分后编译及运行时生命周期回归通过。
- [x] `go test ./node/api -run 'Runtime|PHP|Node' -count=1`：被沙箱禁止 `httptest` 临时端口阻断（`operation not permitted`），不是代码通过证据。
- [x] `GOWORK=off go test -race ./node/api -run 'Runtime' -count=2`：通过，使用隔离测试资源；不代表真实 Docker 安装验收。
- [ ] 六类生产生命周期、PHP FPM/扩展/Supervisor 及删除后同名重建。
- [ ] 任务、日志、终端/WS 和跨节点转发的登录后黑盒闭环。

## 协作事项

- 协调者需同步修正总计划和接口清单中“Success 不含 data”的错误假设。
- 既有错误 envelope 与 1Panel Error helper 也存在全局差异，本任务不擅自修改共享错误处理。
- 当前删除无 Compose 的容器只停止，容器清理仍需补齐并验证。
