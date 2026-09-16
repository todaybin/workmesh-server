<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# SEC-04 网站 P0 501 请求模板

本文件由 `test/contract/website-p0-request-templates.mjs` 生成。模板只使用真实资源变量，不创建 JSON 模拟数据。执行前设置 `BASE`、`COOKIE`、`CSRF` 以及 `WEBSITE_ID`/`SSL_ID`/`CA_ID` 等实际值。写请求必须同时发送 `pcsrftoken` Cookie 和 `X-CSRF-Token`；节点参数只在真实主次节点场景使用。

当前静态矩阵中的网站 501 条目：33；本模板条目：33。两者必须一致。

统一错误 envelope：`HTTP 501` 当前应为 `{ "code": "ERR", "details": { "errCode": "NOT_IMPLEMENTED", "method": "...", "path": "..." }, "message": "该 v2 功能尚未实现" }`。替换为真实处理器后，成功响应必须保持 `{ "code": 200, "data": ... }`，不能以空数据伪造完成。

## 模板

### 1. GET /api/v2/websites/{website_id}

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：websites row by id。
- SQLite/OpenResty 验证：websites table row; site directory exists。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/${WEBSITE_ID}"
curl --fail-with-body -sS -X GET "$URL" -H "$CID"
```

### 2. GET /api/v2/websites/{website_id}/config/{type}

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：website_configs value。
- SQLite/OpenResty 验证：SQLite config row; corresponding managed.conf if type writes one。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/${WEBSITE_ID}/config/${TYPE}"
curl --fail-with-body -sS -X GET "$URL" -H "$CID"
```

### 3. GET /api/v2/websites/{website_id}/https

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：https config。
- SQLite/OpenResty 验证：SQLite https config; OpenResty certificate/redirect config。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/${WEBSITE_ID}/https"
curl --fail-with-body -sS -X GET "$URL" -H "$CID"
```

### 4. POST /api/v2/websites/{website_id}/https

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：updated HTTPS config。
- SQLite/OpenResty 验证：SQLite https row; run nginx -t and HTTPS request。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/${WEBSITE_ID}/https"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"websiteId":${WEBSITE_ID},"enable":true,"websiteSSLId":${SSL_ID},"type":"http","httpConfig":"HTTPToHTTPS","SSLProtocol":["TLSv1.2"],"algorithm":"ECDHE-RSA-AES256-GCM-SHA384","http3":false}
JSON
```

### 5. GET /api/v2/websites/{website_id}/lbs

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：upstream list。
- SQLite/OpenResty 验证：SQLite website config; upstream managed.conf and nginx -t。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/${WEBSITE_ID}/lbs"
curl --fail-with-body -sS -X GET "$URL" -H "$CID"
```

### 6. POST /api/v2/websites/auths

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：auth config list。
- SQLite/OpenResty 验证：SQLite auth config; auth_basic managed.conf/users.htpasswd。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/auths"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"websiteID":${WEBSITE_ID}}
JSON
```

### 7. POST /api/v2/websites/auths/path

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：path auth config list。
- SQLite/OpenResty 验证：SQLite path-auth config; path_auth/managed.conf。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/auths/path"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"websiteID":${WEBSITE_ID}}
JSON
```

### 8. POST /api/v2/websites/auths/update

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：auth operation result。
- SQLite/OpenResty 验证：SQLite auth row; users.htpasswd permissions; nginx -t。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/auths/update"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"websiteID":${WEBSITE_ID},"operate":"create","username":"${REAL_USER}","password":"${REAL_PASSWORD}","remark":"release-check","scope":"server"}
JSON
```

### 9. GET /api/v2/websites/ca/{ca_id}

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：CA detail。
- SQLite/OpenResty 验证：SQLite CA row; CSR/private-key files exist with restricted mode。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/ca/${CA_ID}"
curl --fail-with-body -sS -X GET "$URL" -H "$CID"
```

### 10. POST /api/v2/websites/ca/download

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, binary body with `Content-Disposition`; do not parse as JSON`；当前静态状态：`501`。
- 成功响应验证：binary download (Content-Disposition)。
- SQLite/OpenResty 验证：Downloaded bytes equal persisted CA material; no JSON simulation。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/ca/download"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"id":${CA_ID}}
JSON
```

### 11. GET /api/v2/websites/cors/{website_id}

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：CORS config。
- SQLite/OpenResty 验证：SQLite website config; cors managed.conf and nginx -t。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/cors/${WEBSITE_ID}"
curl --fail-with-body -sS -X GET "$URL" -H "$CID"
```

### 12. GET /api/v2/websites/default/html/{type}

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：HTML content。
- SQLite/OpenResty 验证：Configured default HTML file content; path remains OpenResty site directory。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/default/html/${TYPE}"
curl --fail-with-body -sS -X GET "$URL" -H "$CID"
```

### 13. POST /api/v2/websites/default/server

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：operation result。
- SQLite/OpenResty 验证：SQLite default_server flag; generated OpenResty server selection; nginx -t。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/default/server"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"id":${WEBSITE_ID}}
JSON
```

### 14. GET /api/v2/websites/domains/{website_id}

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：domain list。
- SQLite/OpenResty 验证：SQLite website_domains rows; OpenResty server_name entries。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/domains/${WEBSITE_ID}"
curl --fail-with-body -sS -X GET "$URL" -H "$CID"
```

### 15. POST /api/v2/websites/leech

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：anti-leech config。
- SQLite/OpenResty 验证：SQLite leech config; leech managed.conf and nginx -t。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/leech"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"websiteID":${WEBSITE_ID}}
JSON
```

### 16. POST /api/v2/websites/leech/update

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：updated anti-leech config。
- SQLite/OpenResty 验证：SQLite config changed; request with invalid Referer returns 403。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/leech/update"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"websiteID":${WEBSITE_ID},"enable":true,"serverNames":["${DOMAIN}"],"noneRef":true,"blocked":true}
JSON
```

### 17. POST /api/v2/websites/log/operate

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：log operation result。
- SQLite/OpenResty 验证：SQLite log setting; access log file created/updated under site directory。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/log/operate"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"id":${WEBSITE_ID},"operate":"open","logType":"access"}
JSON
```

### 18. POST /api/v2/websites/log/search

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：paged log data。
- SQLite/OpenResty 验证：Read real access log file; response total/items match file lines。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/log/search"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"id":${WEBSITE_ID},"page":1,"pageSize":20,"logType":"access"}
JSON
```

### 19. POST /api/v2/websites/operate?operateNode={{node_id}}

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：operation/task result。
- SQLite/OpenResty 验证：SQLite website status/task; OpenResty reload or restart result; nginx -t。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/operate?operateNode=${NODE_ID}"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"id":${WEBSITE_ID},"operate":"restart"}
JSON
```

### 20. POST /api/v2/websites/php/version

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：operation result。
- SQLite/OpenResty 验证：SQLite runtime relation; PHP-FPM pool/config and site request。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/php/version"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"websiteID":${WEBSITE_ID},"runtimeID":"${PHP_RUNTIME_ID}"}
JSON
```

### 21. POST /api/v2/websites/proxies/delete

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：operation result。
- SQLite/OpenResty 验证：SQLite proxy config deleted; managed proxy file removed; nginx -t。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/proxies/delete"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"id":${WEBSITE_ID},"name":"${PROXY_NAME}"}
JSON
```

### 22. POST /api/v2/websites/proxies/file

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：operation result。
- SQLite/OpenResty 验证：SQLite proxy file content; OpenResty file exact content and nginx -t。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/proxies/file"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"websiteID":${WEBSITE_ID},"name":"${PROXY_NAME}","content":"location /api { proxy_pass http://127.0.0.1:18080; }"}
JSON
```

### 23. POST /api/v2/websites/proxies/status

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：operation result。
- SQLite/OpenResty 验证：SQLite enabled flag; effective proxy location after nginx reload。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/proxies/status"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"id":${WEBSITE_ID},"name":"${PROXY_NAME}","status":"enable"}
JSON
```

### 24. POST /api/v2/websites/proxies/update

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：proxy config result。
- SQLite/OpenResty 验证：SQLite proxy config; proxy managed.conf and nginx -t。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/proxies/update"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"id":${WEBSITE_ID},"name":"${PROXY_NAME}","operate":"create","enable":true,"cache":false}
JSON
```

### 25. GET /api/v2/websites/proxy/config/{website_id}

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：proxy/cache config。
- SQLite/OpenResty 验证：SQLite proxy config; generated proxy config remains valid。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/proxy/config/${WEBSITE_ID}"
curl --fail-with-body -sS -X GET "$URL" -H "$CID"
```

### 26. GET /api/v2/websites/realip/config/{website_id}

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：real IP config。
- SQLite/OpenResty 验证：SQLite realip config; set_real_ip_from directives and nginx -t。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/realip/config/${WEBSITE_ID}"
curl --fail-with-body -sS -X GET "$URL" -H "$CID"
```

### 27. POST /api/v2/websites/redirect

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：redirect config list。
- SQLite/OpenResty 验证：SQLite redirect config; redirect managed.conf and nginx -t。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/redirect"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"websiteID":${WEBSITE_ID}}
JSON
```

### 28. POST /api/v2/websites/redirect/file

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：operation result。
- SQLite/OpenResty 验证：SQLite redirect file content; OpenResty file and nginx -t。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/redirect/file"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"websiteID":${WEBSITE_ID},"name":"${REDIRECT_NAME}","content":"return 301 https://${DOMAIN}$request_uri;"}
JSON
```

### 29. POST /api/v2/websites/redirect/update

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：redirect config result。
- SQLite/OpenResty 验证：SQLite redirect row; HTTP request returns expected 301/302。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/redirect/update"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"websiteID":${WEBSITE_ID},"operate":"create","name":"${REDIRECT_NAME}","enable":true,"type":"redirect","target":"https://${DOMAIN}","keepPath":true}
JSON
```

### 30. GET /api/v2/websites/resource/{website_id}

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：resource list。
- SQLite/OpenResty 验证：SQLite website/app/runtime relations; paths point to real resources。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/resource/${WEBSITE_ID}"
curl --fail-with-body -sS -X GET "$URL" -H "$CID"
```

### 31. GET /api/v2/websites/ssl/{ssl_id}

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：SSL detail。
- SQLite/OpenResty 验证：SQLite SSL row; certificate/key files and expiry match response。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/ssl/${SSL_ID}"
curl --fail-with-body -sS -X GET "$URL" -H "$CID"
```

### 32. POST /api/v2/websites/ssl/download

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, binary body with `Content-Disposition`; do not parse as JSON`；当前静态状态：`501`。
- 成功响应验证：binary certificate download。
- SQLite/OpenResty 验证：Downloaded bytes match persisted certificate; Content-Disposition set。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/ssl/download"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"id":${SSL_ID}}
JSON
```

### 33. POST /api/v2/websites/update?operateNode={{node_id}}

- 参数来源：`web/src/api/modules/website.ts`（类型字段见 `web/src/api/interface/website.ts`）。
- 预期 envelope：`HTTP 200`, JSON `{ "code": 200, "data": ... }`；当前静态状态：`501`。
- 成功响应验证：updated website。
- SQLite/OpenResty 验证：SQLite websites row changed; OpenResty server_name and config remain valid。

```bash
BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";
URL="$BASE/api/v2/websites/update?operateNode=${NODE_ID}"
curl --fail-with-body -sS -X POST "$URL" -H "$CID" -H "$CT" --data @- <<JSON
{"id":${WEBSITE_ID},"primaryDomain":"${DOMAIN}","remark":"release-check","webSiteGroupId":0,"IPV6":false,"favorite":false}
JSON
```

## 验收顺序

1. 先用 GET 模板确认真实站点、域名、证书和配置记录存在。
2. 再执行 POST 写模板；每次记录响应 JSON、SQLite 变更和生成文件 diff。
3. 对生成配置执行 `docker exec workmesh-openresty-waf nginx -t`，再通过对应 `*.cs.sopvip.com` 域名访问确认行为。
4. 失败时确认 SQLite 事务、配置文件和 OpenResty reload 均回滚；任何 501、固定空列表或模拟成功都保持未完成。
