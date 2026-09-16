<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 网站域名配置服务质量整改（2026-09-06）

状态：`[x]` 本批次完成

## 本批次变更

- 将 `node/service/website_domains_config.go` 尾部的站点文件配置、代理配置解析和访问日志流量统计职责拆到同包文件 `website_domains_config_files.go`。
- 移动的方法和辅助函数保持原函数签名、包级可见性、锁顺序、原子文件替换、SQLite 写入和错误行为不变。
- 为移动后的 `containsString`、`UpdateWebsiteNginxIndex`、`ListWebsiteProxies`、正则解析和 `accessLogTraffic` 补充准确中文注释。
- 原配置文件从 565 行降至 369 行，拆分后新文件 216 行；没有改动 API 路径、OpenResty 目录布局或数据库结构。

## 验证

```text
gofmt -w node/service/website_domains_config.go node/service/website_domains_config_files.go
GOWORK=off go test ./node/service -run 'TestWebsite' -count=1
git diff --check -- node/service/website_domains_config.go node/service/website_domains_config_files.go docs/development/progress/2026-09-06-website-domains-config-quality.md
```

结果：网站服务定向测试通过，差异检查通过。未执行全量 Go/Node、race、vet、生产网站访问或 OpenResty 集成门禁。

## 后续风险

- `BasicConfig` 仍负责读取站点配置、日志状态和访问流量汇总，虽已移出域名配置主文件，后续如需继续拆分应保持返回字段和真实文件来源不变。
- `ListWebsiteProxies` 依赖站点目录下 `.conf/.bak` 文件，真实代理上游和 `nginx -t` 仍需由唯一集成测试负责人验证。
