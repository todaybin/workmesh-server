# ACME/SSL 验收进度（2026-09-08）

## 本轮完成

- 创建 SSL 后，HTTP-01、Let's Encrypt provider 和 DNS 自动验证会自动进入异步申请流程。
- DNS-01 自动流程使用 SQLite 中登记的 DNS 账户凭据，支持当前已接入的 Cloudflare、AliDNS、TencentCloud、AWS Route53 和 HuaweiCloud provider。
- DNS-01 应用原系统的递归 nameserver、`skipDNS` 和 CNAME 开关，并在单次签发结束后恢复进程全局配置。
- DNS 手动流程通过 `/api/v2/websites/ssl/resolve` 创建或复用 order，返回 `domain`、`resolve`、`value`、`err`；`/api/v2/websites/ssl/obtain` 校验 TXT 后 finalize。
- SSL 列表和详情返回 ACME/DNS 关联信息、网站关联和日志路径；私钥、EAB HMAC 和 DNS 凭据均不返回。
- ACME 注册失败不写入 SQLite；证书关联的 ACME/DNS 账户不可删除。

## 自动化证据

通过：

```bash
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
cd web && npm run type-check
git diff --check
WORKMESH_WEBSITE_EXTERNAL_TEST=1 GOWORK=off go test ./node/api -run '^TestExternalWebsiteLifecycle$' -count=1 -v
```

隔离 OpenResty 测试使用临时容器、临时网络和临时站点目录，测试结束无 `workmesh-acceptance-*` 残留资源。

## 仍为 not-run

- Let's Encrypt Staging/Production HTTP-01 真实签发、续期和公网 challenge 访问。
- DNS provider 真实 TXT 写入、传播、清理和手动 TXT finalize。
- 生产 OpenResty reload、生产数据库写入、生产证书签发和生产二进制替换。

原因：当前环境没有获得专用 ACME 邮箱、隔离域名维护窗口和可写 DNS provider 授权。不得用自签证书、固定响应或本地单元测试替代上述真实验收。

## 下一步

取得隔离域名和授权 DNS 账户后，先使用 Let's Encrypt Staging 执行 HTTP-01、DNS-01、手动 DNS 和失败回滚；完成证据记录后，再评估是否需要单独授权一次生产证书签发。
