<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 发布与回滚生产检查清单（2026-09-07）

状态：`[>] 待生产授权`。本文是发布负责人执行时的单页检查表；隔离演练通过不等于生产发布通过。所有勾选项必须附命令输出、时间戳和文件摘要，不能用静态路由清单或模拟数据代替。

## 0. 放行边界

- [ ] 维护窗口、回滚负责人和停止服务授权已记录。
- [ ] 候选二进制、版本号、构建提交和目标架构已冻结；前端无变化时不重新构建前端。
- [ ] 备份目录为独立目录：`/opt/workmesh-server/backups/upgrade-<UTC>`，不得覆盖历史备份。
- [ ] 生产替换只允许受控 root 运维流程执行；`workmesh-server update/restore` 的默认目标是数据目录内的 `releases/current.artifact`，不能误认为会替换 systemd 的 `/opt/workmesh-server/bin/workmesh-server`。

## 1. 候选制品验签

- [ ] 候选文件是普通文件（拒绝目录和符号链接），大小不超过 512 MiB，权限和属主符合发布策略。
- [ ] 记录 `sha256sum <candidate>`；发布单中的摘要必须与构建产物一致。
- [ ] 使用受信任的 Ed25519 公钥验证 `<candidate>.sig`。签名可覆盖 SHA-256 十六进制文本或其原始摘要字节，但验证结果必须明确记录为 `pass`。
- [ ] 验签失败、摘要不一致、签名/公钥解析失败时立即停止，不创建替换文件。
- [ ] 保存候选二进制、签名、公钥指纹和摘要到发布证据目录；日志不得包含私钥或完整会话凭据。

## 2. SQLite 备份与迁移前检查

- [ ] 停止服务前执行只读 `quick_check`、`integrity_check` 和 `foreign_key_check`，结果分别为 `ok`、`ok`、空集。
- [ ] 先停止 `workmesh-server.service`，确认进程已退出，再复制以下同一批次文件：
  - `data/workmesh.db`
  - `data/workmesh.db-wal`（存在时）
  - `data/workmesh.db-shm`（存在时）
  - `config/server.json`、`config/server.env`（存在时）
- [ ] 对备份文件执行 SHA-256，并记录文件大小、权限、属主和复制时间。WAL/SHM 不得遗漏，否则不能宣称可恢复到一致状态。
- [ ] 备份完成后保留原文件；禁止在未验证备份的情况下删除或覆盖生产数据库。

## 3. 隔离迁移幂等性

- [ ] 使用备份副本和候选二进制在独立临时目录启动两次，不监听生产端口、不访问生产 OpenResty/Docker。
- [ ] 第一次启动：所有目标迁移为 `applied`，`migration_runs.status='failed'` 为 0；新字段/索引存在，旧记录数量和关键值保持不变。
- [ ] 第二次启动：相同迁移全部为 `noop`，不得重复插入数据或改变 checksum；`artifact_sha256` 与候选摘要一致。
- [ ] 任一迁移失败即停止发布，保留副本、日志和 `migration_runs`，不得在失败库上反复重试。
- [ ] 迁移演练证据必须包含最终 `schema_migrations` 版本、失败计数、关键新字段和幂等 `noop` 记录。

## 4. 原子替换与启动验证

- [ ] 服务停止后，在二进制目标同一目录写入临时文件，完成写入、`fsync`、权限设置，再执行同目录 `rename`；不得直接截断目标文件。
- [ ] 旧二进制保存为带时间戳的 `.previous.<timestamp>`，并记录其 SHA-256；替换失败时临时文件清理且旧目标仍可启动。
- [ ] 启动服务后依次检查：`systemctl is-active`、`/health`、`/ready`、迁移审计、SQLite 三项完整性、`docker exec workmesh-openresty-waf nginx -t` 和关键站点 HTTP/HTTPS。
- [ ] 记录实际运行二进制 SHA-256，必须等于候选摘要；记录启动日志中无迁移失败、端口冲突或配置错误。
- [ ] 观察一个最小稳定窗口（建议 5 分钟）：无自动重启、就绪状态保持 200、关键站点无新增 5xx。

## 5. 回滚验证

触发条件：服务无法就绪、迁移失败、SQLite 完整性失败、OpenResty 检查失败、关键站点持续 5xx 或运行制品摘要不匹配。

- [ ] 保存失败服务日志、最近 `migration_runs`、当前二进制摘要和用户影响，再停止服务。
- [ ] 原子恢复同一备份批次的旧二进制、配置和环境文件；仅当新版本改变了数据库结构且旧版本不兼容时，才同时恢复匹配批次的 `workmesh.db`、WAL、SHM。
- [ ] 启动旧版本并验证 `/health`、`/ready`、OpenResty `nginx -t`、关键站点和 SQLite `quick_check`/`integrity_check`/外键检查。
- [ ] 确认运行制品摘要等于备份摘要、服务稳定窗口内无重启；保留失败版本和全部备份，未经签字不得清理。
- [ ] 在发布记录中填写触发原因、恢复文件摘要、迁移状态、恢复时间和用户影响；回滚验证未完成前不得重新尝试升级。

## 6. 证据与最终签字

发布单至少包含：候选版本和提交、SHA-256/Ed25519 验签、公钥指纹、SQLite/WAL/SHM 备份清单、迁移两次结果、替换前后摘要、健康与站点检查、回滚演练结果、执行人和时间戳。缺少任一项时状态保持 `blocked/not-run`，不能标记为正式上线。

实现依据：`cmd/workmesh-server/cli_artifact.go`（制品验签和数据目录内原子安装）、`internal/storage/migrations.go`（checksum、迁移账本和审计）、`cmd/workmesh-server/startup_rehearsal_test.go`（隔离双启动演练）。
