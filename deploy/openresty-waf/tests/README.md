# WAF 回归测试

镜像启动并将目标网站切换为“拦截”后执行：

```sh
BASE_URL=https://example.com sh tests/waf-regression.sh
```

测试覆盖正常请求、SQL 注入、XSS、目录穿越、命令注入、PHP 注入、扫描器 User-Agent、TRACE 方法以及 XSS/命令注入请求体。TRACE 在 Nginx 默认配置下可能返回 405，属于已拒绝状态。默认 User-Agent 是 `workmesh-waf-regression`，脚本会单独使用 `sqlmap/1.8` 验证扫描器规则。在“观察”模式下攻击请求应返回业务状态码，同时可在 ModSecurity 审计日志中看到命中记录；切换为“拦截”后攻击样例应返回 403。

站点运行配置位于挂载的 `/www/wwwroot/<domain>/waf/config.json` 和 `rules.json`，全局自定义规则位于 `/opt/workmesh/waf/data/custom-rules.json`。Lua 按文件修改时间和大小失效缓存；修改配置后无需重启 worker。访问、攻击、URL 和 404 频率限制使用 `rateLimits`（或兼容的 `frequencyLimit`）配置，命中事件写入 `/opt/workmesh/waf/logs/workmesh-custom-audit.jsonl`。

## 运行时契约

```sh
sh tests/runtime-contract.sh
```

该脚本验证 Compose 会把宿主机 `./waf` 挂载到容器 `/opt/workmesh/waf`，OpenResty 通过 `/etc/workmesh-waf/modsecurity.conf` 加载 `generated/modsecurity-mode.conf`、`custom-rules.conf`、`standard-rules.conf` 和 `global-rules.conf`，并验证入口脚本在空挂载目录首次启动时补齐默认生成配置、但不会覆盖控制面已经保存的配置。ModSecurity 生成配置变更后仍需要由控制面执行 `/usr/local/openresty/nginx/sbin/nginx -t && /usr/local/openresty/nginx/sbin/nginx -s reload` 才能进入 Nginx worker；站点 JSON 配置由 Lua 按文件修改时间读取，不需要 reload。

## 镜像发布门禁

发布前先执行静态门禁。它不会 build、push、pull、启动或修改容器，只检查候选镜像引用和镜像/Compose/runtime 契约：

```sh
IMAGE_REF=registry.example.com/workmesh/openresty-waf:20260912 \
sh tests/image-release-contract.sh
```

`IMAGE_REF` 必须使用明确的非 `latest` tag 或 immutable digest，并且不能继续指向历史
`20260904` 镜像。真实 build、`nginx -t`、push、签名、digest 固化和部署仍需在有
Docker daemon、镜像仓库和维护窗口的环境执行。

## 域名与容器只读验收

拿到目标环境权限后，执行以下命令。脚本默认使用
`workmesh.cs.sopvip.com` / `61.184.12.165`，也可以通过 `DOMAIN`、`ADDRESS`
覆盖。脚本使用 `curl --resolve` 验证 Host 路由，不修改 DNS；默认允许测试证书，
生产证书验收应设置 `TLS_VERIFY=1`。

```sh
DOMAIN=workmesh.cs.sopvip.com \
ADDRESS=61.184.12.165 \
TLS_VERIFY=1 \
CHECK_CONTAINER=1 \
CHECK_LOGS=1 \
WAF_LOG_DIR=/opt/workmesh-server/openresty-waf/waf/logs \
sh tests/domain-acceptance.sh
```

默认检查：

- HTTP Host 路由返回 `200/301/302/307/308`；
- HTTPS Host 路由返回 `200`；
- SQL 注入样例在拦截模式下返回 `403`；
- 容器内 `/usr/local/openresty/nginx/sbin/nginx -t` 通过，且 Compose 健康状态为 `healthy`；
- `access.log`、`modsecurity-audit.json` 和 `workmesh-custom-audit.jsonl` 存在。

如果站点使用不同的页面、攻击样例或 HTTP 状态，可通过 `HTTP_PATH`、`HTTPS_PATH`、`ATTACK_PATH`、`EXPECTED_*` 环境变量调整。脚本只读检查，不执行 reload、证书签发、DNS 写入、容器重启或清理。
