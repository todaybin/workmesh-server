#!/usr/bin/env node
// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors
/**
 * 静态扫描旧 Core/Agent 路由在 workmesh-server 中的实现状态。
 * 该脚本只读取 Go/JSON 源码，不启动服务，也不依赖第三方包。
 */
import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS', 'Any'];
const MARKERS = [
  ['MIGRATION_PENDING', 'migration_pending'],
  ['StatusNotImplemented', 'status_not_implemented'],
  ['compatibilityHandler', 'compatibility_handler'],
  ['TODO', 'todo'],
];
// 这些查询虽然同一源码文件包含合法的空集合初始化，但响应由持久化状态计算，且有专用测试覆盖。
const dynamicEmptyResponseRoutes = new Set([
  'POST /api/v2/ai/agents/agent/list',
  'POST /api/v2/ai/agents/agent/channels',
  'POST /api/v2/ai/agents/overview',
  'POST /api/v2/ai/agents/delete/check',
  // 这些接口返回布尔状态或配置对象，源码文件中同时包含其他空集合，不能按文件级上下文误判。
  'GET /api/v2/backups/check/:name',
  'GET /api/v2/core/backups/client/:clientType',
]);

function filesUnder(root, { includeTests = false } = {}) {
  if (!fs.existsSync(root)) return [];
  const files = [];
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const full = path.join(root, entry.name);
    if (entry.isDirectory()) files.push(...filesUnder(full, { includeTests }));
    else if (entry.isFile() && full.endsWith('.go') && (includeTests || !full.endsWith('_test.go'))) files.push(full);
  }
  return files;
}

function cleanSegment(value) {
  return value.replaceAll('\\', '/').replace(/^\/+/, '').replace(/\/+$/, '');
}

function joinRoute(...parts) {
  return `/${parts.map(cleanSegment).filter(Boolean).join('/')}`.replaceAll(/\/+/g, '/');
}

function normalizePath(value) {
  return value
    .replaceAll(/\{([A-Za-z_]\w*)\.\.\.\}/g, '*$1')
    .replaceAll(/\{([A-Za-z_]\w*)\}/g, ':$1')
    .replaceAll(/\/+/g, '/');
}

function serveMuxPath(value) {
  return value
    .replaceAll(/:([A-Za-z_]\w*)/g, '{$1}')
    .replaceAll(/\*([A-Za-z_]\w*)/g, '{$1...}');
}

function scanLegacy(legacyRoot) {
  const roots = [
    ['core', path.join(legacyRoot, 'core', 'router'), '/api/v2/core'],
    ['agent', path.join(legacyRoot, 'agent', 'router'), '/api/v2'],
    ['core', path.join(legacyRoot, 'core', 'init', 'router'), '/'],
    ['agent', path.join(legacyRoot, 'agent', 'init', 'router'), '/'],
  ];
  const routes = [];
  for (const [area, root, base] of roots) {
    for (const file of filesUnder(root)) {
      const source = fs.readFileSync(file, 'utf8');
      const groups = new Map([['Router', base]]);
      for (const line of source.split(/\r?\n/)) {
        const assignment = line.match(/\b([A-Za-z_]\w*)\s*:=\s*([A-Za-z_]\w*)\.Group\(\s*["`]([^"`]+)["`]\s*\)/);
        if (assignment) groups.set(assignment[1], joinRoute(groups.get(assignment[2]) ?? base, assignment[3]));
        for (const method of METHODS) {
          const direct = line.match(new RegExp('\\b([A-Za-z_]\\w*)\\.' + method + '\\(\\s*["\\\x27]([^"\\\x27]+)["\\\x27]'));
          if (direct) routes.push({ method, path: joinRoute(groups.get(direct[1]) ?? base, direct[2]), source: path.relative(process.cwd(), file).replaceAll('\\', '/') });
          const chained = line.match(new RegExp('\\b([A-Za-z_]\\w*)\\.Group\\(\\s*["\\\x27]([^"\\\x27]+)["\\\x27]\\s*\\)\\.' + method + '\\(\\s*["\\\x27]([^"\\\x27]+)["\\\x27]'));
          if (chained) routes.push({ method, path: joinRoute(groups.get(chained[1]) ?? base, chained[2], chained[3]), source: path.relative(process.cwd(), file).replaceAll('\\', '/') });
          const handle = line.match(new RegExp('\\bHandleFunc\\(\\s*["\\\x27]' + method + '\\s+([^"\\\x27]+)["\\\x27]'));
          if (handle) routes.push({ method, path: joinRoute(handle[1]), source: path.relative(process.cwd(), file).replaceAll('\\', '/') });
        }
        const plain = line.match(/\bHandleFunc\(\s*["'](\/[^"']*)["']/);
        if (plain && !plain[1].includes(' ')) routes.push({ method: 'GET', path: joinRoute(plain[1]), source: path.relative(process.cwd(), file).replaceAll('\\', '/') });
      }
    }
  }
  // 隐藏路由可能在模块初始化文件中直接注册，而不位于 router 目录。
  // 扫描整个 Core/Agent 源码树并去重，确保基线覆盖动态初始化和插件入口。
  for (const [area, root, base] of [
    ['core', path.join(legacyRoot, 'core'), '/api/v2/core'],
    ['agent', path.join(legacyRoot, 'agent'), '/api/v2'],
  ]) {
    for (const file of filesUnder(root)) {
      if (file.includes(`${path.sep}router${path.sep}`) || file.includes(`${path.sep}init${path.sep}router${path.sep}`)) continue;
      const source = fs.readFileSync(file, 'utf8');
      const groups = new Map([['Router', base]]);
      for (const line of source.split(/\r?\n/)) {
        for (const method of METHODS) {
          const direct = line.match(new RegExp('\\b([A-Za-z_]\\w*)\\.' + method + '\\(\\s*["\\x27]([^"\\x27]+)["\\x27]'));
          if (direct) routes.push({ method, path: joinRoute(groups.get(direct[1]) ?? base, direct[2]), source: path.relative(process.cwd(), file).replaceAll('\\\\', '/') });
          const handle = line.match(new RegExp('\\bHandleFunc\\(\\s*["\\x27]' + method + '\\s+([^"\\x27]+)["\\x27]'));
          if (handle) routes.push({ method, path: joinRoute(handle[1]), source: path.relative(process.cwd(), file).replaceAll('\\\\', '/') });
        }
      }
    }
  }
  const unique = new Map(routes.map((route) => [`${route.method} ${route.path}`, route]));
  return [...unique.values()].sort((a, b) => `${a.method} ${a.path}`.localeCompare(`${b.method} ${b.path}`));
}

function routePattern(route) {
  const pathValue = normalizePath(route.path);
  return `${route.method} ${pathValue}`;
}

function domainFor(routePath) {
  const parts = routePath.split('/').filter(Boolean);
  if (parts[2] === 'core') return parts[3] || 'core';
  return parts[2] || 'unknown';
}

function authFor(text, routePath) {
  if (/\/health$/.test(routePath) || /\/auth\/(captcha|welcome|login)$/.test(routePath)) return 'public';
  if (/WORKMESH_COMMAND_TOKEN|X-WorkMesh-Token|Authorization|Bearer|token|auth|permission|role/i.test(text)) return 'required';
  return 'unknown';
}

function persistenceFor(text) {
  if (/gorm|database\/sql|sql\.DB|\.Create\(|\.Save\(|\.Updates\(|\.Delete\(/i.test(text)) return 'database';
  if (/sync\.Map|atomic\.|map\[string\]|GatewayStateStore|StateStore|in.?memory/i.test(text)) return 'memory';
  if (/\b(os\.|exec\.|io\.|http\.|json\.)/i.test(text)) return 'external';
  return 'unknown';
}

function collectNewSources(projectRoot) {
  const roots = ['node/api', 'control/api', 'runtime', 'cmd'];
  return roots.flatMap((relative) => filesUnder(path.join(projectRoot, relative))).map((file) => ({
    file,
    relative: path.relative(process.cwd(), file).replaceAll('\\', '/'),
    isTest: file.endsWith('_test.go'),
    text: fs.readFileSync(file, 'utf8'),
  }));
}

function findImplementations(route, sources) {
    // 新服务的 ServeMux 使用 {name} 参数；先转换后构造正则，避免旧 :name 替换残留参数名造成误判。
    const normalizedLegacy = serveMuxPath(route.path);
  const matches = [];
  for (const source of sources) {
    if (source.isTest) continue;
    const text = source.text;
    if (route.path === '/') {
      if (/\bHandleFunc\(\s*["'](?:GET\s+)?\/["']/.test(text)) matches.push(source);
      continue;
    }
    const routeRe = new RegExp('(?:HandleFunc|\\.(?:GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|Any))\\(\\s*["\\\x27](?:' + route.method + '\\s+)?' + normalizedLegacy.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replaceAll(':', '\\{[^}]+\\}') + '(?:["\\\x27]|/)', 'i');
    // 旧 Gin 路由使用 :id，新服务的 ServeMux 使用 {id}；两种字面量都作为实现证据。
    const literal = text.includes(`${route.method} ${route.path}`) || text.includes(`${route.method} ${normalizePath(route.path)}`) || text.includes(`${route.method} ${serveMuxPath(route.path)}`);
    // 路由常量在循环注册时以独立字符串出现（例如 "POST "+p），需要单独识别。
    const servePath = route.path.replaceAll(/:([A-Za-z_]\w*)/g, '{$1}').replaceAll(/\*([A-Za-z_]\w*)/g, '{$1...}');
    const plainLiteral = text.includes(`"${route.path}"`) || text.includes(`'${route.path}'`) || text.includes(`"${servePath}"`) || text.includes(`'${servePath}'`) || text.includes(`"${normalizePath(route.path)}"`) || text.includes(`'${normalizePath(route.path)}'`);
    // 统一前缀处理器（例如 /api/v2/ai/）覆盖该前缀下的全部路由。
    // 只将新服务源码中的前缀处理器视为实现，legacy_routes.go 仍按兼容占位单独标记。
    const prefixRe = /HandleFunc\(\s*"([^"]+)"/gi;
    const prefixReSingle = /HandleFunc\(\s*'([^']+)'/gi;
    let prefixMatch = false;
    for (const match of [...text.matchAll(prefixRe), ...text.matchAll(prefixReSingle)]) {
      const raw = match[1];
      const separator = raw.indexOf(' ');
      const method = separator > 0 ? raw.slice(0, separator).toUpperCase() : '';
      const prefix = separator > 0 ? raw.slice(separator + 1) : raw;
      if ((!method || method === route.method.toUpperCase() || method === 'ANY') && prefix.startsWith('/api/') && prefix.length > 5 && prefix.endsWith('/') && route.path.startsWith(prefix)) {
        prefixMatch = true;
        break;
      }
    }
    if (literal || plainLiteral || routeRe.test(text) || prefixMatch) matches.push(source);
  }
  return matches;
}

function inspectRoute(route, sources) {
  const matches = findImplementations(route, sources);
  const markers = new Set();
  const evidence = [];
  let best = matches[0];
  const contexts = matches.map((source) => {
    const indexes = [source.text.indexOf(route.path), source.text.indexOf(normalizePath(route.path))].filter((index) => index >= 0);
    const index = indexes[0] ?? -1;
    return { source, context: index >= 0 ? source.text.slice(Math.max(0, index - 160), index + 1000) : source.text };
  });
  const legacyConcreteHandler = contexts.some(({ source }) => {
    if (!source.relative.endsWith('legacy_routes.go')) return false;
    const escaped = route.path.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const registration = new RegExp(`${route.method}\\s+${escaped}[^\\n]*\\n(?:(?!migrationPendingMessage).){0,360}handle[A-Z]\\w+\\(w,\\s*r\\)`, 's');
    return registration.test(source.text);
  });
  for (const { source, context } of contexts) {
    for (const [marker, key] of MARKERS) {
      if (context.includes(marker)) { markers.add(key); evidence.push(`${key}:${source.relative}`); }
    }
    if (/compatibilityHandler/.test(context)) best = source;
  }
  if (matches.some((source) => source.relative.endsWith('legacy_routes.go'))) {
    markers.add('legacy_route');
    // 兼容注册文件中调用真实 handler 的路由仍属于已实现，不应被误判为占位。
    if (legacyConcreteHandler) {
      markers.add('legacy_concrete_handler');
    }
  }
  const concrete = contexts.filter(({ source }) => !source.relative.endsWith('legacy_routes.go'));
  const concreteText = concrete.map(({ context }) => context).join('\n');
  const hasCompatibility = concrete.some(({ source, context }) => /compatibilityHandler/.test(context) || source.relative.endsWith('compatibility.go'));
  // 路由数组常通过循环注册，源码中不会出现 HandleFunc("GET /path") 的直接形式。
  // 只要非测试、非兼容文件包含精确路径且未声明迁移占位，即视为有具体注册证据。
  const arrayRegistration = sources.some((source) => !source.isTest && !source.relative.endsWith('legacy_routes.go') && !source.relative.endsWith('compatibility.go') && source.text.includes(route.path) && !source.text.includes('MIGRATION_PENDING'));
  const hasConcrete = (concrete.length > 0 || arrayRegistration) && !hasCompatibility;
  best = concrete[0]?.source ?? best;
  let status = 'missing';
  if (hasCompatibility) status = 'compatibility';
  else if (hasConcrete) status = 'implemented';
  else if (markers.has('legacy_concrete_handler')) status = 'implemented';
  else if (markers.has('migration_pending') || markers.has('status_not_implemented')) status = 'pending';
  else if (markers.has('legacy_route')) status = 'compatibility';
  if (/\[\](?:any|map\[[^\]]+\][^\]]+)?\s*\{\s*\}/.test(concreteText) && !dynamicEmptyResponseRoutes.has(`${route.method} ${route.path}`)) {
    markers.add('fixed_empty_list');
    if (status === 'implemented') status = 'partial';
  }
  if (/TODO/.test(concreteText)) markers.add('todo');
  const combined = contexts.map(({ context }) => context).join('\n');
  const gaps = [];
  if (status === 'missing') gaps.push('新服务未发现对应路由注册');
  if (status === 'pending') gaps.push('返回 501/MIGRATION_PENDING 或 StatusNotImplemented');
  if (status === 'compatibility') gaps.push('仅由 compatibilityHandler/兼容占位承接');
  if (markers.has('fixed_empty_list')) gaps.push('检测到固定空列表响应');
  if (markers.has('todo')) gaps.push('实现附近存在 TODO');
  return {
    method: route.method,
    path: route.path,
    source: route.source,
    new: { status, source: best?.relative ?? null },
    domain: domainFor(route.path),
    auth: authFor(combined, route.path),
    persistence: persistenceFor(combined),
    test: matches.some((source) => fs.existsSync(source.file.replace(/\.go$/, '_test.go'))) ? 'present' : 'missing',
    gap: gaps,
    evidence: [...new Set(evidence)],
  };
}

function usage() {
  console.error('用法: node implementation-scan.mjs [--legacy <apps/workmesh-node>] [--project <apps/workmesh-server>] [--manifest <routes.json>] [--out <implementation-status.json>] [--markdown <function-checklist.md>]');
}

const args = process.argv.slice(2);
const option = (name, fallback) => { const i = args.indexOf(name); return i >= 0 ? args[i + 1] : fallback; };
if (args.includes('--help') || args.includes('-h')) { usage(); process.exit(0); }
const projectRoot = path.resolve(option('--project', process.cwd()));
const legacyRoot = path.resolve(option('--legacy', path.resolve(projectRoot, '../workmesh-node')));
const output = option('--out', null);
const markdownOutput = option('--markdown', null);
const manifestPath = option('--manifest', null);
let routes = scanLegacy(legacyRoot);
if (manifestPath) {
  const manifest = path.resolve(manifestPath);
  if (!fs.existsSync(manifest)) throw new Error(`路由清单不存在: ${manifest}`);
  routes = JSON.parse(fs.readFileSync(manifest, 'utf8')).routes ?? [];
}
if (!routes.length) throw new Error(`未发现旧 Core/Agent 路由: ${legacyRoot}`);
const sources = collectNewSources(projectRoot);
const interfaces = routes.map((route) => {
  // 标准库 ServeMux 的 GET 处理器按 HTTP 语义同时承接 HEAD。
  // 复用 GET 的实现证据，避免把静态资源 HEAD 误报为 missing。
  if (route.method === 'HEAD') return { ...inspectRoute({ ...route, method: 'GET' }, sources), method: 'HEAD' };
  return inspectRoute(route, sources);
});
const report = {
  schema: 1,
  generatedFrom: 'apps/workmesh-node/core+agent',
  project: path.relative(process.cwd(), projectRoot).replaceAll('\\', '/') || '.',
  generatedAt: new Date().toISOString(),
  routeCount: interfaces.length,
  summary: interfaces.reduce((acc, item) => { acc[item.new.status] = (acc[item.new.status] || 0) + 1; return acc; }, {}),
  interfaces,
};
const json = `${JSON.stringify(report, null, 2)}\n`;
if (output) {
  const target = path.resolve(output);
  fs.writeFileSync(target, json, 'utf8');
  console.log(`已生成实现状态报告 ${interfaces.length} 条: ${target}`);
} else {
  process.stdout.write(json);
}
if (markdownOutput) {
  const target = path.resolve(markdownOutput);
  const byDomain = new Map();
  for (const item of interfaces) {
    const list = byDomain.get(item.domain) ?? [];
    list.push(item);
    byDomain.set(item.domain, list);
  }
  const lines = [
    '<!-- SPDX-License-Identifier: GPL-3.0-only -->',
    '<!-- Copyright (c) 2026 WorkMesh contributors -->',
    '',
    '# WorkMesh 功能迁移逐路由清单',
    '',
    `基线来源：旧 \`apps/workmesh-node/core\` 与 \`agent\` 全源码，生成时间：${report.generatedAt}。`,
    `共 ${report.routeCount} 条接口：${Object.entries(report.summary).map(([key, value]) => `${key} ${value}`).join('、')}。`,
    '',
    '状态定义：`implemented`=已实现并有具体处理器，`partial`=具体处理器仍返回固定空数据或存在 TODO，`compatibility`=兼容占位，`pending`=迁移中，`missing`=未发现注册。',
    '',
  ];
  for (const [domain, items] of [...byDomain.entries()].sort(([a], [b]) => a.localeCompare(b))) {
    lines.push(`## ${domain}`, '', '| 状态 | 方法 | 路径 | 新实现 | 认证 | 持久化 | 测试 | 缺口 |', '| --- | --- | --- | --- | --- | --- | --- | --- |');
    for (const item of items.sort((a, b) => `${a.method} ${a.path}`.localeCompare(`${b.method} ${b.path}`))) {
      lines.push(`| ${item.new.status} | ${item.method} | \`${item.path}\` | ${item.new.source ?? '-'} | ${item.auth} | ${item.persistence} | ${item.test} | ${(item.gap ?? []).join('；') || '-'} |`);
    }
    lines.push('');
  }
  fs.writeFileSync(target, `${lines.join('\n')}\n`, 'utf8');
  console.log(`已生成逐路由功能清单: ${target}`);
}
