<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WAF SQLite 数据源收口（历史方案）

状态：`[x]` 已被后续文件权威方案取代（2026-09-11）

> 本文记录的是 2026-09-06 的中间方案，不是当前 WAF 运行态的事实来源。
> 当前实现以 [`website-openresty-waf.md`](../../api/website-openresty-waf.md) 为准：
> 网站元数据仍使用 SQLite，WAF 运行态配置、规则、名单和 JSONL 审计日志使用
> WorkMesh 自有文件；SQLite 不参与 WAF 运行时读写。

## 当时的整改

- 当时曾计划在 SQLite 初始化成功时，将网站 WAF、全局 WAF 和访问控制列表只写入关系表：
  `website_waf_sites`、`website_waf_rules`、`website_waf_global`、`website_waf_access_entries`。
- 当时曾计划不再生成或读取 `site.json`、`global.json`、`access-lists.json`，避免 JSON 与数据库状态分叉。
- 旧版本 sidecar 只允许在启动时进行一次性兼容导入，导入来源、SHA-256 和时间写入 `website_waf_sidecar_imports`；后续重启不会再次覆盖 SQLite。
- 无数据库的隔离单测仍保留文件降级路径。
- 更新了 API 测试，使其验证 SQLite 关系表和“不生成 WAF sidecar”，不再把旧 JSON 文件当作正式输出。

## 历史验证

```text
go test ./node/service -run TestWAFNoDBSidecarsRemainCompatible -count=1
go test ./node/api -run 'Test(ZNMPStaticWebsiteAllInterfaces|WebsiteWAF)' -count=1
git diff --check
```

以上定向检查通过。全量 `go test`、`-race`、`go vet`、Node 合约和发布后检查由唯一集成测试负责人统一执行，避免重复测试和结果分散。

## 后续替代结果

- 后续为满足 OpenResty 真正生效要求，WAF 运行态改为 WorkMesh 自有 JSON/JSONL 文件权威。
- WAF 保存流程增加 `-t`、reload、reload 后复检、配置 hash、快照回滚和 `effective` 状态。
- 该历史文档不再作为当前 WAF 数据源或生产验收依据。
