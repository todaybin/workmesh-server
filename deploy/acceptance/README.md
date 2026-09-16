<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WorkMesh 生产预检

当前主项目只有 `/www/apps/workmesh-server`。前端、后端、部署脚本和 WAF 镜像资产都在本仓库内；业务行为只参考只读 `/www/apps/1Panel`，不得再把已移除的 `workmesh-node` 作为当前来源。

`preflight.sh` 是只读预检入口，用于在执行真实域名、WAF、数据库和发布验收前确认环境条件。脚本不会启动、停止、重启、删除容器，不签发证书，不写 DNS，不 reload OpenResty。

默认执行：

```bash
deploy/acceptance/preflight.sh
```

常用参数：

```bash
WORKMESH_ACCEPTANCE_DOMAIN=workmesh.cs.sopvip.com \
WORKMESH_PUBLIC_IP=61.184.12.165 \
CHECK_CONTAINER=1 \
STRICT=1 \
deploy/acceptance/preflight.sh
```

环境变量：

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `WORKMESH_DATA_DIR` | `/opt/workmesh-server/data` | Server 数据目录 |
| `WORKMESH_WEBSITE_ROOT` | `/www/wwwroot` | 网站根目录 |
| `WORKMESH_OPENRESTY_DIR` | `/opt/workmesh-server/openresty-waf` | OpenResty/WAF 部署目录 |
| `WORKMESH_OPENRESTY_CONTAINER` | `workmesh-openresty-waf` | 容器内 `nginx -t` 检查目标 |
| `WORKMESH_ACCEPTANCE_DOMAIN` / `DOMAIN` | `workmesh.cs.sopvip.com` | 域名解析检查目标 |
| `WORKMESH_PUBLIC_IP` / `ADDRESS` | `61.184.12.165` | 期望解析地址；传入 `auto` 才自动发现本机非回环 IPv4 |
| `WORKMESH_CONTAINER_OPENRESTY_BIN` | `/usr/local/openresty/nginx/sbin/nginx` | 容器内 `nginx -t` 可执行文件路径 |
| `CHECK_CONTAINER` | `0` | 设为 `1` 时只读执行容器内 `nginx -t`，并要求健康状态为 `healthy` |
| `STRICT` | `0` | 设为 `1` 时任何 warning 都使脚本返回非零 |
| `WORKMESH_PREFLIGHT_TMP` | `/tmp/workmesh-preflight-$$` | 数据库 Compose 静态检查的临时 dummy secret 目录 |

脚本覆盖：

- Docker CLI、Docker daemon/socket、Compose 可用性；
- OpenResty/WAF 目录、WAF 数据/生成/日志目录、关键生成文件；
- OpenResty Compose 静态配置；
- 数据库 Compose 模板静态配置；
- 域名解析和目标 IP 匹配；
- `curl`、`openssl`、`certbot`、`/etc/letsencrypt`；
- Chromium/Chrome/Firefox 和 Playwright/Puppeteer；
- 域名/WAF、OpenResty WAF 镜像和数据库验收脚本是否存在。

本机域名规划可先执行：

```bash
deploy/install/domain-plan.sh
```

它只输出 `<prefix>.cs.sopvip.com` 的 DNS A 记录和验收命令，不写 DNS 或生产配置。

脚本会从自身路径定位 `workmesh-server` 仓库根目录，因此可以从任意工作目录执行。

当前状态：

- [x] 只读预检入口和证据收集入口已在本仓库内整理完成。
- [!] 当前环境 `preflight.sh` 仍有 warning：Docker daemon/socket 不可访问、生产 WAF 挂载缺 `generated/standard-rules.conf`、默认域名 `workmesh.cs.sopvip.com` 尚未解析、缺少浏览器和 Playwright/Puppeteer；严格模式还会拒绝旧 `20260904` WAF 镜像。
- [!] 当前环境 `deploy/database/preflight.sh` 仍有 warning：生产数据库运行 Compose 和 secret 目录缺失，且 Docker daemon 不可用。
- [>] 严格模式全部通过前，不进入生产 OpenResty reload、数据库容器启动、证书签发或发布切换。

`preflight.sh` 只证明前置条件，不替代以下真实验收：

- `deploy/acceptance/domain-acceptance.sh` 与 `deploy/openresty-waf/tests/domain-acceptance.sh` 的 Host 路由、HTTPS 和 WAF 请求矩阵；
- `deploy/database/acceptance.sh` 的数据库容器启动、健康检查、备份和恢复；
- ACME HTTP-01/DNS-01 签发、续期和失败回滚；
- 生产二进制发布、服务重启和回滚。

## 站点与域名对账

`site-reconcile.sh` 用于只读核对 SQLite 网站/域名记录和站点目录中的
`nginx/site.conf` 是否一致，同时检查每个站点的 `waf/config.json` 与 `waf/rules.json`
是否存在。它只读取 SQLite 与网站文件，并把报告写入输出目录，不 reload OpenResty、
不修改 DNS、不删除历史站点。

脚本优先使用系统 `sqlite3 -readonly`。如果目标环境没有 `sqlite3`，会回退到仓库内
Go 导出器 `deploy/acceptance/cmd/site-reconcile-sqlite`，以 `mode=ro` 打开数据库；
也可以通过 `WORKMESH_SITE_RECONCILE_SQLITE_EXPORTER=/path/to/site-reconcile-sqlite`
提供预编译二进制，避免现场依赖 Go 工具链。

```bash
deploy/acceptance/site-reconcile.sh \
  --sqlite-db /opt/workmesh-server/data/workmesh.db \
  --website-root /www/wwwroot \
  --out /tmp/workmesh-site-reconcile-$(date +%Y%m%d-%H%M%S)
```

输出内容：

- `summary.txt`：SQLite 网站/域名数量、文件系统 `site.conf` 数量、warning 和 mismatch 计数。
- `sqlite-websites.tsv`、`sqlite-domains.tsv`：SQLite 只读摘要。
- `filesystem-site-confs.tsv`：文件系统中发现的 `nginx/site.conf`。
- `sqlite-to-files.tsv`：SQLite 记录到 `site.conf` 和 WAF 文件的核对结果。
- `files-to-sqlite.tsv`：文件系统 `site.conf` 是否能匹配 SQLite `site_dir`。
- `orphan-site-classification.tsv`：孤立 `site.conf` 的只读分类和建议动作。
- `mismatches.txt`：缺失域名、缺失配置、孤儿 `site.conf` 和 WAF 文件缺失记录。

2026-09-12 当前环境只读对账结果：`/opt/workmesh-server/data/workmesh.db` 可读，
但 SQLite `websites=0`、`website_domains=0`，而 `/www/wwwroot` 下存在 19 个
`nginx/site.conf`，`orphan_site_confs=19`。新增分类报告已将 `.example`/`types-*`/
`waf-test.example`/迁移样例与 `znmp.sopvip.com` 分开；这些文件系统站点配置仍需人工
确认归属，未获授权前不得删除、移动或自动导入。

站点对账状态：

- [!] SQLite 与文件系统当前不一致，19 个 `site.conf` 不能视为已入库站点。
- [>] 必须先输出分类清单，再由人工确认导入 SQLite、重建或授权清理。
- [!] 对账未通过前，不得把域名部署、站点测试或 WAF 生产 reload 标为完成。

严格模式会在存在 warning 或 mismatch 时返回非零：

```bash
deploy/acceptance/site-reconcile.sh --strict
```

自检：

```bash
deploy/acceptance/site-reconcile.sh --self-test
```

## 只读证据收集

`readonly-evidence.sh` 用于拿到目标环境权限后集中保存脱敏证据。默认只写入证据目录，不修改 OpenResty、容器、数据库、证书、DNS 或网站文件；`curl` 只保存状态码、连接指标和脱敏响应头摘要，不保存响应体。
其中 `waf/runtime-summary.txt` 会显式记录 `runtime.json` 存在性、SHA-256、`effective`
状态、生成配置 `generated/*.conf` 和 `standard-rules.conf` 状态，便于确认 WAF 保存后
是否真正进入 OpenResty 生效链，而不是只完成表面保存。

默认执行：

```bash
deploy/acceptance/readonly-evidence.sh
```

带域名、站点配置和容器 `nginx -t` 的执行示例：

```bash
deploy/acceptance/readonly-evidence.sh \
  --out /tmp/workmesh-evidence-prod-$(date +%Y%m%d-%H%M%S) \
  --openresty-dir /opt/workmesh-server/openresty-waf \
  --website-conf /www/wwwroot/example.com/nginx/site.conf \
  --database-compose /opt/workmesh-server/data/database/docker-compose.yml \
  --sqlite-db /opt/workmesh-server/data/workmesh.db \
  --website-id 1 \
  --domain example.com \
  --resolve-ip 203.0.113.10 \
  --api-base https://panel.example.com \
  --cid "Cookie: workmesh_session=<secret>; pcsrftoken=<secret>" \
  --check-container
```

常用参数和环境变量：

| 参数 / 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `--out` / `EVIDENCE_DIR` | `/tmp/workmesh-evidence-<timestamp>` | 证据输出目录 |
| `--openresty-dir` / `WORKMESH_OPENRESTY_DIR` | `/opt/workmesh-server/openresty-waf` | WAF 目录、Compose 和生成配置所在目录 |
| `--container` / `WORKMESH_OPENRESTY_CONTAINER` | `workmesh-openresty-waf` | 可选容器 `nginx -t` 目标 |
| `--container-nginx` / `WORKMESH_CONTAINER_OPENRESTY_BIN` | `/usr/local/openresty/nginx/sbin/nginx` | 容器内 OpenResty nginx 路径 |
| `--check-container` / `CHECK_CONTAINER=1` | `0` | 执行只读 `docker exec <container> nginx -t` |
| `--website-conf` / `WEBSITE_CONF` | 空 | 保存脱敏后的站点 `site.conf` |
| `--database-compose` / `WORKMESH_DATABASE_COMPOSE` | 仓库数据库 Compose 模板 | 执行 `docker compose config --quiet` |
| `--sqlite-db` / `WORKMESH_SQLITE_DB` | `$WORKMESH_DATA_DIR/workmesh.db` | 只读查询网站、域名和运行状态摘要 |
| `--api-base` / `API_BASE` / `WORKMESH_API_BASE` | 空 | 探测控制面只读 API 状态 |
| `--website-id` / `WEBSITE_ID` | 空 | 指定站点的 API 和 SQLite 摘要 |
| `--base` / `BASE` | 空 | 探测站点或控制面根路径及攻击路径 |
| `--cid` / `CID` | 空 | 可选单个请求头，仅发送给 `curl`，不写入证据 |
| `--domain` / `DOMAIN` | 空 | 探测 `http://DOMAIN/`、`https://DOMAIN/` 及攻击路径 |
| `--resolve-ip` / `RESOLVE_IP` | 空 | 对 `DOMAIN` 探测使用 `curl --resolve` |
| `--attack-path` / `ATTACK_PATH` | 编码后的 script probe | WAF 攻击路径探测 |
| `--curl-timeout` / `CURL_TIMEOUT` | `10` | 单次 curl 超时秒数 |

输出内容：

- `metadata.txt`：采集时间、Git HEAD、工作区摘要、Docker 版本摘要、是否注入 `CID`。
- `openresty/`：本机 `openresty -t` 或 `nginx -t`，以及可选容器 `nginx -t` 输出。
- `waf/files.txt`、`waf/sha256.txt`：WAF 目录文件列表和逐文件 SHA-256。
- `site/site-conf.txt`：可选站点 `site.conf` 的脱敏快照和原文件 SHA-256。
- `database/compose-config-quiet.txt`：数据库 Compose 静态配置校验结果。
- `sqlite/summary.txt`：SQLite 网站、域名、OpenResty 配置长度和数据库运行状态摘要。
- `api/*.txt`：WAF、域名和站点规则只读 API 的状态码、下载字节数和响应头摘要。
- `http/*.txt`：可选 `BASE`、`DOMAIN`、HTTP、HTTPS 和攻击路径的状态码与响应头摘要。
- `manifest.txt`：证据目录文件清单。

WAF 和页面验收状态：

- [x] `docs/img/` 七张 WAF 参考图已有七个对应页面和路由。
- [!] 当前环境未完成浏览器截图像素验收、页面交互保存验收和保存后 `effective` 复核。
- [!] WAF 设置保存必须让 OpenResty 实际生效：生产数据目录下保存失败、`nginx -t` 失败、reload 失败或 reload 后复检失败，都不能标为完成。
- [>] 真实完成标准是保存 `nginx -t`、reload、reload 后复检、WAF 文件 hash、HTTP/HTTPS Host 路由、block/observe 请求和失败回滚证据。

合约自检：

```bash
deploy/acceptance/readonly-evidence.sh --self-test
```

自检会执行 Shell 语法检查、扫描脚本中的运行态变更命令，并验证 `CID` 与敏感响应头明文不会写入证据。
