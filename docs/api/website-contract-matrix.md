# 网站模块 HTTP/WS 契约矩阵

> 梳理日期：2026-09-06  
> 参考来源：`/www/apps/1Panel/frontend/src/routers/modules/website.ts`、`src/api/modules/website.ts`、`src/api/interface/website.ts` 及网站页面调用点。  
> WorkMesh 实现：`node/api/website_*.go`、`node/service/website_*.go`、`node/api/runtime_toolbox_routes.go`。  
> 本文是接口契约和验收清单，不是模拟数据；本次静态梳理没有向业务服务发送请求，因此业务状态统一为 `not-run`，不能据此宣称上线通过。

## 1. 范围和状态定义

网站模块必须保持 1Panel 前端的字段名称、HTTP 方法、响应包络和同路径多操作语义。正式调用前缀统一为 `/api/v2`；参考代码中省略的 `/api/v2` 在下表中已补全。旧 `/api/v1` 只作为迁移历史，不得由新前端调用。

状态含义：

| 状态 | 含义 |
| --- | --- |
| `explicit-real-handler` | WorkMesh 有明确的业务处理器，仍需黑盒请求验证 |
| `wildcard-real-handler` | 由 `website_extensions.go` 的真实分发器按路径分支处理，仍需黑盒验证 |
| `fallback-present` | 当前还有 `fallbackRouteHandler` 注册；不能视为功能完成，必须补专用处理器或确认注册顺序 |
| `blocked` | 需要 Docker、OpenResty、ACME、DNS、外部上游、SSH 或有效登录会话等当前环境未提供的依赖 |
| `not-run` | 已静态发现但本次未发送真实请求 |

全局前端扫描（`docs/inventory/frontend-http-ws-contract-matrix.md`）统计网站菜单引用 130 条，其中 HTTP 130 条；网站 API 文件有 106 个导出函数。菜单引用数包含同一共享接口被多个页面复用，不能当作唯一路径数。

## 2. 菜单和页面树

来源为 `frontend/src/routers/modules/website.ts`：

| 菜单/页面 | 路由 | 主要操作 | 权限 |
| --- | --- | --- | --- |
| 网站列表 | `/websites` | 搜索、创建、启停、重启、删除、批量操作、分组、预检查 | `website_view` |
| 网站详情 | `/websites/:id/config/:tab` | 基础信息、目录、默认文档、伪静态、HTTPS、PHP、代理、重定向、认证、日志等 | `website_view` |
| SSL 证书 | `/websites/ssl` | ACME、证书申请/上传/续期/推送/下载、CA | `website_cert_view` |
| 网站模板 | `/websites/templates` | 模板搜索、创建、编辑、删除、上传、预览、产物 | `website_view` |
| PHP 运行环境 | `/websites/runtimes/php` | 运行时生命周期、配置、扩展、FPM、终端 | `website_runtime_view` |
| Node 运行环境 | `/websites/runtimes/node` | 运行时生命周期、包和终端 | `website_runtime_view` |
| Java 运行环境 | `/websites/runtimes/java` | 运行时生命周期、终端 | `website_runtime_view` |
| Go 运行环境 | `/websites/runtimes/go` | 运行时生命周期、终端 | `website_runtime_view` |
| Python 运行环境 | `/websites/runtimes/python` | 运行时生命周期、终端 | `website_runtime_view` |
| .NET 运行环境 | `/websites/runtimes/dotnet` | 运行时生命周期、终端 | `website_runtime_view` |

详情页还包含：基础信息、站点目录、默认文档、伪静态、HTTPS、PHP、反向代理、负载均衡、限流、认证、路径认证、防盗链、真实 IP、CORS、资源、数据库、TCP/UDP Stream、日志、Nginx/OpenResty 配置和模块。

## 3. 公共请求和响应约定

### 3.1 请求

- JSON 请求使用 `Content-Type: application/json`；模板和证书文件上传使用 `multipart/form-data`。
- 所有写操作必须携带登录会话或 `X-WorkMesh-Token`，并经过角色权限检查；不能用静态 token 绕过鉴权。
- 分页请求沿用 `{ page, pageSize, keyword?, ... }`。分页响应为 `{ code: 200, data: { total, items, page, pageSize } }`。
- 普通成功响应为 `{ code: 200, data: ... }`；失败响应必须含非 2xx HTTP 状态和可读错误信息，前端不得把错误当作空成功数据。
- 创建网站时前端会复制请求并对 `ftpPassword` 执行 `encodeBase64Fields`；服务端必须兼容该编码，不得记录明文密码。
- `searchSSL`、`pushSSLToNode` 支持 `CurrentNode` 请求头；这是节点路由选择，不是 JSON 字段。

### 3.2 响应形状

| 类型 | 形状 | 典型调用 |
| --- | --- | --- |
| 分页 | `{ code, data: { total, items, page, pageSize } }` | 网站搜索、DNS/ACME/SSL/模板搜索 |
| 数组 | `{ code, data: T[] }` | 网站列表、域名、代理、LBS、资源 |
| 对象 | `{ code, data: T }` | 网站详情、配置、HTTPS、证书 |
| 操作结果 | `{ code, data: ... }` | 创建、更新、删除、启停 |
| 文件下载 | HTTP body 为 Blob，不能按 JSON 解码 | SSL/CA 下载 |
| 文件上传 | multipart，返回文件元信息 | SSL 文件、模板 ZIP |

## 4. 网站类型和生命周期

### 4.1 创建类型

`createWebsite` 使用 `POST /api/v2/websites`，超时 10 分钟。`type` 是决定部署方案的关键字段，不能把所有类型合并成一个静态站点。

| `type` | 典型字段 | 业务含义 | 验收依赖 |
| --- | --- | --- | --- |
| `deployment` | `appType`, `appInstallId`, `templateOutputID`, `taskID` | 应用商店/模板一键部署 | 应用包、任务执行器 |
| `runtime` | `runtimeID`, `runtimeType`（php/node/java/go/python/dotnet） | 绑定运行环境的网站 | 对应容器/运行时 |
| `static` | `siteDir`, `domains` | 静态站点 | OpenResty 文件目录 |
| `proxy` | `proxyType`, `proxyAddress`, `proxyProtocol` | 反向代理 | 可访问真实上游 |
| `subsite` | `parentWebsiteID` | 主站点下的次站点 | 已存在主站点、域名隔离 |
| `stream` | `streamPorts`, `udp`, `algorithm`, `servers` | TCP/UDP Stream | 真实 TCP/UDP 上游 |

完整创建字段：`primaryDomain`, `type`, `alias`, `remark`, `appType`, `appInstallId`, `webSiteGroupId`, `proxy`, `proxyType`, `proxyAddress`, `proxyProtocol`, `runtimeID`, `runtimeType`, `domains`, `parentWebsiteID`, `siteDir`, `streamPorts`, `udp`, `algorithm`, `servers`, `enableSSL`, `websiteSSLID`, `createDB`, `dbName`, `dbPassword`, `dbFormat`, `dbUser`, `dbHost`, `ftpUser`, `ftpPassword`, `templateOutputID`, `taskID`。未提供的可选字段必须按原前端默认值处理，不能写入虚假上游或 JSON 模拟记录。

### 4.2 生命周期和批量操作

| 前端函数 | 方法和路径 | 请求关键字段/变体 | 响应 | WorkMesh | 状态 |
| --- | --- | --- | --- | --- | --- |
| `searchWebsites` | POST `/api/v2/websites/search` | 分页、关键字、筛选；可带 `?operateNode=` | 分页 `WebsiteRes` | `website_crud.go` | `explicit-real-handler` / `not-run` |
| `listWebsites` | GET `/api/v2/websites/list` | 无 | `WebsiteDTO[]` | `website_crud.go` | `explicit-real-handler` / `not-run` |
| `createWebsite` | POST `/api/v2/websites` | 第 4.1 节字段 | 操作结果/网站任务 | `website_crud.go`, `website_lifecycle.go` | `explicit-real-handler` / `not-run` |
| `opWebsite` | POST `/api/v2/websites/operate` | `{ id, operate }`；`start`, `stop`, `restart`，可带 `operateNode` | 操作结果 | `website_crud.go`, `website_lifecycle.go` | `explicit-real-handler` / `not-run` |
| `updateWebsite` | POST `/api/v2/websites/update` | 网站 ID 和可更新字段；可带 `operateNode` | 操作结果 | `website_crud.go` | `explicit-real-handler` / `not-run` |
| `getWebsite` | GET `/api/v2/websites/{id}` | 路径 ID | `WebsiteDTO` | `website_crud.go` | `explicit-real-handler` / `not-run` |
| `getWebsiteOptions` | POST `/api/v2/websites/options` | `{ websiteID?, type?, ... }` | `WebsiteOption[]` | `website_extensions.go` | `wildcard-real-handler` / `not-run` |
| `getWebsiteConfig` | GET `/api/v2/websites/{id}/config/{type}` | `type` 常用 `basic`, `openresty` 或具体配置名 | `File`/配置对象 | `website_config_routes.go` | `explicit-real-handler` / `not-run` |
| `deleteWebsite` | POST `/api/v2/websites/del` | `{ id, ... }` | 操作结果 | `website_crud.go` | `explicit-real-handler` / `not-run` |
| `preCheck` | POST `/api/v2/websites/check` | 创建前域名、目录、端口等 | `CheckRes[]` | `website_crud.go`/扩展路由 | `explicit-real-handler` / `not-run` |
| `batchOperate` | POST `/api/v2/websites/batch/operate` | `{ ids: number[], operate: string, taskID: string }` | 操作结果/任务 | `website_extensions.go` 或 fallback | `fallback-present` |
| `batchSetGroup` | POST `/api/v2/websites/batch/group` | `{ ids, groupID/webSiteGroupId }` | 操作结果 | 扩展/兼容路由 | `fallback-present` |
| `batchSetHttps` | POST `/api/v2/websites/batch/ssl` | `{ ids, ...HTTPSReq }` | 操作结果 | 扩展/兼容路由 | `fallback-present` |
| — | POST `/api/v2/websites/group/change` | `{ id, groupID }` | 操作结果 | legacy 路由存在 | `fallback-present` |

`operate` 是同一路径的核心变体，测试必须分别覆盖 `start`、`stop`、`restart`，并确认 SQLite 网站状态、OpenResty 配置和操作日志三者一致。删除必须验证目录、配置、关联域名/证书引用和任务记录的清理策略。

## 5. 统一配置入口：`scope` + `operate`

`getNginxConfig` 和 `updateNginxConfig` 是前端多个页面共用的接口，不能只按 URL 判断业务。请求至少包含 `{ websiteId, scope, operate, params? }`。

| `scope` | `operate` | `params` 形状/用途 | 页面 |
| --- | --- | --- | --- |
| `index` | `get/update` | 默认文档数组或配置对象 | 默认文档 |
| `limit-conn` | `get/update/add/delete` | `[{ limit_conn: "perserver 300" }, { limit_conn: "perip 25" }, { limit_rate: "512k" }]` | 限流 |
| `http-per` | `get/update` | 性能参数对象 | Nginx 性能 |
| `path` | `get/update` | 路径和根目录 | 站点目录 |
| `root` | `get/update` | 站点根目录 | 站点目录 |

| 前端函数 | 方法和路径 | 响应 | WorkMesh | 状态 |
| --- | --- | --- | --- | --- |
| `getNginxConfig` | POST `/api/v2/websites/config` | `NginxScopeConfig` | `website_config_routes.go` → `configHandler` | `explicit-real-handler` / `not-run` |
| `updateNginxConfig` | POST `/api/v2/websites/config/update` | 操作结果 | 同上 | `explicit-real-handler` / `not-run` |
| `updateNginxFile` | POST `/api/v2/websites/nginx/update` | 操作结果 | `website_config_write.go` | `explicit-real-handler` / `not-run` |
| `changeDefaultServer` | POST `/api/v2/websites/default/server` | 操作结果 | `website_config_routes.go` | `explicit-real-handler` / `not-run` |
| `getDefaultHtml` | GET `/api/v2/websites/default/html/{type}` | `WebsiteHtml` | `website_extensions.go` | `wildcard-real-handler` / `not-run` |
| `updateDefaultHtml` | POST `/api/v2/websites/default/html/update` | 操作结果 | `website_extensions.go` | `wildcard-real-handler` / `not-run` |

## 6. 域名、目录和伪静态

| 前端函数 | 方法和路径 | 关键请求字段/变体 | 响应 | WorkMesh | 状态 |
| --- | --- | --- | --- | --- | --- |
| `listDomains` | GET `/api/v2/websites/domains/{websiteId}` | 路径站点 ID | `Domain[]` | `website_domain.go` | `explicit-real-handler` / `not-run` |
| `createDomain` | POST `/api/v2/websites/domains` | `{ websiteID, domains: [{ domain, port, ssl }] }` | 操作结果 | `website_domain.go`, `website_domains_config.go` | `explicit-real-handler` / `not-run` |
| `updateDomain` | POST `/api/v2/websites/domains/update` | `{ id, ssl }` | 操作结果 | 同上 | `explicit-real-handler` / `not-run` |
| `deleteDomain` | POST `/api/v2/websites/domains/del/` | `{ id }` | 操作结果 | 同上 | `explicit-real-handler` / `not-run` |
| `getDirConfig` | POST `/api/v2/websites/dir` | 网站 ID、目录配置参数 | `DirConfig` | `website_config_routes.go` | `explicit-real-handler` / `not-run` |
| `updateWebsiteDir` | POST `/api/v2/websites/dir/update` | 目录字段 | 操作结果 | 同上 | `explicit-real-handler` / `not-run` |
| `updateWebsiteDirPermission` | POST `/api/v2/websites/dir/permission` | 目录、用户、权限 | 操作结果 | 同上 | `explicit-real-handler` / `not-run` |
| `getRewriteConfig` | POST `/api/v2/websites/rewrite` | `{ websiteID, name }` | `RewriteRes` | `website_rewrite.go` | `explicit-real-handler` / `not-run` |
| `updateRewriteConfig` | POST `/api/v2/websites/rewrite/update` | `{ websiteID, name, content }` | 操作结果 | `website_rewrite.go` | `explicit-real-handler` / `not-run` |
| `listCustomRewrite` | GET `/api/v2/websites/rewrite/custom` | 可选 `?websiteID=` | `string[]` 或配置 | `website_config_routes.go` | `explicit-real-handler` / `not-run` |
| `operateCustomRewrite` | POST `/api/v2/websites/rewrite/custom` | `{ operate: create/delete, name, content }`（也兼容 `websiteID`, `config`） | 配置/操作结果 | `website_config_routes.go` | `explicit-real-handler` / `not-run` |

主域名不能删除。域名变更验收必须同时检查 SQLite 域名记录、生成的 `site.conf` 的 `server_name`、OpenResty `nginx -t` 以及绑定域名的实际 HTTP 访问。

## 7. HTTPS、证书、ACME、DNS 和 CA

### 7.1 站点 HTTPS

| 前端函数 | 方法和路径 | 关键字段/变体 | 响应 | WorkMesh | 状态 |
| --- | --- | --- | --- | --- | --- |
| `getHTTPSConfig` | GET `/api/v2/websites/{id}/https` | 路径 ID | `HTTPSConfig` | `website_config_routes.go`, `website_https.go` | `explicit-real-handler` / `not-run` |
| `updateHTTPSConfig` | POST `/api/v2/websites/{websiteId}/https` | `enable`, `websiteSSLId`, `type`, `certificate`, `privateKey`, `privateKeyPath`, `certificatePath`, `httpConfig`, `SSLProtocol`, `algorithm`, `http3`, `acmeAccountID` | `HTTPSConfig` | 同上 | `explicit-real-handler` / `not-run` |

前端常用变体：`type: existed`、证书粘贴 `importType: paste`、`httpConfig: HTTPToHTTPS`、`SSLProtocol: [TLSv1.3, TLSv1.2]`、`httpsPort: 443`。服务端还兼容历史字段 `enabled`、`SSLID`，但新实现以 1Panel 字段为准。`operate` 仅允许 `enable`/`disable`。

### 7.2 账户和证书

| 前端函数 | 方法和路径 | 关键字段/变体 | 响应 | 状态 |
| --- | --- | --- | --- | --- |
| `searchDnsAccount` | POST `/api/v2/websites/dns/search` | 分页 | 分页 `DnsAccount` | `explicit-real-handler` / `not-run` |
| `createDnsAccount` | POST `/api/v2/websites/dns` | `name`, `type`, `provider`, `authorization/credentials` | `DnsAccount`/操作结果 | `explicit-real-handler` / `not-run` |
| `updateDnsAccount` | POST `/api/v2/websites/dns/update` | `id` + 账户字段 | `DnsAccount`/操作结果 | `explicit-real-handler` / `not-run` |
| `deleteDnsAccount` | POST `/api/v2/websites/dns/del` | `{ id }` | 操作结果 | `explicit-real-handler` / `not-run` |
| `searchAcmeAccount` | POST `/api/v2/websites/acme/search` | 分页 | 分页 `AcmeAccount` | `explicit-real-handler` / `not-run` |
| `createAcmeAccount` | POST `/api/v2/websites/acme` | `type: letsencrypt`, email, keyType | `AcmeAccount` | `explicit-real-handler` / `not-run` |
| `updateAcmeAccount` | POST `/api/v2/websites/acme/update` | `id` +账户字段 | `AcmeAccount` | `explicit-real-handler` / `not-run` |
| `deleteAcmeAccount` | POST `/api/v2/websites/acme/del` | `{ id }` | 操作结果 | `explicit-real-handler` / `not-run` |
| `searchSSL` | POST `/api/v2/websites/ssl/search` | 分页，可带 `CurrentNode` | 分页 `SSLDTO` | `explicit-real-handler` / `not-run` |
| `listSSL` / `listLocalNodeSSL` | POST `/api/v2/websites/ssl/list` | 证书筛选；普通/本节点请求 | `SSLDTO[]` | `explicit-real-handler` / `not-run` |
| `createSSL` | POST `/api/v2/websites/ssl` | 证书元数据 | `SSLCreate`/操作结果 | `explicit-real-handler` / `not-run` |
| `getSSL` | GET `/api/v2/websites/ssl/{id}` | 路径 ID | `SSL` | `explicit-real-handler` / `not-run` |
| `obtainSSL` | POST `/api/v2/websites/ssl/obtain` | 域名、ACME 账户、DNS/HTTP-01 参数 | 操作结果 | `blocked`（需要 ACME/域名） |
| `updateSSL` | POST `/api/v2/websites/ssl/update` | 证书属性 | 操作结果 | `explicit-real-handler` / `not-run` |
| `deleteSSL` | POST `/api/v2/websites/ssl/del` | `{ id }` | 操作结果 | `explicit-real-handler` / `not-run` |
| `pushSSLToNode` | POST `/api/v2/websites/ssl/push` | 证书和目标节点，可带 `CurrentNode` | 操作结果 | `blocked`（无次节点） |
| `getDnsResolve` | POST `/api/v2/websites/ssl/resolve` | 域名解析请求 | `DNSResolve[]` | `blocked`（真实 DNS） |
| `uploadSSL` | POST `/api/v2/websites/ssl/upload` | `type: paste`, `sslID`, `privateKey`, `certificate`, 路径字段 | 操作结果 | `explicit-real-handler` / `not-run` |
| `uploadSSLFile` | multipart `/api/v2/websites/ssl/upload/file` | 证书文件 | `File` | `explicit-real-handler` / `not-run` |
| `downloadFile` | DOWNLOAD `/api/v2/websites/ssl/download` | 下载参数 | Blob | `explicit-real-handler` / `not-run` |

### 7.3 自签 CA

| 函数 | 方法和路径 | 请求/响应 | 状态 |
| --- | --- | --- | --- |
| `searchCAs` | POST `/api/v2/websites/ca/search` | 分页 → `CA` 分页 | `explicit-real-handler` / `not-run` |
| `createCA` | POST `/api/v2/websites/ca` | CA 字段 → `CA` | `explicit-real-handler` / `not-run` |
| `getCA` | GET `/api/v2/websites/ca/{id}` | ID → `CADTO` | `explicit-real-handler` / `not-run` |
| `obtainSSLByCA` | POST `/api/v2/websites/ca/obtain` | CA、域名、有效期 → 操作结果 | `explicit-real-handler` / `not-run` |
| `renewSSLByCA` | POST `/api/v2/websites/ca/renew` | `{ SSLID }` → 操作结果 | `explicit-real-handler` / `not-run` |
| `deleteCA` | POST `/api/v2/websites/ca/del` | `{ id }` → 操作结果 | `explicit-real-handler` / `not-run` |
| `downloadCAFile` | DOWNLOAD `/api/v2/websites/ca/download` | 下载参数 → Blob | `explicit-real-handler` / `not-run` |

自动签发验收使用当前系统的 Let's Encrypt HTTP-01；必须绑定独立 `*.cs.sopvip.com` 前置域名，验证 DNS/HTTP 可达、挑战文件、证书落盘和 HTTPS 配置。没有真实域名、端口和 ACME 凭据时只能记录 `blocked`，禁止用假证书或 JSON 数据代替。

## 8. 反向代理、缓存、重定向和安全配置

### 8.1 反向代理和缓存

| 函数 | 方法和路径 | 多操作/关键字段 | 响应 | WorkMesh | 状态 |
| --- | --- | --- | --- | --- | --- |
| `getProxyConfig` | POST `/api/v2/websites/proxies` | 网站 ID、筛选 | `ProxyConfig[]` | `website_config_routes.go`, `website_proxy.go` | `explicit-real-handler` / `not-run` |
| `operateProxyConfig` | POST `/api/v2/websites/proxies/update` | `operate: create/edit/delete`；`name`, `match`, `proxyPass`, `proxyProtocol`, `proxyAddress`, `proxyHost`, `sni`, `sslVerify`, `replaces`, CORS/缓存字段 | 操作结果 | `website_proxy.go` | `wildcard-real-handler` / `not-run` |
| `deleteProxyConfig` | POST `/api/v2/websites/proxies/delete` | `{ id, websiteID }` | 操作结果 | 扩展分发器 | `wildcard-real-handler` / `not-run` |
| `updateProxyConfigStatus` | POST `/api/v2/websites/proxies/status` | `{ id, name, status }` | 操作结果 | 扩展分发器 | `wildcard-real-handler` / `not-run` |
| `updateProxyConfigFile` | POST `/api/v2/websites/proxies/file` | 配置内容/文件字段 | 操作结果 | 扩展分发器 | `wildcard-real-handler` / `not-run` |
| `getCacheConfig` | GET `/api/v2/websites/proxy/config/{id}` | ID | `WebsiteCacheConfig` | `website_config_routes.go` | `explicit-real-handler` / `not-run` |
| `updateCacheConfig` | POST `/api/v2/websites/proxy/config` | 缓存字段 | 操作结果 | `website_config_routes.go` | `explicit-real-handler` / `not-run` |
| `clearProxyCache` | POST `/api/v2/websites/proxy/clear` | 网站 ID | 操作结果 | `website_config_routes.go` | `explicit-real-handler` / `not-run` |

前端可能传入的代理状态值包括 `enable`, `disable`, `enabled`, `running`, `on`；服务端必须归一化后再持久化。真实验收必须使用可访问的上游，不能用 `127.0.0.1:9` 等伪地址。

### 8.2 重定向、认证、防盗链、真实 IP、CORS

| 函数 | 方法和路径 | 关键请求字段/变体 | 响应 | 状态 |
| --- | --- | --- | --- | --- |
| `getRedirectConfig` | POST `/api/v2/websites/redirect` | 网站 ID | `RedirectConfig[]` | `explicit-real-handler` / `not-run` |
| `operateRedirectConfig` | POST `/api/v2/websites/redirect/update` | `operate: create/edit/delete/enable/disable`；`domains`, `enable`, `name`, `keepPath`, `type`, `redirect`, `path`, `target`, `redirectRoot` | 操作结果 | `wildcard-real-handler` / `not-run` |
| `updateRedirectConfigFile` | POST `/api/v2/websites/redirect/file` | `filePath`, `content` | 操作结果 | `wildcard-real-handler` / `not-run` |
| `getAuthConfig` | POST `/api/v2/websites/auths` | `{ websiteID, scope: root, ... }` | `AuthConfig` | `wildcard-real-handler` / `not-run` |
| `operateAuthConfig` | POST `/api/v2/websites/auths/update` | `operate: create/edit/delete/enable/disable`, 用户名/密码/备注 | 操作结果 | `wildcard-real-handler` / `not-run` |
| `getPathAuthConfig` | POST `/api/v2/websites/auths/path` | 网站 ID、路径 | `NginxPathAuthConfig[]` | `wildcard-real-handler` / `not-run` |
| `operatePathAuthConfig` | POST `/api/v2/websites/auths/path/update` | `path`, `name`, 用户名/密码、`operate` | 操作结果 | `wildcard-real-handler` / `not-run` |
| `getAntiLeech` | POST `/api/v2/websites/leech` | 网站 ID | `LeechConfig` | `explicit-real-handler` / `not-run` |
| `updateAntiLeech` | POST `/api/v2/websites/leech/update` | `enable`, `cache`, `cacheTime`, `cacheUint`, `extends`, `return`, `serverNames`, `noneRef`, `logEnable`, `blocked` | 操作结果 | `explicit-real-handler` / `not-run` |
| `getRealIPConfig` | GET `/api/v2/websites/realip/config/{id}` | ID | `WebsiteRealIPConfig` | `explicit-real-handler` / `not-run` |
| `updateRealIPConfig` | POST `/api/v2/websites/realip/config` | `websiteID`, `open`, `ipFrom`, `ipHeader`, `ipOther` | 操作结果 | `explicit-real-handler` / `not-run` |
| `getCorsConfig` | GET `/api/v2/websites/cors/{id}` | ID | `CorsConfig` | `explicit-real-handler` / `not-run` |
| `updateCorsConfig` | POST `/api/v2/websites/cors/update` | `websiteID`, `cors`, `allowOrigins`, `allowMethods`, `allowHeaders`, `allowCredentials`, `preflight` | 操作结果 | `explicit-real-handler` / `not-run` |

## 9. PHP、运行时、负载均衡和 Stream

| 函数/页面 | 方法和路径 | 关键字段/响应 | WorkMesh | 状态 |
| --- | --- | --- | --- | --- |
| `changePHPVersion` | POST `/api/v2/websites/php/version` | `{ websiteID, runtimeID }` → 操作结果 | `website_extensions.go`, `runtime_php_routes.go` | `wildcard-real-handler` / `not-run` |
| PHP 配置 | POST `/api/v2/runtimes/php/config` 等运行时接口 | `scope: params/upload_max_filesize/max_execution_time/disable_functions` | `runtime_php_config.go` | `not-run` |
| `getLoadBalances` | GET `/api/v2/websites/{id}/lbs` | ID → `NginxUpstream[]` | `website_extensions.go`/配置存储 | `wildcard-real-handler` / `not-run` |
| `createLoadBalance` | POST `/api/v2/websites/lbs/create` | `websiteID`, `name`, `algorithm`, `servers[]` | `website_config_routes.go` | `explicit-real-handler` / `not-run` |
| `updateLoadBalance` | POST `/api/v2/websites/lbs/update` | 同上 | 同上 | `explicit-real-handler` / `not-run` |
| `deleteLoadBalance` | POST `/api/v2/websites/lbs/del` | `{ websiteID, name/id }` | 兼容/扩展路由 | `fallback-present` |
| `updateLoadBalanceFile` | POST `/api/v2/websites/lbs/file` | 内容或文件字段 | `website_config_routes.go` | `explicit-real-handler` / `not-run` |
| `updateWebsiteStream` | POST `/api/v2/websites/stream/update` | `websiteID`, `streamPorts`, `udp`, `algorithm`, `servers` | `website_stream.go`, 配置写入 | `explicit-real-handler` / `not-run` |
| 数据库绑定 | GET `/api/v2/websites/databases`、POST `/api/v2/websites/databases` | `websiteID`, `databaseID`, `databaseType` | `website_extensions.go` | `wildcard-real-handler` / `not-run` |
| 资源查询 | GET `/api/v2/websites/resource/{id}` | ID → `WebsiteResource[]` | `website_extensions.go` | `wildcard-real-handler` / `not-run` |
| Composer | POST `/api/v2/websites/exec/composer` | Composer 执行参数 | 扩展/兼容路由 | `fallback-present` |
| 跨站访问 | POST `/api/v2/websites/crosssite` | 跨站操作参数 | 扩展/兼容路由 | `fallback-present` |

每种运行时至少做真实闭环：创建 → 查询详情 → 停止 → 启动 → 重启 → 日志/任务查询 → 删除 → 重建并验证 SQLite 持久化。PHP 还要验证 FPM、扩展、配置和站点实际访问；没有 Docker 或真实运行时只能记 `blocked`。

## 10. 网站日志和监控

### 10.1 站点访问/错误日志

| 前端函数 | 方法和路径 | 请求 | `logType`/`operate` 变体 | 响应/实现 | 状态 |
| --- | --- | --- | --- | --- | --- |
| `getWebsiteLog` | POST `/api/v2/websites/log/search` | `{ id, page?, pageSize?, logType }` | `access.log`、`error.log` | 日志分页/内容；`website_extensions.go` → `WebsiteLog` | `wildcard-real-handler` / `not-run` |
| `opWebsiteLog` | POST `/api/v2/websites/log/operate` | `{ id, operate, logType }` | `enable`, `disable`, `delete` | 操作结果；`OperateWebsiteLog`/`ClearWebsiteLog` | `wildcard-real-handler` / `not-run` |

`access.log` 是访问日志，`error.log` 是错误日志；`delete` 表示清空对应文件，不是删除网站。验收必须确认日志文件实际开关、清空、重新产生内容，并与日志审计记录区分。

### 10.2 监控日志族

以下接口由网站监控页面或共享日志页面调用，必须纳入正式清单，当前实现仍需检查是否被 fallback 覆盖：

```text
POST /api/v2/websites/monitor/config/global
POST /api/v2/websites/monitor/config/site
POST /api/v2/websites/monitor/config/site/update
POST /api/v2/websites/monitor/logs/search
POST /api/v2/websites/monitor/logs/detail
POST /api/v2/websites/monitor/logs/clear
POST /api/v2/websites/monitor/logs/stat
POST /api/v2/websites/monitor/qps
POST /api/v2/websites/monitor/rank
POST /api/v2/websites/monitor/stat
POST /api/v2/websites/monitor/trend
POST /api/v2/websites/monitor/visitors
POST /api/v2/websites/monitor/visitors/loc
POST /api/v2/websites/monitor/websites
```

实现映射：`node/api/website_monitor_logs.go`。若完整路由组装中仍由 `legacy_routes_websites.go` 注册 `fallbackRouteHandler`，必须在集成测试中确认专用路由优先，否则状态保持 `fallback-present`。

## 11. WAF 和 OpenResty

### 11.1 WAF 路径族

```text
GET/POST /api/v2/websites/waf/global
GET/POST /api/v2/websites/waf/sites
GET/POST /api/v2/websites/waf/rules
POST     /api/v2/websites/waf/rules/delete
POST     /api/v2/websites/waf/access-lists
POST     /api/v2/websites/waf/block/search
POST     /api/v2/websites/waf/log/search
POST     /api/v2/websites/waf/attack/stat
POST     /api/v2/websites/waf/relation/stat
POST     /api/v2/websites/waf/test
```

专用注册：`node/api/website_waf.go`、`website_waf_rules.go`；服务：`node/service/website_waf.go`、`website_waf_legacy.go`。WAF 状态、规则、封禁、日志和统计必须使用 SQLite 持久化，禁止 JSON sidecar 作为运行态数据源。

### 11.2 OpenResty 路径族

```text
GET  /api/v2/openresty
GET  /api/v2/openresty/https
GET  /api/v2/openresty/modules
GET  /api/v2/openresty/status
POST /api/v2/openresty/build
POST /api/v2/openresty/file
POST /api/v2/openresty/https
POST /api/v2/openresty/modules/update
POST /api/v2/openresty/scope
POST /api/v2/openresty/update
```

实现：`node/api/website_openresty.go`；兼容 fallback：`node/api/legacy_routes_openresty.go`。验收必须运行真实 `docker exec workmesh-openresty-waf nginx -t`，清理 `/www/wwwroot/runtime.example/nginx/site.conf` 中的 `proxy_pass http://;` 等无效残留；不得改变系统既有 OpenResty/WAF 目录布局。

## 12. 模板和产物

| 前端函数 | 方法和路径 | 请求/响应 | 状态 |
| --- | --- | --- | --- |
| `searchTemplates` | POST `/api/v2/websites/templates/search` | 分页/筛选 → 分页 `Template` | `explicit-real-handler`/`not-run` |
| `createTemplate` | POST `/api/v2/websites/templates` | 模板字段 → 操作结果 | `explicit-real-handler`/`not-run` |
| `updateTemplate` | POST `/api/v2/websites/templates/update` | 模板字段 → 操作结果 | `explicit-real-handler`/`not-run` |
| `deleteTemplate` | POST `/api/v2/websites/templates/del` | `{ id }` → 操作结果 | `explicit-real-handler`/`not-run` |
| `getTemplate` | POST `/api/v2/websites/templates/get` | `{ id }` → `Template` | `explicit-real-handler`/`not-run` |
| `uploadTemplateZip` | multipart POST `/api/v2/websites/templates/upload` | `file` → `{ filePath, variables[] }` | `explicit-real-handler`/`not-run` |
| `previewTemplate` | POST `/api/v2/websites/templates/preview` | 预览参数 → `PreviewDTO` | `explicit-real-handler`/`not-run` |
| `searchTemplateOutputs` | POST `/api/v2/websites/templates/outputs/search` | 分页 → 分页 `TemplateOutputDTO` | `explicit-real-handler`/`not-run` |
| `createTemplateOutput` | POST `/api/v2/websites/templates/outputs` | 产物字段 → 操作结果 | `explicit-real-handler`/`not-run` |
| `deleteTemplateOutput` | POST `/api/v2/websites/templates/outputs/del` | `{ id }` → 操作结果 | `explicit-real-handler`/`not-run` |
| `getTemplateOutput` | POST `/api/v2/websites/templates/outputs/get` | `{ id }` → `TemplateOutputDTO` | `explicit-real-handler`/`not-run` |

模板上传和预览必须使用真实文件、真实模板目录和 SQLite 元数据；不能用内存数组或 JSON 假数据模拟页面结果。

## 13. WS 终端契约

网站运行时终端不是网站专用 HTTP API，而是复用主机终端 WS。参考页面 `frontend/src/views/website/runtime/components/terminal.vue` 使用：

| 场景 | Upgrade 路径 | 查询参数/示例 | 依赖 | 状态 |
| --- | --- | --- | --- | --- |
| 运行时容器终端 | `GET/WS /api/v2/hosts/terminal/container` | `source=container&containerid=<containerID>&user=<user>&command=/bin/bash` | 运行时容器、管理员会话 | `blocked`/`not-run` |
| 数据库容器终端 | `GET/WS /api/v2/hosts/terminal/container` | 同路径，容器 ID 和命令由数据库页面传入 | 数据库容器 | `blocked`/`not-run` |
| 本地主机终端 | `GET/WS /api/v2/hosts/terminal/local` | 页面生成的终端参数 | 主机 shell | `blocked`/`not-run` |
| SSH 终端 | `GET/WS /api/v2/hosts/terminal/ssh` | SSH 主机、端口、用户、认证参数 | 可连通 SSH 主机 | `blocked`/`not-run` |
| 脚本执行流 | `GET/WS /api/v2/core/script/run` | 脚本/任务参数由脚本页面生成 | 有权限的任务执行器 | `blocked`/`not-run` |

服务端注册：`node/api/runtime_toolbox_routes.go`、`node/api/websocket_stream.go`。必须验证 RFC6455 Upgrade、Origin、Session 或 `X-WorkMesh-Token` 鉴权、客户端掩码、单消息最大 1 MiB、非分片、空闲超时、30 秒写超时和断线清理。无有效会话、容器或 SSH 目标时，不能将静态路由存在写成 WS 通过。

## 14. WorkMesh 路由实现和 fallback 风险

| 领域 | 专用实现 | 风险 |
| --- | --- | --- |
| CRUD/生命周期 | `website_crud.go`, `website_lifecycle.go` | `/api/v2/sites` 是兼容主/次站点别名，需确认不会替代网站主接口 |
| 域名 | `website_domain.go`, `website_domains_config.go`, `website_domain_runtime.go` | 主域名删除、配置重渲染必须验证 |
| 配置/目录/重写 | `website_config_routes.go`, `website_config_handlers.go`, `website_config_write.go`, `website_rewrite.go` | 同路径按 `scope`/`operate` 分支，不能只按 URL 返回固定数据 |
| 代理/HTTPS/证书 | `website_proxy.go`, `website_https.go`, `website_cert_routes.go`, `ssl_acme.go` | 外部上游、ACME、次节点为 blocked 条件 |
| 日志 | `website_logs_dirs.go`, `website_monitor_logs.go` | 访问/错误日志与审计日志不能混淆 |
| WAF/OpenResty | `website_waf.go`, `website_openresty.go` | 目录不能改变；必须以 OpenResty 实际配置测试为准 |
| 模板 | `website_templates_routes.go`, `website_template_upload.go`, `website_templates_render.go` | 上传、预览、产物要真实文件和 SQLite |
| 兼容分发 | `website_extensions.go` | `GET/POST /api/v2/websites/{rest...}` 对未知路径返回 404；已知路径必须核对分支 |
| 历史 fallback | `legacy_routes_websites.go`, `legacy_routes_openresty.go` | 同路径重复注册的实际覆盖顺序必须由 ServeMux/路由测试确认 |

特别检查：`legacy_routes_websites.go` 仍注册大量 `/api/v2/websites/...` 的 `fallbackRouteHandler`，包括日志、监控、代理、LBS、模板、WAF、OpenResty 相关路径。专用处理器存在不等于运行时一定命中专用处理器；集成门禁必须记录实际命中的处理器和返回体。

## 15. 真实测试矩阵和上线验收

本次文档阶段全部为 `not-run`；以下是唯一集成测试智能体必须执行的门禁，不允许各开发智能体各自宣称通过：

| 阶段 | 测试内容 | 证据要求 | 当前状态 |
| --- | --- | --- | --- |
| 路由 | 106 个前端导出调用、网站 130 条菜单引用去重后逐一请求 | 路由、方法、HTTP 状态、响应摘要 | `not-run` |
| 契约 | 每个 `type/operate/logType/scope` 变体 | 请求 JSON、响应 JSON/Blob、错误响应 | `not-run` |
| SQLite | 创建、更新、删除、重启后的记录和迁移 | 数据库表、字段、重启前后对比 | `not-run` |
| 生命周期 | deployment/runtime/static/proxy/subsite/stream | 创建→查→停→启→重启→日志/任务→删→重建 | `not-run`/`blocked` |
| 网站访问 | 每个网站绑定独立 `*.cs.sopvip.com` 前缀 | HTTP 状态、Host、内容、配置路径 | `blocked`（若 DNS/容器不可用） |
| OpenResty | 配置渲染和 `nginx -t` | 命令输出、实际 `site.conf` 路径 | `blocked`（若容器不可用） |
| ACME | Let's Encrypt HTTP-01 自动签发和续期 | challenge、证书文件、HTTPS 访问 | `blocked`（若公网条件不可用） |
| PHP | FPM、扩展、配置、站点访问 | 容器状态、配置、页面响应 | `blocked`（若 Docker 不可用） |
| WS | 4 条 WS 路径和断线清理 | Upgrade 状态、收发消息、关闭码 | `blocked`（无会话/资源） |
| 安全 | 未登录、低权限、非法字段、路径穿越、无效操作 | 401/403/400/404，不能返回 200 假成功 | `not-run` |
| 质量 | `GOWORK=off go test ./...`、`go vet ./...`、必要的 race 门禁 | 原始命令输出和时间 | `not-run` |

每条记录至少包含：菜单、页面、前端函数、HTTP/WS、方法、完整 `/api/v2` 路径、请求摘要、响应摘要、HTTP 状态码、命中的 WorkMesh 处理器、配置/日志文件路径、域名访问结果、生命周期结果、失败原因和未覆盖项。外部依赖缺失要明确写 `blocked`，不允许用静态扫描结果冒充真实通过。

## 16. v1 → v2 迁移记录

- 正式前端调用统一为 `/api/v2/...`；本文件所有路径均为 v2。
- 旧 v1 路径和旧字段只能在迁移适配层或历史文档中出现，不能重新暴露为新业务入口。
- `enabled`/`SSLID` 等 WorkMesh 历史兼容字段必须在服务端兼容读取，但响应应同时提供原版字段（如 `enable`、`websiteSSLId`）以保证 1Panel 前端无感切换。
- 任何 v1→v2 变更必须在 `docs/migration` 增加：旧路径、旧方法、旧请求/响应、v2 路径、兼容期限、调用方和测试证据。
- 迁移完成标准不是路由返回 200，而是前端页面真实操作、SQLite 持久化、OpenResty 配置和日志审计均一致。

