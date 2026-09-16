# V2-WRITE-PARAM 参数契约专项审计

日期：2026-09-08
状态：`[>] 进行中`（通用尾随 JSON 边界已完成，字段白名单和真实外部资源仍待完成）

范围：`node/api` 的应用安装/操作、网站配置/操作、容器操作、运行时更新、系统设置高风险写接口。目标是确认请求字段、必填校验、范围限制、JSON 尾部、真实持久化和统一 v2 envelope；本轮只审计和运行已有定向测试，没有修改前端、1Panel 或生产数据。

## 路由族与当前证据

| 路由族 | 入口 | 已验证 | 结论 |
| --- | --- | --- | --- |
| 应用 | `/api/v2/apps/install`、`/api/v2/apps/installed/*` | `apps_test.go`、`app_install_task_test.go` | 安装记录、任务日志、Compose/.env 有真实落盘；`appBody` 仍忽略 JSON 解码错误和尾随 JSON，字段未做统一白名单，需后续收紧 |
| 网站 | `/api/v2/websites/config`、`config/update`、`operate`、`domains`、`proxy/*`、`lbs/*`、`cors/update`、`realip/config`、`stream/update` | `website_test.go`、`website_advanced_test.go`、`website_isolation_test.go` | 站点配置和 OpenResty 片段读取/写入真实存储，路径和部分数值有校验；公共 `decodeJSON` 不拒绝未知字段或尾随 JSON，需逐路由参数白名单 |
| 容器 | `/api/v2/containers/*`、`/api/v2/compose/*` | `container_features_test.go`、`compose_test.go`、`container_*_test.go` | Docker 参数、名称、环境变量、路径有白名单，Compose/模板状态可持久化；容器操作解析器对未知字段和尾随 JSON 宽松，外部 Docker 状态未写 SQLite（符合外部资源模型，但需黑盒验收） |
| 运行时 | `/api/v2/runtimes`、`operate`、`update`、`del`、PHP 配置 | `runtime_lifecycle_test.go`、`runtime_persistence_test.go`、`runtime_toolbox_test.go` | `runtimeBody` 拒绝多段 JSON；端口 1-65535、协议、容器名、PHP_VERSION 等有范围校验；更新状态和任务记录持久化，真实 Docker/安装命令仍需隔离环境测试 |
| 设置 | `/api/v2/settings/*`、`/api/v2/core/settings/*`、`/api/v2/config/global` | `host_container_cron_operational_test.go`、`core_handlers_test.go`、`analytics_test.go` | 关键设置可持久化并有部分布尔/数值校验；`requestMap` 当前只 Decode 一次，不拒绝尾随 JSON，多个设置入口忽略解析错误，需统一严格解析和字段白名单 |

## 关键缺口

1. `appBody` (`node/api/apps.go`) 对 JSON 解码错误使用忽略策略，`/apps/install` 等写接口可能把畸形请求当成空对象继续处理；这是上线前阻断项。
2. `requestMap` (`node/api/functional_domains.go`) 原先不检查第二个 JSON 值；本轮已接入公共 `decodeSingleJSON`，设置、日志、备份等复用入口现在拒绝尾随 JSON，但字段白名单仍需逐路由补齐。
3. `decodeJSON` (`node/api/host_container_cron.go`) 和 `decodeExtension` 不启用 `DisallowUnknownFields`，未知字段会静默接受。需要按 1Panel 实际契约逐路由建立白名单，不能全局贸然拒绝旧前端兼容字段。
4. 应用状态当前仍调用 `saveJSONState("app_store_state", ...)` 并维护 `apps.json` 兼容文件；SQLite 关系表同步存在，但尚未证明 SQLite 是唯一生产数据源。后续必须完成一次 SQLite-only 迁移后再删除写 JSON。
5. 容器 operate/update 的成功性依赖 Docker CLI/daemon，定向单元测试只能证明参数和命令，不代表真实 Docker 生命周期已通过。

## 本轮验证

```text
GOWORK=off go test ./node/api -run 'Test(App|Website|Container|Runtime|HostMonitorSettings|Compose)' -count=1
```

结果：`ok github.com/todaybin/workmesh-server/node/api`，耗时约 14 秒。

追加验证：

```text
GOWORK=off go test ./node/api -run 'Test(Decode|App|Website|Container|Runtime|HostMonitorSettings|Compose)' -count=1
```

结果：通过；新增 `decodeSingleJSON`、`requestMap`、`decodeJSON` 尾随值拒绝回归测试。

## 下一步拆分

- `V2-PARAM-APP-01`：为 apps 写入口引入严格单对象解码测试，再按兼容字段建立白名单。
- `V2-PARAM-SETTINGS-01`：统一 requestMap 的尾随 JSON 和类型/范围测试，先覆盖设置写接口。
- `V2-PARAM-WEBSITE-01`：按 scope/operate 建立网站配置字段矩阵，确认 SQLite/OpenResty 双写和回滚。
- `V2-PARAM-CONTAINER-01`：用隔离 Docker daemon 验证 operate/create/update 的真实生命周期；没有 daemon 时保持 blocked，不使用模拟成功。
- `V2-PARAM-SQLITE-01`：完成 app_store JSON 到 SQLite 的只读迁移、回滚和 SQLite-only 读写验收。
