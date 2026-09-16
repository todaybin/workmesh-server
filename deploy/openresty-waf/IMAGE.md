# WorkMesh WAF 镜像

镜像基于 `openresty/openresty:1.27.1.2-3-bookworm`，运行时组合为 ModSecurity v3、OWASP CRS 4.14.0 和 OpenResty Lua。ModSecurity-nginx connector 按 OpenResty 对应的 Nginx `1.27.1` ABI 在镜像内编译，不能直接使用 Debian 的 Nginx `1.22` 模块。CRS 在 Docker 构建阶段按固定 tag 下载，构建末尾会执行 `nginx -t`；构建失败时不得发布镜像。网络受限节点可通过 `--build-arg DEBIAN_MIRROR=mirrors.aliyun.com` 指定 Debian 镜像源，默认仍为 `deb.debian.org`。

在线发布时应由 CI 生成多架构 manifest，并记录镜像 digest、SBOM 和签名。仓库内
提供的 `build-release.sh` 会在有 Docker daemon 的机器上执行静态门禁、真实构建、
镜像内 `nginx -t`、临时容器 smoke test，并生成 digest/归档清单：

```sh
cd deploy/openresty-waf
IMAGE_REF=registry.example.com/workmesh/openresty-waf:20260913 \
PLATFORM=linux/amd64 PUSH=1 SAVE=0 \
sh ./build-release.sh
```

需要离线镜像包时，在构建机执行：

```sh
IMAGE_REF=registry.example.com/workmesh/openresty-waf:20260913 \
PLATFORM=linux/amd64 PUSH=0 SAVE=1 \
sh ./build-release.sh
```

归档文件、SHA256 和 manifest 会写入 `deploy/openresty-waf/dist/`。该脚本不会把
digest 当作构建 tag；构建完成后应把 manifest 中的 digest 固化到生产 Compose。

手工 buildx 命令如下：

```sh
docker buildx build --platform linux/amd64,linux/arm64 \
  -t registry.example.com/workmesh/openresty-waf:1.0.0 --push .
docker buildx imagetools inspect registry.example.com/workmesh/openresty-waf:1.0.0
```

离线节点使用同一 digest 导出和导入：

```sh
docker pull registry.example.com/workmesh/openresty-waf:1.0.0
docker save registry.example.com/workmesh/openresty-waf:1.0.0 | gzip > workmesh-openresty-waf-1.0.0.tar.gz
gunzip -c workmesh-openresty-waf-1.0.0.tar.gz | docker load
```

将 digest 写入节点安装配置的 `WORKMESH_OPENRESTY_WAF_IMAGE`，不要使用 `latest`。生产 Compose 必须把宿主机 `/opt/workmesh-server/openresty-waf/waf` 挂载到容器 `/opt/workmesh/waf`；其中 `data`、`generated`、`logs` 分别由 Server 的 WAF 配置、生成配置和日志读取。Compose 挂载 OpenResty 全局配置数据、WAF 数据和 `${WEBSITE_DIR:-/www/wwwroot}` 站点目录；站点目录中的 `nginx/` 配置、`waf/` 规则和 `logs/` 日志由 Agent 与容器共享，不会覆盖镜像内的 WAF 主配置。入口脚本只在 `generated/*.conf` 缺失时写入默认值，避免空挂载目录导致 CRS 没有被加载，同时不会覆盖控制面已经保存的 WAF 设置。启动后先确认 `docker compose up -d` 的健康检查为 `healthy`，再把网站切换到“拦截”模式并执行 `tests/waf-regression.sh`。

`standardRules=false` 只应移除生成的 `standard-rules.conf` 中的 CRS include，不应通过 `SecRuleEngine Off` 关闭整个 ModSecurity 引擎。镜像构建阶段和入口脚本必须保留默认的 `standard-rules.conf`，以便首次启动即加载 CRS。控制面保存 ModSecurity 生成配置后必须执行 `/usr/local/openresty/nginx/sbin/nginx -t && /usr/local/openresty/nginx/sbin/nginx -s reload`，否则 Nginx worker 不会重新读取 `generated/*.conf`。

发布前执行 `tests/image-release-contract.sh`。该门禁拒绝 `latest` 和历史
`20260904` 镜像引用，检查 Dockerfile 构建期 `nginx -t`、Compose WAF 挂载和
`standard-rules.conf` include 链；门禁本身不执行 build/push 或容器变更。
