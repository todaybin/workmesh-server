<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 网站高级设置与隔离黑盒验收（2026-09-08）

## 已完成

- 路由缺口扫描器修正了 `/:param`、`operate:param`、循环注册、统一分发器和 `DOWNLOAD/UPLOAD`（前端实际 POST）误报；当前 websites 静态状态为 81 条 `not-run`、0 条 501，表示已找到处理器但仍需真实 HTTP 证据。
- 网站设置同次更新会按待写文件重建托管 include，首次启用/禁用重定向、代理、CORS、真实 IP 等配置即可生效。
- 网站删除会校验目录边界和子站点依赖，并清理完整隔离站点目录；SQLite 网站、域名、设置和 WAF 记录同步删除。
- Basic Auth 密码不再写入 SQLite；仅保存 `hasPassword`，哈希文件以运行时 worker 可读权限生成。
- 反向代理默认写入 WebSocket Upgrade、连接/读写超时头。

## 隔离黑盒结果

显式命令：

```bash
WORKMESH_WEBSITE_EXTERNAL_TEST=1 GOWORK=off \
  go test ./node/api -run '^TestExternalWebsiteLifecycle$' -count=1 -v
```

通过。测试使用临时 `workmesh-acceptance-*` OpenResty 容器、Python 上游容器、Docker 网络、随机回环端口和 `*.cs.sopvip.com` 主机名；验证静态站、反代、CORS、真实 IP、重定向、Basic Auth、证书绑定 HTTPS、`openresty -t`、reload、非法配置回滚和删除清理。测试结束无残留临时容器或网络。

第一次尝试因 Docker Hub `nginx:alpine` OCI 描述符拉取失败，未进入业务步骤；随后改用本机已有镜像重跑通过。

## 仍未完成

- 尚未在真实隔离域名上执行 ACME HTTP-01 签发/续期；DNS、证书账户和维护窗口仍需授权。
- 尚未对现有生产 OpenResty 执行 reload、生产数据库写入、业务容器操作或生产二进制替换。
- TCP/UDP、运行时站点、WAF 攻防高级规则和跨节点网站操作仍按 `not-run`/后续批次处理。
