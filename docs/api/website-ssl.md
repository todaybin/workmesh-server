# 网站 SSL API 迁移

旧 Agent 的 `website_ssl.go` 共 13 条路由，本项目已在 `node/api/ssl.go` 注册相同路径：搜索、列表、创建、申请、DNS 解析、删除、按网站/编号查询、更新、节点推送、JSON/文件上传、下载和主节点导入。

`node/service/ssl.go` 提供证书元数据服务：创建申请配置、按域名查询、PEM X.509 解析、过期检查、更新和删除。证书元数据、ACME 账户、CA 和签发记录统一写入 SQLite；证书私钥只保存在服务内部，模型 JSON 序列化明确使用 `json:"-"`，下载接口仅返回证书 PEM。导入证书时会校验证书未过期、主域名格式；传入 PEM 私钥时还会校验私钥格式及证书/私钥公钥匹配。

## Let's Encrypt HTTP-01

当 `provider` 为 `http` 或 `letsencrypt` 时，`POST /api/v2/websites/ssl/obtain` 启动真实的 certbot webroot 流程。接口只负责将记录置为 `applying` 并返回申请状态，签发在后台执行；成功后把 `fullchain.pem`、`privkey.pem` 的真实内容和证书有效期写回 `website_ssls`，失败则把状态写为 `error` 并保存错误消息，避免记录永久停留在 `applying`。

申请前必须满足以下条件：

- 主域名和附加域名为合法 DNS 名称；HTTP-01 拒绝通配符、IP 地址和路径字符。
- 站点记录存在，实际 webroot 为站点目录下的 `app`；`.well-known/acme-challenge` 会被创建，且不能是符号链接。
- 若请求携带 `acmeAccountId`，该账户必须真实存在；其邮箱用于 certbot 注册参数。
- OpenResty 站点配置必须把 `/.well-known/acme-challenge/` 映射到上述 webroot，且公网 HTTP 80 端口和 DNS 已经指向本机。应用不会伪造 challenge 或签发结果。

证书目录默认使用 `/etc/letsencrypt`，可通过后端进程环境变量 `WORKMESH_LETSENCRYPT_DIR` 指定；certbot 可通过 `WORKMESH_CERTBOT_BIN` 指定。续期扫描只对启用自动续期且即将到期的 HTTP-01 证书启动同一真实流程。证书输出会再次校验覆盖全部申请域名后才写入 SQLite。

当前接口仍保持 1Panel 的同步响应形状（`code=200`、`data.id`、`data.status=applying`）；真正的完成结果通过后续 SSL 查询接口读取。DNS 挑战查询、节点推送和非 HTTP-01 DNS provider 不会伪造成功，未配置真实执行器时返回明确错误。
