<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 升级、SQLite 迁移与回滚就绪（2026-09-07）

状态：`isolated-drill-passed`。本次未替换生产二进制、未重启
`workmesh-server`、未写入生产 SQLite，也未修改 `/www/apps/1Panel`。

## 隔离迁移演练证据（2026-09-07）

`cmd/workmesh-server/startup_rehearsal_test.go` 使用临时数据目录和两个独立
测试子进程调用正式启动存储阶段（`storage.Open`、统一领域迁移、运行时/日志
迁移、共享服务注入），并先通过现有 CLI 验签及原子安装临时候选制品。定向命令：

```text
GOWORK=off go test ./cmd/workmesh-server -run '^TestIsolatedMigrationRehearsal$' -count=1
ok   github.com/todaybin/workmesh-server/cmd/workmesh-server  0.098s
```

演练断言：临时 SQLite 共登记 13 个启动迁移；`0001-migration-metadata`、
`0012-runtime-task-state`、`0013-log-audit-v2` 和 `0015-website-template-relational`
各有且仅有一次 `applied` 与一次 `noop`；`migration_runs` 中 `failed=0`，第二次启动
记录的 `artifact_sha256` 与候选制品一致。演练不监听端口、不访问生产路径。

## 1. 当前真实状态

| 项目 | 观察结果 | 结论 |
| --- | --- | --- |
| systemd | `workmesh-server.service` active/running，`MainPID=290426`，工作目录 `/opt/workmesh-server` | 单进程入口稳定 |
| 当前制品 | `/opt/workmesh-server/bin/workmesh-server`，`26596130` bytes，SHA-256 `4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f` | 发布前必须重新记录候选制品哈希 |
| SQLite | `/opt/workmesh-server/data/workmesh.db`，`41328640` bytes，`0640 root:root`；WAL `894072` bytes，SHM `32768` bytes | 备份必须同时覆盖主库、WAL、SHM |
| SQLite 健康 | 只读 URI 检查 `quick_check=ok`、`integrity_check=ok`、`foreign_key_check` 为空 | 当前库可作为升级演练输入 |
| 迁移账本 | `schema_migrations` 15 条，最新 `0015-website-template-relational` | 当前代码目标为 0015 |
| 迁移审计 | 最近记录 `status=noop`，`artifact_sha256` 为当前进程哈希，`backup_path=/opt/workmesh-server/backups` | 启动审计存在；尚无本轮候选升级记录 |
| 备份目录 | `/opt/workmesh-server/backups/deploy-20260906T090558Z-website-template` 含旧二进制、SQLite/WAL、环境文件、OpenResty 配置归档 | 可作为回滚参考，不得覆盖原目录 |
| 发布脚本 | 仓库没有独立生产部署脚本；CLI `update/restore` 位于 `cmd/workmesh-server/cli_artifact.go` | 生产替换仍需受控运维脚本或人工命令 |

## 2. 代码入口与迁移顺序

启动入口为 `cmd/workmesh-server/main.go:runServer`：

1. `storage.Open(<WORKMESH_DATA_DIR>/workmesh.db)`：配置 WAL、外键和 busy timeout，并应用内置 `0001-migration-metadata`。
2. `nodeapi.SetSharedStore`：应用运行时 `0009-runtime-state-v2`、`0010-runtime-payload`、`0011-runtime-created-at`、`0012-runtime-task-state` 和日志 `0013-log-audit-v2`。
3. `initializeUnifiedSchema`：应用 `0002-legacy-payloads`、`0004-node-terminal-control-plane`、`0005-database-resources`、`0006-database-admin-metadata`、网站 `0006-website-relational`、`0014-website-default-html`、`0015-website-template-relational`。
4. 旧 JSON 仅作为首次导入输入；成功后归档到数据目录 `backups/legacy-*`。已有 SQLite 记录不会再次从 JSON 覆盖。

迁移由 `internal/storage/migrations.go:ApplyMigrations` 执行：每个 ID 保存 checksum，已应用迁移内容变化会阻止启动；每次 applied/noop/failed 都写入 `migration_runs`。制品哈希来自
`WORKMESH_ARTIFACT_SHA256`，未设置时读取当前可执行文件；备份根目录来自
`WORKMESH_MIGRATION_BACKUP_PATH`，未设置时为可执行文件上级的 `backups`。

## 3. 二进制更新边界

`cmd/workmesh-server/cli_artifact.go` 已实现：

- SHA-256 校验与 Ed25519 签名校验；拒绝符号链接、目录和超过 512 MiB 的制品。
- 临时文件写入、`fsync`、权限 `0750`、同目录 `rename` 原子替换。
- 旧制品保存为 `<target>.previous.<unix-ns>`，结果写入数据目录 `deployment-artifact.json`。

但该 CLI 有两个发布限制：

- `--target` 必须位于 `WORKMESH_DATA_DIR` 内，默认目标是 `data/releases/current.artifact`，不能直接替换 `/opt/workmesh-server/bin/workmesh-server`。
- `handleCLIArtifact` 安装后不会自动重启 systemd；`restartRunningService` 只由部分设置类 CLI 调用。

因此生产升级必须由具备 root 权限的受控流程完成“候选制品验签 → 停止服务 → 备份 → 原子替换 `/opt/workmesh-server/bin/workmesh-server` → 启动服务 → 健康检查”。不得把数据目录里的 `current.artifact` 误认为正在运行的 systemd 制品。

## 4. 隔离升级演练命令清单

以下命令是执行模板，变量必须由唯一集成负责人在隔离目录中显式赋值；本轮未执行。

```bash
set -eu
release_id="$(date -u +%Y%m%dT%H%M%SZ)"
source_bin="/path/to/workmesh-server-linux-amd64"
source_sig="${source_bin}.sig"
source_pub="/path/to/workmesh-update-ed25519.pub"
staging="/opt/workmesh-server/.staging/${release_id}"
backup="/opt/workmesh-server/backups/upgrade-${release_id}"
db="/opt/workmesh-server/data/workmesh.db"

install -d -m 0750 "$staging" "$backup"
sha256sum "$source_bin" | tee "$staging/candidate.sha256"
test "$(stat -c %s "$source_bin")" -le $((512*1024*1024))

# 只读校验现有数据库；不能在服务运行期间复制不一致的 WAL 状态。
python3 - "$db" <<'PY'
import sqlite3, sys
db = sys.argv[1]
con = sqlite3.connect("file:" + db + "?mode=ro", uri=True)
assert con.execute("PRAGMA quick_check").fetchone()[0] == "ok"
assert con.execute("PRAGMA integrity_check").fetchone()[0] == "ok"
assert con.execute("PRAGMA foreign_key_check").fetchall() == []
print("sqlite preflight: ok")
PY

# 停止后再复制主库/WAL/SHM，避免备份缺页；停止必须由人工批准。
systemctl stop workmesh-server.service
cp -a /opt/workmesh-server/bin/workmesh-server "$backup/workmesh-server.previous"
cp -a /opt/workmesh-server/config/server.json "$backup/server.json"
cp -a /opt/workmesh-server/config/server.env "$backup/server.env"
for sidecar in "$db-wal" "$db-shm"; do
  test ! -e "$sidecar" || cp -a "$sidecar" "$backup/"
done
cp -a "$db" "$backup/workmesh.db"
sha256sum "$backup/workmesh-server.previous" "$backup/workmesh.db" > "$backup/before.sha256"

# 隔离副本迁移：两次启动，第二次必须为 noop。使用临时端口，
# timeout 结束候选进程，不触碰 systemd 生产服务。
export WORKMESH_DATA_DIR="$staging/data"
install -d -m 0750 "$WORKMESH_DATA_DIR"
cp -a "$backup/workmesh.db" "$WORKMESH_DATA_DIR/workmesh.db"
candidate_sha="$(sha256sum "$source_bin" | awk '{print $1}')"
timeout --signal=TERM 8s env \
  WORKMESH_SERVER_ADDR=127.0.0.1:19999 \
  WORKMESH_DATA_DIR="$WORKMESH_DATA_DIR" \
  WORKMESH_ARTIFACT_SHA256="$candidate_sha" \
  WORKMESH_MIGRATION_BACKUP_PATH="$backup" \
  "$source_bin" >"$staging/first-start.log" 2>&1 || test "$?" -eq 124
timeout --signal=TERM 8s env \
  WORKMESH_SERVER_ADDR=127.0.0.1:19999 \
  WORKMESH_DATA_DIR="$WORKMESH_DATA_DIR" \
  WORKMESH_ARTIFACT_SHA256="$candidate_sha" \
  WORKMESH_MIGRATION_BACKUP_PATH="$backup" \
  "$source_bin" >"$staging/second-start.log" 2>&1 || test "$?" -eq 124
python3 - "$WORKMESH_DATA_DIR/workmesh.db" <<'PY'
import sqlite3, sys
con = sqlite3.connect(sys.argv[1])
ids = [r[0] for r in con.execute("select id from schema_migrations order by id")]
assert ids and ids[-1] == "0015-website-template-relational"
assert con.execute("select count(*) from migration_runs where status='failed'").fetchone()[0] == 0
assert con.execute("select count(*) from migration_runs where status='noop'").fetchone()[0] >= 1
print("migration rehearsal: ok", ids[-1])
PY
```

说明：候选进程应在日志中出现迁移完成并监听临时端口；可另开终端请求
`curl --fail http://127.0.0.1:19999/ready`，或在进程退出后以 SQLite
`migration_runs` 记录为准。不得把生产路径作为演练数据目录。

## 5. 生产替换与回滚清单（待授权）

### 升级

1. 冻结源码和前端，记录 Go 源码集合哈希、候选二进制 SHA-256、签名验证结果。
2. 确认备份目录位于 `/opt/workmesh-server/backups/upgrade-<UTC>`，并记录二进制、配置、SQLite/WAL/SHM 哈希。
3. 确认隔离副本迁移两次均成功，旧行数保留、旧字段仍在、`schema_migrations` 目标 ID 与代码一致。
4. 在维护窗口停止服务，复制备份后以同目录临时文件 `rename` 替换二进制；不要覆盖 `data/`、`config/`、`wwwroot/` 或 OpenResty 目录。
5. 启动服务并检查 `systemctl is-active`、`/health`、`/ready`、`migration_runs`、SQLite 三项完整性、OpenResty `nginx -t` 和现有站点 HTTP/HTTPS。
6. 保留旧制品和备份至少一个发布周期；发布记录必须包含实际运行制品哈希和迁移审计 `artifact_sha256`。

### 回滚

1. 触发条件：服务无法就绪、迁移失败、关键站点 5xx、SQLite 完整性失败或 OpenResty 检查失败。
2. 停止服务并保存失败日志、`migration_runs` 最新记录和当前制品哈希；禁止在失败库上继续重试写迁移。
3. 原子恢复 `workmesh-server.previous`、`server.json`、`server.env`；数据库仅在确认升级迁移已改变结构且新版本无法兼容时，才恢复同一备份批次的 `workmesh.db` 及 WAL/SHM。
4. 启动旧制品，确认 `/health`、`/ready`、OpenResty 和关键网站恢复；执行 SQLite `quick_check`、`integrity_check`、外键检查。
5. 将回滚原因、恢复文件哈希、服务日志和用户影响写入发布记录。未经验证不得删除新库或旧备份。

## 6. 当前阻塞项

- **候选制品与签名材料未指定**：缺少正式发布二进制、SHA-256、Ed25519 签名和公钥路径，不能执行验签升级。
- **无生产发布脚本**：仓库只有受限于数据目录的 CLI 原子安装；替换 systemd `/opt/workmesh-server/bin` 需另行编写并审核 root 运维脚本。
- **未完成隔离迁移演练**：当前只有生产库只读证据，没有新候选制品在独立 SQLite 副本上的“两次启动/一次迁移、一次 noop”证据。
- **回滚窗口和授权未确认**：停止服务、替换二进制、恢复 SQLite/WAL/SHM 均会改变生产状态，须由发布负责人批准维护窗口。
- **SQLite CLI 不可用**：当前环境没有 `sqlite3` 命令，本盘点使用 Python `sqlite3` 只读 URI；发布主机仍应安装并记录 SQLite 工具版本。
- **新字段迁移证据不足**：目前代码已有版本迁移和 checksum 保护，但尚无针对下一候选版本的新增字段、旧数据保留和降级兼容报告。

在上述阻塞解除前，升级/回滚只能标记 `not-run`，不得宣称正式发布完成。该文档仅提供命令级执行模板和证据要求。
