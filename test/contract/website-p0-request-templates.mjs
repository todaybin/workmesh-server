#!/usr/bin/env node
/* SPDX-License-Identifier: GPL-3.0-only */
/* Generate executable P0 website HTTP request templates; never sends requests. */

import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '../..');
const detailPath = path.join(root, 'docs/development/progress/2026-09-08-route-gap-detail.md');
const outputPath = path.join(root, 'docs/development/progress/2026-09-08-website-p0-request-templates.md');

const common = 'BASE=${BASE:-http://127.0.0.1:9999}; COOKIE=${COOKIE:?set session cookie}; CSRF=${CSRF:?set pcsrftoken}; CID="Cookie: workmesh_session=$COOKIE; pcsrftoken=$CSRF"; CT="Content-Type: application/json";';
const specs = [
  ['GET', '/api/v2/websites/{website_id}', '', 'websites row by id', 'websites table row; site directory exists'],
  ['GET', '/api/v2/websites/{website_id}/config/{type}', '', 'website_configs value', 'SQLite config row; corresponding managed.conf if type writes one'],
  ['GET', '/api/v2/websites/{website_id}/https', '', 'https config', 'SQLite https config; OpenResty certificate/redirect config'],
  ['POST', '/api/v2/websites/{website_id}/https', '{"websiteId":{{website_id}},"enable":true,"websiteSSLId":{{ssl_id}},"type":"http","httpConfig":"HTTPToHTTPS","SSLProtocol":["TLSv1.2"],"algorithm":"ECDHE-RSA-AES256-GCM-SHA384","http3":false}', 'updated HTTPS config', 'SQLite https row; run nginx -t and HTTPS request'],
  ['GET', '/api/v2/websites/{website_id}/lbs', '', 'upstream list', 'SQLite website config; upstream managed.conf and nginx -t'],
  ['POST', '/api/v2/websites/auths', '{"websiteID":{{website_id}}}', 'auth config list', 'SQLite auth config; auth_basic managed.conf/users.htpasswd'],
  ['POST', '/api/v2/websites/auths/path', '{"websiteID":{{website_id}}}', 'path auth config list', 'SQLite path-auth config; path_auth/managed.conf'],
  ['POST', '/api/v2/websites/auths/update', '{"websiteID":{{website_id}},"operate":"create","username":"{{real_user}}","password":"{{real_password}}","remark":"release-check","scope":"server"}', 'auth operation result', 'SQLite auth row; users.htpasswd permissions; nginx -t'],
  ['GET', '/api/v2/websites/ca/{ca_id}', '', 'CA detail', 'SQLite CA row; CSR/private-key files exist with restricted mode'],
  ['POST', '/api/v2/websites/ca/download', '{"id":{{ca_id}}}', 'binary download (Content-Disposition)', 'Downloaded bytes equal persisted CA material; no JSON simulation'],
  ['GET', '/api/v2/websites/cors/{website_id}', '', 'CORS config', 'SQLite website config; cors managed.conf and nginx -t'],
  ['GET', '/api/v2/websites/default/html/{type}', '', 'HTML content', 'Configured default HTML file content; path remains OpenResty site directory'],
  ['POST', '/api/v2/websites/default/server', '{"id":{{website_id}}}', 'operation result', 'SQLite default_server flag; generated OpenResty server selection; nginx -t'],
  ['GET', '/api/v2/websites/domains/{website_id}', '', 'domain list', 'SQLite website_domains rows; OpenResty server_name entries'],
  ['POST', '/api/v2/websites/leech', '{"websiteID":{{website_id}}}', 'anti-leech config', 'SQLite leech config; leech managed.conf and nginx -t'],
  ['POST', '/api/v2/websites/leech/update', '{"websiteID":{{website_id}},"enable":true,"serverNames":["{{domain}}"],"noneRef":true,"blocked":true}', 'updated anti-leech config', 'SQLite config changed; request with invalid Referer returns 403'],
  ['POST', '/api/v2/websites/log/operate', '{"id":{{website_id}},"operate":"open","logType":"access"}', 'log operation result', 'SQLite log setting; access log file created/updated under site directory'],
  ['POST', '/api/v2/websites/log/search', '{"id":{{website_id}},"page":1,"pageSize":20,"logType":"access"}', 'paged log data', 'Read real access log file; response total/items match file lines'],
  ['POST', '/api/v2/websites/operate?operateNode={{node_id}}', '{"id":{{website_id}},"operate":"restart"}', 'operation/task result', 'SQLite website status/task; OpenResty reload or restart result; nginx -t'],
  ['POST', '/api/v2/websites/php/version', '{"websiteID":{{website_id}},"runtimeID":"{{php_runtime_id}}"}', 'operation result', 'SQLite runtime relation; PHP-FPM pool/config and site request'],
  ['POST', '/api/v2/websites/proxies/delete', '{"id":{{website_id}},"name":"{{proxy_name}}"}', 'operation result', 'SQLite proxy config deleted; managed proxy file removed; nginx -t'],
  ['POST', '/api/v2/websites/proxies/file', '{"websiteID":{{website_id}},"name":"{{proxy_name}}","content":"location /api { proxy_pass http://127.0.0.1:18080; }"}', 'operation result', 'SQLite proxy file content; OpenResty file exact content and nginx -t'],
  ['POST', '/api/v2/websites/proxies/status', '{"id":{{website_id}},"name":"{{proxy_name}}","status":"enable"}', 'operation result', 'SQLite enabled flag; effective proxy location after nginx reload'],
  ['POST', '/api/v2/websites/proxies/update', '{"id":{{website_id}},"name":"{{proxy_name}}","operate":"create","enable":true,"cache":false}', 'proxy config result', 'SQLite proxy config; proxy managed.conf and nginx -t'],
  ['GET', '/api/v2/websites/proxy/config/{website_id}', '', 'proxy/cache config', 'SQLite proxy config; generated proxy config remains valid'],
  ['GET', '/api/v2/websites/realip/config/{website_id}', '', 'real IP config', 'SQLite realip config; set_real_ip_from directives and nginx -t'],
  ['POST', '/api/v2/websites/redirect', '{"websiteID":{{website_id}}}', 'redirect config list', 'SQLite redirect config; redirect managed.conf and nginx -t'],
  ['POST', '/api/v2/websites/redirect/file', '{"websiteID":{{website_id}},"name":"{{redirect_name}}","content":"return 301 https://{{domain}}$request_uri;"}', 'operation result', 'SQLite redirect file content; OpenResty file and nginx -t'],
  ['POST', '/api/v2/websites/redirect/update', '{"websiteID":{{website_id}},"operate":"create","name":"{{redirect_name}}","enable":true,"type":"redirect","target":"https://{{domain}}","keepPath":true}', 'redirect config result', 'SQLite redirect row; HTTP request returns expected 301/302'],
  ['GET', '/api/v2/websites/resource/{website_id}', '', 'resource list', 'SQLite website/app/runtime relations; paths point to real resources'],
  ['GET', '/api/v2/websites/ssl/{ssl_id}', '', 'SSL detail', 'SQLite SSL row; certificate/key files and expiry match response'],
  ['POST', '/api/v2/websites/ssl/download', '{"id":{{ssl_id}}}', 'binary certificate download', 'Downloaded bytes match persisted certificate; Content-Disposition set'],
  ['POST', '/api/v2/websites/update?operateNode={{node_id}}', '{"id":{{website_id}},"primaryDomain":"{{domain}}","remark":"release-check","webSiteGroupId":0,"IPV6":false,"favorite":false}', 'updated website', 'SQLite websites row changed; OpenResty server_name and config remain valid'],
];

function routeCountFromDetail() {
  const text = fs.readFileSync(detailPath, 'utf8');
  return text.split('\n').filter((line) => line.startsWith('| websites |') && line.includes('| 501 |')).length;
}

function curl(spec) {
  const [method, route, body] = spec;
  const pathValue = route
    .replace(/:param/g, '')
    .replace(/\{\{([a-z0-9_]+)\}\}/g, (_, name) => `\${${name.toUpperCase()}}`)
    .replace(/\{([a-z0-9_]+)\}/g, (_, name) => `\${${name.toUpperCase()}}`);
  let command = `${common}\nURL="$BASE${pathValue}"\ncurl --fail-with-body -sS -X ${method} "$URL" -H "$CID"`;
  if (body) {
    const expandedBody = body.replace(/\{\{([a-z0-9_]+)\}\}/g, (_, name) => `\${${name.toUpperCase()}}`);
    command += ` -H "$CT" --data @- <<JSON\n${expandedBody}\nJSON`;
  }
  return command;
}

function envelope(method, response) {
  if (response.includes('binary')) return '`HTTP 200`, binary body with `Content-Disposition`; do not parse as JSON`';
  return '`HTTP 200`, JSON `{ "code": 200, "data": ... }`';
}

function render() {
  const lines = [
    '<!-- SPDX-License-Identifier: GPL-3.0-only -->',
    '<!-- Copyright (c) 2026 WorkMesh contributors -->',
    '',
    '# SEC-04 网站 P0 501 请求模板',
    '',
    '本文件由 `test/contract/website-p0-request-templates.mjs` 生成。模板只使用真实资源变量，不创建 JSON 模拟数据。执行前设置 `BASE`、`COOKIE`、`CSRF` 以及 `WEBSITE_ID`/`SSL_ID`/`CA_ID` 等实际值。写请求必须同时发送 `pcsrftoken` Cookie 和 `X-CSRF-Token`；节点参数只在真实主次节点场景使用。',
    '',
    `当前静态矩阵中的网站 501 条目：${routeCountFromDetail()}；本模板条目：${specs.length}。两者必须一致。`,
    '',
    '统一错误 envelope：`HTTP 501` 当前应为 `{ "code": "ERR", "details": { "errCode": "NOT_IMPLEMENTED", "method": "...", "path": "..." }, "message": "该 v2 功能尚未实现" }`。替换为真实处理器后，成功响应必须保持 `{ "code": 200, "data": ... }`，不能以空数据伪造完成。',
    '',
    '## 模板',
    '',
  ];
  specs.forEach((spec, index) => {
    const [method, route, body, response, verify] = spec;
    lines.push(`### ${index + 1}. ${method} ${route}`, '', `- 参数来源：\`web/src/api/modules/website.ts\`（类型字段见 \`web/src/api/interface/website.ts\`）。`, `- 预期 envelope：${envelope(method, response)}；当前静态状态：\`501\`。`, `- 成功响应验证：${response}。`, `- SQLite/OpenResty 验证：${verify}。`, '', '```bash', curl(spec), '```', '');
  });
  lines.push('## 验收顺序', '', '1. 先用 GET 模板确认真实站点、域名、证书和配置记录存在。', '2. 再执行 POST 写模板；每次记录响应 JSON、SQLite 变更和生成文件 diff。', '3. 对生成配置执行 `docker exec workmesh-openresty-waf nginx -t`，再通过对应 `*.cs.sopvip.com` 域名访问确认行为。', '4. 失败时确认 SQLite 事务、配置文件和 OpenResty reload 均回滚；任何 501、固定空列表或模拟成功都保持未完成。', '');
  return lines.join('\n');
}

const count = routeCountFromDetail();
if (count !== specs.length) {
  console.error(`website 501/template count mismatch: detail=${count}, templates=${specs.length}`);
  process.exit(2);
}
const output = render();
if (process.argv.includes('--write')) fs.writeFileSync(outputPath, output);
else process.stdout.write(output);
console.error(`website P0 templates: ${specs.length}`);
