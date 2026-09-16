#!/usr/bin/env node
/* SPDX-License-Identifier: GPL-3.0-only */
/* Static per-route gap audit. It never sends an HTTP request. */

import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '../..');
const inventoryPath = path.join(root, 'docs/inventory/frontend-api-inventory.json');
const outputPath = path.join(root, 'docs/development/progress/2026-09-08-route-gap-detail.md');
const domains = new Set(['websites', 'runtimes', 'databases', 'containers', 'hosts', 'cronjobs', 'logs', 'settings']);
const httpMethods = new Set(['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']);
const transportMethods = new Set(['DOWNLOAD', 'UPLOAD', 'POSTLOCALNODE', 'POSTWITHCONFIG']);

function filesUnder(dir, { includeTests = false } = {}) {
  return fs.readdirSync(path.join(root, dir))
    .filter((name) => name.endsWith('.go') && (includeTests || !name.endsWith('_test.go')))
    .map((name) => path.join(dir, name));
}

function sourceForScan(source) {
  return source.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|\s)\/\/.*$/gm, '$1');
}

function readGoRoutes() {
  const records = [];
  for (const file of [...filesUnder('node/api'), ...filesUnder('control/api')]) {
    const source = sourceForScan(fs.readFileSync(path.join(root, file), 'utf8'));
    const re = /HandleFunc\("(?:(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+)?([^" ]+)"(?:,\s*([^\n]+))?/g;
    for (const match of source.matchAll(re)) {
      const method = match[1] || '*';
      const route = match[2];
      records.push({ method, route, file, fallback: /fallbackRouteHandler/.test(match[3] || '') });
    }
    if (file.includes('legacy_routes')) continue;
    // 统一分发器以契约数组声明自己承接的路径；这些声明和分发器共同构成注册证据。
    if (/\b[A-Za-z_]\w*ContractPaths\s*=/.test(source)) {
      for (const match of source.matchAll(/["`](GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+(\/api\/v2\/[^"`]+)["`]/g)) {
        records.push({ method: match[1], route: match[2], file, fallback: false });
      }
    }
    // 部分模块通过局部注册助手添加别名路由。
    for (const match of source.matchAll(/\b(?:register|waf)\(\s*["`](GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)["`]\s*,\s*["`](\/api\/v2\/[^"`]+)["`]/g)) {
      records.push({ method: match[1], route: match[2], file, fallback: false });
    }
  }
  return records;
}

function normalizeRoute(route) {
  let value = route.split('?')[0].trim();
  // 清单解析器会把查询表达式附加为 /operate:param；只有紧跟普通段的
  // :param 是附加标记，独立的 /:param 仍是动态路径段。
  value = value.replace(/([^/]):param(?=$|\/)/g, '$1');
  value = value.replace(/:[A-Za-z_][A-Za-z0-9_]*/g, '{param}');
  value = value.replace(/\{[^}]+\.\.\.\}/g, '{rest...}');
  value = value.replace(/\{[^}]+\}/g, '{param}');
  return value.replace(/\/$/, '') || '/';
}

function inventoryMethod(method) {
  if (httpMethods.has(method)) return method;
  // web/src/api/index.ts 的 download 和 upload 都通过 Axios POST 发送。
  if (method === 'DOWNLOAD' || method === 'UPLOAD') return 'POST';
  if (transportMethods.has(method)) return 'POST';
  return method;
}

function routePatternCovers(registered, requested) {
  // A number of domains intentionally use a single authenticated dispatcher
  // for all descendants (for example `/api/v2/containers/` and
  // `/api/v2/hosts/`).  ServeMux treats that trailing-slash pattern as a
  // subtree, so the static audit must do the same instead of reporting every
  // child route as a fallback-only 501.
  const registeredText = registered.split('?')[0].trim();
  if (registeredText.endsWith('/')) {
    const prefix = normalizeRoute(registeredText);
    const target = normalizeRoute(requested);
    return target !== prefix && target.startsWith(prefix + '/');
  }
  const registeredParts = normalizeRoute(registered).split('/');
  const requestedParts = normalizeRoute(requested).split('/');
  if (registeredParts.length !== requestedParts.length || registeredParts.includes('{rest...}')) return false;
  return registeredParts.every((part, index) => part === requestedParts[index] || part === '{param}');
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

// 识别 []string 和 []struct 循环中的 HandleFunc("METHOD "+item.path) 注册。
function hasLoopRegistration(file, method, route) {
  if (file.includes('legacy_routes')) return false;
  const source = sourceForScan(fs.readFileSync(path.join(root, file), 'utf8'));
  const candidates = [...new Set([route, normalizeRoute(route)])];
  for (const candidate of candidates) {
    const escaped = escapeRegExp(candidate).replaceAll('\\{param\\}', '\\{[^}"`]+\\}');
    const listLoop = new RegExp(`range\\s+\\[\\]string\\s*\\{[^}]*["\`]${escaped}["\`][^}]*\\}\\s*\\{[\\s\\S]{0,1600}?HandleFunc\\(\\s*["\`]${method}\\s+["\`]\\s*\\+`, 'i');
    if (listLoop.test(source)) return true;
    const structLoop = new RegExp(`range\\s+\\[\\]struct[^{]*\\{[\\s\\S]{0,5000}?["\`]${escaped}["\`][\\s\\S]{0,5000}?\\}\\s*\\{[\\s\\S]{0,2600}?HandleFunc\\(\\s*["\`]${method}\\s+["\`]\\s*\\+\\s*[A-Za-z_]\\w*\\.(?:path|route)\\b`, 'i');
    if (structLoop.test(source)) return true;
  }
  return false;
}

function domainOf(route) {
  return route.path.split('/')[3] || 'root';
}

function sourceLabel(route) {
  return (route.sources || []).map((source) => source.replace(/^.*frontend\/src\//, 'web/src/')).join('; ');
}

function testEvidence(domain, route, goFiles) {
  const token = normalizeRoute(route.path).split('/').filter((part) => part && part !== '{param}').slice(-1)[0] || domain;
  return goFiles.filter((file) => file.endsWith('_test.go')).filter((file) => {
    const source = fs.readFileSync(path.join(root, file), 'utf8');
    return source.includes(`/api/v2/${domain}`) || source.toLowerCase().includes(token.toLowerCase());
  }).slice(0, 4);
}

const inventory = JSON.parse(fs.readFileSync(inventoryPath, 'utf8'));
const goRoutes = readGoRoutes();
const goFiles = [...filesUnder('node/api', { includeTests: true }), ...filesUnder('control/api', { includeTests: true })];
const rows = [];

for (const route of inventory.routes) {
  const domain = domainOf(route);
  if (!domains.has(domain)) continue;
  for (const declaredMethod of route.methods) {
    const method = inventoryMethod(declaredMethod);
    const normalized = normalizeRoute(route.path);
    const matches = goRoutes.filter((item) => (item.method === '*' || item.method === method) && routePatternCovers(item.route, normalized));
    const loopSources = [...filesUnder('node/api'), ...filesUnder('control/api')]
      .filter((file) => hasLoopRegistration(file, method, route.path))
      .map((file) => ({ method, route: route.path, file, fallback: false }));
    const explicit = [...matches.filter((item) => !item.fallback), ...loopSources]
      .filter((item, index, all) => all.findIndex((candidate) => candidate.file === item.file) === index);
    const fallback = matches.filter((item) => item.fallback);
    let status = '501';
    // legacy fallback 声明会由 legacyFilterMux 在真实注册时过滤；存在显式
    // 处理器时不能再把同一路径标成 partial。
    if (explicit.length) status = 'not-run';
    const evidence = testEvidence(domain, route, goFiles);
    rows.push({
      domain,
      method: declaredMethod,
      httpMethod: method,
      path: route.path,
      normalizedPath: normalized,
      status,
      handler: explicit.map((item) => item.file).join(', ') || (fallback.length ? fallback.map((item) => item.file).join(', ') : 'missing'),
      fallback: fallback.map((item) => item.file),
      parameterSource: sourceLabel(route),
      testStatus: evidence.length ? 'static-test-evidence' : 'not-run',
      testEvidence: evidence,
    });
  }
}

const summary = {};
for (const row of rows) {
  summary[row.domain] ||= { total: 0, implemented: 0, partial: 0, notRun: 0, fallback501: 0, testEvidence: 0 };
  const item = summary[row.domain];
  item.total += 1;
  if (row.status === 'implemented') item.implemented += 1;
  if (row.status === 'partial') item.partial += 1;
  if (row.status === 'not-run') item.notRun += 1;
  if (row.status === '501') item.fallback501 += 1;
  if (row.testStatus === 'static-test-evidence') item.testEvidence += 1;
}

function escape(value) {
  return String(value ?? '-').replaceAll('|', '\\|').replaceAll('\n', ' ');
}

function markdown() {
  const lines = [
    '<!-- SPDX-License-Identifier: GPL-3.0-only -->',
    '<!-- Copyright (c) 2026 WorkMesh contributors -->',
    '',
    '# SEC-03 逐路由缺口明细（静态生成）',
    '',
    '来源：`docs/inventory/frontend-api-inventory.json`、`node/api/*.go`、`control/api/*.go`。本文件只描述静态注册状态，未发送 HTTP 请求；`not-run` 不代表失败。',
    '',
    '## 统计',
    '',
    '| 领域 | 总条目 | 501/缺失 | partial | not-run | 静态测试证据 |',
    '| --- | ---: | ---: | ---: | ---: | ---: |',
  ];
  for (const domain of [...domains]) {
    const item = summary[domain] || { total: 0, fallback501: 0, partial: 0, notRun: 0, testEvidence: 0 };
    lines.push(`| ${domain} | ${item.total} | ${item.fallback501} | ${item.partial} | ${item.notRun} | ${item.testEvidence} |`);
  }
  lines.push('', '## 逐路由映射', '', '| 领域 | 前端方法 | 实际 HTTP | 路由 | 状态 | 真实处理器 | fallback | 参数来源 | 测试状态 |', '| --- | --- | --- | --- | --- | --- | --- | --- | --- |');
  for (const row of rows) {
    lines.push(`| ${escape(row.domain)} | ${escape(row.method)} | ${escape(row.httpMethod)} | \`${escape(row.path)}\` | ${row.status} | ${escape(row.handler)} | ${escape(row.fallback.join(', '))} | ${escape(row.parameterSource)} | ${row.testStatus}${row.testEvidence.length ? ` (${escape(row.testEvidence.join(', '))})` : ''} |`);
  }
  lines.push('', '## 解释', '', '- `not-run`：已找到显式 Go 处理器，但尚未由本脚本执行真实 HTTP/WS/SSE 验证。', '- `partial`：保留给发现具体处理器仍缺少业务闭环的后续审计结果；legacy fallback 声明本身不再导致 partial。', '- `501`：未找到显式处理器，或只找到 `fallbackRouteHandler`，上线前必须替换为真实业务逻辑。', '- `DOWNLOAD`、`UPLOAD`、`POSTLOCALNODE`、`POSTWITHCONFIG` 是前端传输语义；当前前端适配器均以 POST 发送这些请求，最终仍以黑盒请求记录为准。');
  return lines.join('\n') + '\n';
}

const output = markdown();
if (process.argv.includes('--write')) fs.writeFileSync(outputPath, output);
else process.stdout.write(output);
if (process.argv.includes('--summary')) process.stderr.write(`${JSON.stringify(summary)}\n`);
