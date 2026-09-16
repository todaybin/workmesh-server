<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# ACME HTTP-01 续期与 HTTPS 安装闭环

更新时间：2026-09-06（Asia/Shanghai）

## 本轮完成

- `StartBackgroundTasks` 在服务启动后立即执行一次证书维护扫描，之后每 6 小时扫描一次；服务退出由 `context.Context` 取消，单进程内不会重复启动扫描器。
- `SSLService.RenewDue` 扫描前先处理两类状态：
  - `certbot.timer` 或管理员在服务进程外续期成功的 `live/<domain>` 证书；
  - 本服务之前签发成功但站点文件或 OpenResty reload 失败的证书。
- `SyncCertbotLiveCertificates` 只读取 `WORKMESH_LETSENCRYPT_DIR`（默认 `/etc/letsencrypt`）下真实的 certbot `live` 文件，允许 certbot 标准的 `live -> archive` 符号链接，但拒绝最终路径越过证书根目录。
- 读取的证书必须通过 PEM、有效期、域名覆盖和证书/私钥公钥匹配校验；通过后先写入 `website_ssls` SQLite，再同步到引用站点的 `ssl/fullchain.pem` 和 `ssl/privkey.pem`。
- 同步证书前备份每个站点的旧证书文件；写入失败或 OpenResty reload 失败时恢复全部已修改文件，并尝试再次 reload 旧证书。
- 同步失败不会把已真实签发的证书伪装成签发失败，SQLite 保留 `status=ready`，并把同步失败原因写入 `message`；下一个后台周期会自动重试。
- 同步成功后 SQLite `message` 记录“证书已同步，站点 HTTPS 配置已 reload”，便于面板查看实际状态。
- 新增 `ssl_sync_test.go` 专项测试，覆盖 certbot 合法/越界符号链接、文件回滚和 SQLite ready 状态重试幂等性；测试交由唯一集成测试负责人执行。

## 现有 systemd / certbot 关系

- `workmesh-server.service` 负责启动 WorkMesh 进程，服务内部后台扫描器负责 SQLite 状态、站点证书文件和 OpenResty reload 的闭环。
- 宿主机存在 `certbot.timer`。它可以独立更新 `/etc/letsencrypt/live`；WorkMesh 每 6 小时读取并接管新证书，因此不依赖修改 certbot systemd 单元或安装额外 shell hook。
- 发布后只读核对：`certbot.timer=active`，下一次计划触发时间为 2026-09-06 17:10:01 CST；`workmesh-server.service` 当前 `MainPID=2602114`、`NRestarts=0`。
- 两个入口均使用同一真实 SQLite 和同一证书同步函数，避免“certbot 已续期但网站仍加载旧证书”。

## 待唯一集成测试负责人验证

- 使用真实 staging/测试证书验证 `certbot.timer` 更新后 SQLite `expire_date`、证书 PEM、站点证书文件和 OpenResty reload 一致。
- 人为制造 OpenResty reload 失败，确认站点旧证书文件恢复、SQLite `status=ready` 与 `message` 错误记录保留，下一周期可恢复。
- 人为制造证书目录符号链接越界，确认同步被拒绝且不写入站点根目录外文件。
- 验证服务重启后后台扫描器只启动一个实例，且不会对 stopped 网站重新启用 HTTPS。

本轮 ACME 代码已由统一门禁验证并随迁移审计修复制品部署；未重新编译前端。发布后 `workmesh-server.service`、网站 HTTP/HTTPS、OpenResty 和 SQLite 完整性均通过只读核对。
