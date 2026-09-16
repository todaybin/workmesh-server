<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Website Extensions 结构质量整改（2026-09-07）

状态：`[x]` 本批次完成

## 本批次变更

- `node/api/website_extensions.go` 从 608 行降至 416 行，保留 `/api/v2/websites/*` POST 多操作分发、参数兼容和响应 envelope。
- 新增 `node/api/website_extensions_store.go`（125 行），承载网站扩展 SQLite 状态、集合初始化、快照持久化和公共响应辅助。
- 新增 `node/api/website_extensions_get.go`（103 行），承载数据库、证书账户、默认页面、负载均衡和资源 GET 查询。
- 统一分发器仍保持同一 `websiteExtensionStore` 锁范围；旧 JSON 仅作为 SQLite 快照兼容输入，正式状态仍写入 `website_extension_state`。
- 保持同一路径多操作契约、字段别名、真实网站/数据库服务调用和错误状态不变；未修改 `/www/apps/1Panel`、`workmesh-node`、前端或生产环境。

## 验证

```text
gofmt -w node/api/website_extensions.go node/api/website_extensions_store.go node/api/website_extensions_get.go
go test ./node/api -run 'WebsiteExtension|WebsiteAdvanced|DefaultHTML|Website.*Auth|Website.*Proxy' -count=1
git diff --check -- node/api/website_extensions.go node/api/website_extensions_store.go node/api/website_extensions_get.go
```

结果：相关 `node/api` 定向测试通过（2.218s），差异检查通过。全量 Go/Race/Vet 门禁交由唯一集成测试负责人在源码冻结后执行。

## 备注

- ACME、WAF、模板和代理的真实业务仍由对应服务/适配器执行；本批次只调整扩展路由组织，不使用模拟业务数据。
