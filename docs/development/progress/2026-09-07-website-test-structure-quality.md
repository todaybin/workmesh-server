# Website Test Structure Quality

日期：2026-09-07

## 完成项

- 将 `node/api/website_test.go` 中高级网站、OpenResty 与 XPack 路由测试拆分到 `node/api/website_advanced_test.go`。
- 保持测试请求、响应断言和真实 SQLite/文件数据行为不变；补回扩展字段测试的响应解码片段。
- `website_test.go` 442 行，`website_advanced_test.go` 426 行，均低于单文件 500 行阅读上限。

## 验证

```text
GOWORK=off go test ./node/api -run 'TestWebsite|TestOpenResty|TestXPack' -count=1
ok  github.com/todaybin/workmesh-server/node/api  12.343s
```

本次未执行全量门禁，未修改 `apps/1Panel`、前端或生产部署。
