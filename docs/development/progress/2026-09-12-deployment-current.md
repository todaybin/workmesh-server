<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-09-12 当前部署现场

## 已确认

- 主项目仍为 `/www/apps/workmesh-server`，前后端和部署资产均在该目录。
- 2026-09-13 已确认线上旧页面来自旧运行制品：`/opt/workmesh-server/bin/workmesh-server` 的 SHA-256 为 `4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f`，`/opt/workmesh-server/web/dist` 仍包含旧 `waf-tabs` bundle。
- 当前仓库已通过 `make build-release` 生成单二进制发布包 `/www/apps/workmesh-server/release/workmesh-server-linux-amd64`，SHA-256 为 `655207b820fa7e4c74ad1f10275ec428033c02dbfbfb60b9d24ef22b7e1c043a`；包内包含新版 WAF 七路由。
- 新增 `deploy/install/activate-release.sh`，目标机执行时会校验 `.sha256`、备份并原子替换二进制，检查 systemd ExecStart、MainPID、9999 端口归属和新版 WAF chunk；失败时尝试回滚。
- 建议隔离验收域名为 `workmesh.cs.sopvip.com`。
- `/etc/network/interfaces` 中当前主机 IPv4 配置为 `61.184.12.165`。
- 新增 `deploy/acceptance/host-ip.sh`，在 netlink 不可用时可从静态 Debian/Ubuntu 网络配置读取非回环 IPv4。
- 新增 `deploy/install/domain-plan.sh`，只读输出 DNS A 记录和域名验收命令。
- `deploy/install/prepare-runtime.sh --apply --domain workmesh.cs.sopvip.com` 会准备数据库/WAF 运行目录，并生成：
  - `WORKMESH_SERVER_ADDR=0.0.0.0:9999`
  - `WORKMESH_DATA_DIR=<运行数据目录>`
  - `WORKMESH_PUBLIC_URL=https://workmesh.cs.sopvip.com`
- 准备脚本保留已有 secret、Gateway 凭据、WAF JSON 和生成配置；不会启动或删除容器。

## 当前实际阻塞

- Docker CLI 为 `29.7.2`，但 `/var/run/docker.sock` 访问被拒绝，无法执行真实容器/镜像/Compose 操作。
- `/opt/workmesh-server/openresty-waf/docker-compose.yml` 仍引用
  `workmesh/openresty-waf:20260904`；严格预检已将该引用判定为旧/不可发布镜像。
- `/opt/workmesh-server/openresty-waf/waf/generated/standard-rules.conf` 缺失。
- `workmesh.cs.sopvip.com` 当前无法在受限网络命名空间解析；不能据此断定域名服务商配置失败，需在真实网络主机执行 `dig +short A`。
- 当前命名空间禁止监听 loopback，无法通过本地 HTTP 启动测试证明服务可访问。
- 当前执行环境无法写入 `/opt/workmesh-server`（根和 `/www` 挂载为只读），无法访问 systemd 总线；本次没有替换生产二进制、重启服务或声称公网已更新。
- 当前主机命令探测结果：Go、psql、redis-server、redis-cli、certbot 存在；PHP、php-fpm、MySQL/MariaDB、OpenResty/nginx、Docker daemon 不可用。

## 已验证命令

```bash
deploy/install/domain-plan.sh
deploy/install/domain-plan.sh --self-test
deploy/install/prepare-runtime.sh --self-test
bash deploy/acceptance/domain-acceptance.sh --self-test
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go test ./deploy/acceptance ./deploy/database -count=1
GOWORK=off GOCACHE=/tmp/workmesh-go-cache go vet ./...
bash deploy/openresty-waf/tests/runtime-contract.sh
IMAGE_REF=registry.example.com/workmesh/openresty-waf:20260912 \
  bash deploy/openresty-waf/tests/image-release-contract.sh
```

以上本地可执行门禁通过。生产预检当前应失败，原因是 Docker daemon、旧 WAF 镜像、
缺失生成配置和域名/浏览器验收条件未满足；不能将静态门禁结果写成上线完成。

## 真实部署主机下一步

1. 配置 DNS：`workmesh IN A 61.184.12.165`，再从公网执行 `dig +short A workmesh.cs.sopvip.com`。
2. 在维护窗口执行 `deploy/install/prepare-runtime.sh --apply --force --domain workmesh.cs.sopvip.com`。
3. 备份并核对旧 WorkMesh 资源，禁止触碰 `WorkMesh-postgresql-ZNMP` 和 `WorkMesh-redis-ZNMP`。
4. 用新 Dockerfile 构建并固定 WAF 镜像 digest，替换旧 `20260904` 引用。
5. 执行数据库 Compose 黑盒验收，启动 PostgreSQL、Redis、MariaDB 并验证健康、备份、恢复。
6. 启动 WorkMesh Server，验证 `/health`、`/ready`、登录和前端静态资源。
7. 在真实宿主机执行 `deploy/install/activate-release.sh` 完成二进制实际切换，再检查服务 MainPID 和公网资源。
8. 创建独立测试站点，生成 `site.conf`，执行 OpenResty `nginx -t`、reload、HTTPS、WAF observe/block 请求矩阵。
9. 完成 SQLite 与 `/www/wwwroot` 19 个孤立 `site.conf` 的人工分类后，再决定导入、重建或清理。
10. 安装浏览器自动化依赖，完成 `docs/img/` 对应七个 WAF 页面的截图、交互和保存后 `effective` 验收。
