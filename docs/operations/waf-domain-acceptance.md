<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# WAF 域名、HTTPS 和 ACME 真实验收矩阵

本文档只记录无凭据的验收矩阵和命令模板。当前主项目为 `/www/apps/workmesh-server`，前后端和 WAF 镜像源码均在该目录内，业务参考为只读 `/www/apps/1Panel`。执行前必须取得维护窗口、隔离域名、有效登录会话、DNS/ACME 授权和可回滚备份；本文档中的命令不得在未授权生产环境直接执行。

执行真实 OpenResty/WAF 验收前，必须先完成：

```bash
deploy/acceptance/preflight.sh
deploy/database/preflight.sh
deploy/acceptance/site-reconcile.sh --strict
```

其中 `site-reconcile.sh --strict` 未通过时，不得直接对 `/www/wwwroot` 中未与 SQLite 匹配的站点配置执行 reload、导入或删除。

## 已有可复用证据

- `node/api/website_external_test.go`：`WORKMESH_WEBSITE_EXTERNAL_TEST=1 go test ./node/api -run '^TestExternalWebsiteLifecycle$'` 可在隔离 Docker/OpenResty 中验证站点生成配置、`openresty -t`、reload、HTTP/HTTPS 访问和失败回滚。
- `node/api/website_waf_external_test.go`：`WORKMESH_WAF_EXTERNAL_TEST=1 go test ./node/api -run '^TestExternalWAFLifecycle$'` 可在隔离容器中验证 WAF Lua 规则、黑白名单和审计日志。
- `test/contract/http-smoke.mjs`：只覆盖无需登录的 HTTP smoke，不覆盖登录后站点、ACME、DNS、证书签发或生产 OpenResty。
- `test/contract/website-p0-request-templates.mjs`：能生成登录后网站 API 请求模板，但不是生产公网域名、ACME 或证书验收脚本。

结论：当前仓库已有隔离环境测试和只读预检脚本，但缺少生产/准生产公网域名、HTTP-01、DNS-01、HTTPS、WAF 和回滚的真实执行证据。

## 环境变量占位

```bash
export BASE='https://<workmesh-admin-host>'
export COOKIE='<workmesh_session>'
export CSRF='<pcsrftoken>'
export DOMAIN='workmesh.cs.sopvip.com'
export ALT_DOMAIN='<www.acceptance.example.com>'
export PUBLIC_IP='61.184.12.165'
export WEBSITE_ID='<website-id>'
export SSL_ID='<ssl-id>'
export ACME_ACCOUNT_ID='<acme-account-id>'
export DNS_ACCOUNT_ID='<dns-account-id>'
export OPENRESTY_CONTAINER='<openresty-container-name>'
export LETSENCRYPT_DIR='/etc/letsencrypt'
export CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"
export CT='Content-Type: application/json'
```

仓库内只读域名验收脚本默认就是上述 `DOMAIN` 和 `PUBLIC_IP`。如需测试其他
域名/IP，必须显式传入参数或环境变量，避免把 DNS 解析结果误当作目标地址。
`deploy/acceptance/domain-acceptance.sh` 的普通请求默认接受
`200,301,302,307,308`，HTTPS 根路径默认接受 `200`，攻击请求默认要求
`403`；可用 `--expected-http-status`、`--expected-https-status` 和
`--expected-attack-status` 调整已批准的站点策略。

## 验收矩阵

| 阶段 | 目标 | 命令模板 | 通过标准 |
| --- | --- | --- | --- |
| 1. DNS 只读预检 | 域名解析到目标主机 | `dig +short A "$DOMAIN"`、`dig +short AAAA "$DOMAIN"`、`curl --resolve "$DOMAIN:80:$PUBLIC_IP" "http://$DOMAIN/"` | A/AAAA 与目标一致；HTTP 80 可达且返回预期状态，Host 命中目标站点或预期默认站点 |
| 2. 站点域名记录 | SQLite 域名记录和 OpenResty `server_name` 一致 | `curl --fail-with-body -sS "$BASE/api/v2/websites/domains/$WEBSITE_ID" -H "$CID"` | 响应包含 `$DOMAIN`；生成的 `site.conf` 包含对应 `server_name` |
| 3. OpenResty 配置检查 | 写入前确认当前配置有效 | `docker exec "$OPENRESTY_CONTAINER" nginx -t` | 返回 0；没有 duplicate listen、证书路径缺失或 include 错误 |
| 4. HTTP-01 webroot | challenge 目录真实可公网访问 | `curl --fail-with-body -sS --resolve "$DOMAIN:80:$PUBLIC_IP" "http://$DOMAIN/.well-known/acme-challenge/<probe-file>"` | 返回预期探针内容；不得由固定 JSON 或反代假响应代替 |
| 5. ACME HTTP-01 签发 | WorkMesh 使用 certbot webroot 真实签发 | `curl --fail-with-body -sS -X POST "$BASE/api/v2/websites/ssl/obtain" -H "$CID" -H "$CT" --data '{"ID":'"$SSL_ID"'}'` | API 先返回 `status=applying`；随后 SSL 查询变为 `ready`，`message` 无错误 |
| 6. HTTP-01 证书落盘 | SQLite、证书文件和域名覆盖一致 | `curl --fail-with-body -sS "$BASE/api/v2/websites/ssl/$SSL_ID" -H "$CID"`、`openssl x509 -in "$LETSENCRYPT_DIR/live/$DOMAIN/fullchain.pem" -noout -subject -issuer -dates -ext subjectAltName` | 证书未过期，SAN 覆盖 `$DOMAIN`/`$ALT_DOMAIN`；私钥不出现在 API JSON |
| 7. DNS-01 自动签发 | DNS provider 写入、传播、清理真实发生 | `curl --fail-with-body -sS -X POST "$BASE/api/v2/websites/ssl/obtain" -H "$CID" -H "$CT" --data '{"ID":'"$SSL_ID"'}'`、`dig +short TXT "_acme-challenge.$DOMAIN"` | 签发期间 TXT 可查；完成后状态为 `ready`，provider 凭据未进入响应或日志 |
| 8. DNS-01 手动签发 | 手动 TXT resolve/finalize 完整闭环 | `curl --fail-with-body -sS -X POST "$BASE/api/v2/websites/ssl/resolve" -H "$CID" -H "$CT" --data '{"websiteSSLId":'"$SSL_ID"'}'`、`dig +short TXT "_acme-challenge.$DOMAIN"`、`curl --fail-with-body -sS -X POST "$BASE/api/v2/websites/ssl/obtain" -H "$CID" -H "$CT" --data '{"ID":'"$SSL_ID"',"TXTRecords":{"'"$DOMAIN"'":"<txt-value>"}}'` | resolve 返回的 TXT 与公网 DNS 一致；finalize 后证书为 `ready` |
| 9. HTTPS 启用 | WorkMesh 将证书绑定到站点并生成 443 配置 | `curl --fail-with-body -sS -X POST "$BASE/api/v2/websites/$WEBSITE_ID/https" -H "$CID" -H "$CT" --data '{"enable":true,"websiteSSLId":'"$SSL_ID"',"httpConfig":"HTTPToHTTPS","SSLProtocol":["TLSv1.3","TLSv1.2"],"http3":false}'` | API 成功；`site.conf` 出现受管 HTTPS 块；证书文件权限为 cert `0644`、key `0600` |
| 10. HTTPS 路由 | 公网 443 Host 路由和证书链正确 | `curl -Iv --resolve "$DOMAIN:443:$PUBLIC_IP" "https://$DOMAIN/"`、`openssl s_client -connect "$PUBLIC_IP:443" -servername "$DOMAIN" -showcerts </dev/null` | HTTP 200/预期跳转；SNI 证书匹配；TLS 协议和证书链符合配置 |
| 11. HTTP 到 HTTPS | 80 端口跳转策略符合站点配置 | `curl -I --resolve "$DOMAIN:80:$PUBLIC_IP" "http://$DOMAIN/path?q=1"` | `HTTPToHTTPS` 时返回 301/302 到同 Host HTTPS；`HTTPSOnly`/关闭时符合配置 |
| 12. WAF 生效 | 页面保存后的 WAF 设置真实影响 OpenResty 请求 | `curl -i --resolve "$DOMAIN:443:$PUBLIC_IP" "https://$DOMAIN/<known-block-path>"`、`curl --fail-with-body -sS "$BASE/api/v2/websites/waf/logs" -H "$CID" -H "$CT" --data '{"websiteId":'"$WEBSITE_ID"',"page":1,"pageSize":20}'` | block 模式返回 403；日志含真实 Host、URL、规则、动作、状态和 IP 归属地字段 |
| 13. 续期 | 自动续期不会伪造成功 | `certbot renew --dry-run --config-dir "$LETSENCRYPT_DIR" --work-dir "$LETSENCRYPT_DIR/work" --logs-dir "$LETSENCRYPT_DIR/logs"` 或等待 WorkMesh 续期任务 | 新证书被 `SyncCertbotLiveCertificates`/续期任务同步到 SQLite；失败状态和日志可见 |
| 14. 失败回滚 | 无效证书或无效配置不会污染运行态 | 使用无效 `websiteSSLId` 或不覆盖域名的测试证书调用 HTTPS 更新 | API 返回错误；SQLite、`site.conf`、证书文件和 OpenResty 有效配置保持调用前状态 |
| 15. 清理 | 验收资源可审计清理 | 查询站点、SSL、ACME/DNS 账户和日志记录 | 临时站点、临时证书和临时 DNS TXT 清理完成；生产证书和业务域名不被误删 |

## API 命令模板

创建 ACME 账户。`caDirURL` 可指向 Let's Encrypt Staging；不要在文档或终端历史中写入 EAB HMAC 等敏感值。

```bash
curl --fail-with-body -sS -X POST "$BASE/api/v2/websites/acme" \
  -H "$CID" -H "$CT" \
  --data '{"email":"ops@example.com","type":"letsencrypt","keyType":"RSA2048","caDirURL":"https://acme-staging-v02.api.letsencrypt.org/directory"}'
```

创建 HTTP-01 证书记录。

```bash
curl --fail-with-body -sS -X POST "$BASE/api/v2/websites/ssl" \
  -H "$CID" -H "$CT" \
  --data '{"primaryDomain":"'"$DOMAIN"'","otherDomains":"'"$ALT_DOMAIN"'","provider":"http","acmeAccountId":'"$ACME_ACCOUNT_ID"',"autoRenew":true,"keyType":"RSA2048"}'
```

创建 DNS-01 自动证书记录。DNS 账户凭据必须通过正式接口或界面单独录入，本文档不保存凭据。

```bash
curl --fail-with-body -sS -X POST "$BASE/api/v2/websites/ssl" \
  -H "$CID" -H "$CT" \
  --data '{"primaryDomain":"'"$DOMAIN"'","otherDomains":"'"$ALT_DOMAIN"'","provider":"dnsaccount","acmeAccountId":'"$ACME_ACCOUNT_ID"',"dnsAccountId":'"$DNS_ACCOUNT_ID"',"autoRenew":true,"keyType":"RSA2048","skipDNS":false}'
```

轮询证书状态和申请日志路径。

```bash
curl --fail-with-body -sS "$BASE/api/v2/websites/ssl/$SSL_ID" -H "$CID"
```

绑定证书并启用 HTTPS。

```bash
curl --fail-with-body -sS -X POST "$BASE/api/v2/websites/$WEBSITE_ID/https" \
  -H "$CID" -H "$CT" \
  --data '{"enable":true,"websiteSSLId":'"$SSL_ID"',"httpConfig":"HTTPToHTTPS","SSLProtocol":["TLSv1.3","TLSv1.2"],"http3":false}'
```

## 运维执行顺序

1. 先在隔离 Docker 环境跑 `WORKMESH_WEBSITE_EXTERNAL_TEST=1` 和 `WORKMESH_WAF_EXTERNAL_TEST=1`，确认源码和镜像基础行为。
2. 执行 `deploy/acceptance/preflight.sh`、`deploy/database/preflight.sh` 和 `deploy/acceptance/site-reconcile.sh --strict`；站点目录、SQLite 记录和 WAF 文件未对账通过前，不进入生产 OpenResty reload。
3. 再用 Let's Encrypt Staging、隔离域名和真实 DNS 执行 HTTP-01、DNS-01 和 HTTPS 路由验收。
4. 每次写操作前后保存 `site.conf`、SSL 记录、WAF 配置 SHA、`nginx -t` 输出和公网 `curl` 结果。
5. 只有当 DNS、ACME、OpenResty reload、HTTPS、WAF 请求矩阵和回滚全部有证据时，才能把对应任务从 `not-run` 改为完成。
