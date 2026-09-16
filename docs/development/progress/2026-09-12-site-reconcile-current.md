<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 站点配置当前对账（2026-09-12）

状态：`[!]` 已完成只读发现；SQLite 与文件系统不一致，等待分类、导入或授权清理

本文件记录当前环境 `/opt/workmesh-server/data/workmesh.db` 与 `/www/wwwroot`
的只读对账结果。执行命令：

```bash
RECONCILE_DIR=/tmp/workmesh-site-reconcile-current-20260912 \
WORKMESH_SQLITE_DB=/opt/workmesh-server/data/workmesh.db \
WORKMESH_WEBSITE_ROOT=/www/wwwroot \
bash deploy/acceptance/site-reconcile.sh
```

脚本通过 Go SQLite fallback 以 `mode=ro` 打开数据库，没有执行 OpenResty reload、
Docker 操作、DNS 写入、数据库写入或网站目录清理。

## 当前结果

| 项目 | 数量 |
| --- | ---: |
| SQLite `websites` | 0 |
| SQLite `website_domains` | 0 |
| 文件系统 `nginx/site.conf` | 19 |
| warnings | 0 |
| mismatches | 19 |

结论：当前文件系统中存在 19 个 OpenResty 站点配置，但 SQLite 网站/域名关系表为空。
这些配置不能直接认定为 WorkMesh 已登记站点，也不能直接删除、导入或触发生产 reload；
下一步必须逐个确认归属。

## 未入库站点配置

| 路径 | `server_name` | 初步分类 | 下一步 |
| --- | --- | --- | --- |
| `/www/wwwroot/advanced.example/nginx/site.conf` | `advanced.example` | 历史/测试样例 | 如无需保留，走授权清理流程 |
| `/www/wwwroot/aliases.example/nginx/site.conf` | `aliases.example` | 历史/测试样例 | 如无需保留，走授权清理流程 |
| `/www/wwwroot/example.com/nginx/site.conf` | `example.com` | 历史/测试样例 | 如无需保留，走授权清理流程 |
| `/www/wwwroot/example.org/nginx/site.conf` | `example.org` | 历史/测试样例 | 如无需保留，走授权清理流程 |
| `/www/wwwroot/extended.example/nginx/site.conf` | `extended.example` | 历史/测试样例 | 如无需保留，走授权清理流程 |
| `/www/wwwroot/legacy-domain.example/nginx/site.conf` | `legacy-domain.example` | 历史/测试样例 | 如无需保留，走授权清理流程 |
| `/www/wwwroot/resource.example/nginx/site.conf` | `resource.example` | 历史/测试样例 | 如无需保留，走授权清理流程 |
| `/www/wwwroot/static.example/nginx/site.conf` | `static.example` | 历史/测试样例 | 如无需保留，走授权清理流程 |
| `/www/wwwroot/types-deployment.example/nginx/site.conf` | `types-deployment.example` | 网站类型验收样例 | 需用真实部署上游重建或清理 |
| `/www/wwwroot/types-php.example/nginx/site.conf` | `types-php.example` | 网站类型验收样例 | 需用真实 PHP-FPM 重建或清理 |
| `/www/wwwroot/types-proxy.example/nginx/site.conf` | `types-proxy.example` | 网站类型验收样例 | 需用真实反代上游重建或清理 |
| `/www/wwwroot/types-runtime.example/nginx/site.conf` | `types-runtime.example` | 网站类型验收样例 | 需用真实运行环境重建或清理 |
| `/www/wwwroot/types-static.example/nginx/site.conf` | `types-static.example` | 网站类型验收样例 | 需用隔离域名重建静态站点或清理 |
| `/www/wwwroot/types-stream.example/nginx/site.conf` | `types-stream.example` | 网站类型验收样例 | 需用真实 TCP/UDP upstream 重建或清理 |
| `/www/wwwroot/types-subsite.example/nginx/site.conf` | `types-subsite.example` | 网站类型验收样例 | 需用真实父子站点关系重建或清理 |
| `/www/wwwroot/upgrade.example/nginx/site.conf` | `upgrade.example` | 升级/迁移样例 | 对照升级用例后清理或归档 |
| `/www/wwwroot/valid.example/nginx/site.conf` | `valid.example` | 测试样例 | 如无需保留，走授权清理流程 |
| `/www/wwwroot/waf-test.example/nginx/site.conf` | `waf-test.example` | WAF 验收样例 | 需用真实 WAF 攻防矩阵重建或清理 |
| `/www/wwwroot/znmp.sopvip.com/nginx/site.conf` | `znmp.sopvip.com` | 可能的生产域名配置 | 优先核对 DNS、证书、目录内容和业务归属；确认后通过 API 导入或重建 SQLite 记录 |

## 处理计划

1. `[x]` 先冻结只读证据：保留 `summary.txt`、`files-to-sqlite.tsv`、相关 `site.conf`
   脱敏快照、WAF 文件 hash 和当前 Git/部署版本。
2. `[!]` 对 `znmp.sopvip.com` 单独执行域名解析、证书、HTTP/HTTPS、目录内容和 WAF 文件检查；
   如果确认为生产站点，优先走 API/迁移任务恢复 SQLite `websites` 与 `website_domains`
   记录，而不是手工改库。
3. `[>]` 对 `types-*`、`waf-test.example` 和其他 `.example` 配置按测试样例处理：
   需要继续验收的，用真实隔离域名和真实上游重建；不需要的，进入授权清理清单。
4. `[!]` 清理前必须先确认 OpenResty `nginx -t` 当前状态、备份待清理目录、生成回滚计划；
   清理后再次执行 `site-reconcile.sh --strict` 和域名/WAF 请求矩阵。
5. `[!]` 未完成导入或清理前，不能把网站/域名部署验收标为通过。

## 阻断

- [!] 当前环境无 Docker Socket 权限，无法验证容器内 OpenResty `nginx -t`、reload 和真实 WAF 拦截。
- [!] 当前没有管理员会话、DNS/ACME 凭据和生产维护窗口，不能执行 API 导入、证书签发或清理动作。
- [!] 历史文档曾记录 `znmp.sopvip.com` 在 SQLite 中存在站点记录；当前只读结果为空，
  需要先查明是数据迁移、数据库路径、部署版本还是后续清理导致的差异。
