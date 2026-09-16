<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 网站配置 include 与测试隔离整改

状态：`[>]` 进行中；代码拆分和基础 OpenResty 配置回归已完成，真实网站全类型/全设置验收未完成。

## 本次完成

- `WebsiteService` 显式传入临时根目录时优先使用该目录，避免测试调用环境变量后写入生产网站路径。
- 管理 include 按真实生成文件判断；目录资源使用 `*.conf`，缺失普通文件不写入 include。
- 网站默认权限在非特权临时目录中忽略 `EPERM/EINVAL` 的属主变更失败，但仍保留权限模式校验。
- 增加临时 SQLite/文件站点回归，覆盖空 upstream、目录误引用和用户配置保留。
- `node/service/website.go` 已按 schema、加载、持久化、生命周期、域名配置、渲染、include、rewrite、WAF、OpenResty、分组和证书职责拆分，主文件约 376 行；新增网站服务文件均不超过 500 行。
- 生产 WAF `workmesh-openresty-waf` 当前 healthy，`nginx -t` 成功；`znmp.sopvip.com` 入口返回 HTTP 200。

## 验证

```text
GOCACHE=$PWD/.cache/go-build GOPATH=$PWD/.cache/gopath GOWORK=off \
go test ./node/service -run 'TestManagedWebsiteIncludesOnlyReferenceGeneratedFiles|TestWebsiteServicePersistsWebsiteAndWAF|TestWebsiteRuntimeID' -count=1
```

结果：通过。

## 残余

- 部分历史方法仍超过 80 行，需要继续按校验、事务、文件落盘和回滚拆分；不能把质量门禁标记为通过。
- 一键部署、运行环境、静态、反向代理、子网站、TCP/UDP 六类网站以及域名、HTTPS、CORS、真实 IP、伪静态、防盗链、重定向、PHP 等设置尚未完成逐项真实黑盒验收。
- Let's Encrypt HTTP-01 需要对具体 `*.cs.sopvip.com` 前置子域名逐个签发/续期验证；通配符证书不属于 HTTP-01 能力。
