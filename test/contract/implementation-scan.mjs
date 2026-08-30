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

function filesUnder(root) {
  if (!fs.existsSync(root)) return [];
  const files = [];
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const full = path.join(root, entry.name);
    if (entry.isDirectory()) files.push(...filesUnder(full));
    else if (entry.isFile() && full.endsWith('.go')) files.push(full);
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
  const normalizedLegacy = normalizePath(route.path);
  const matches = [];
  for (const source of sources) {
    if (source.isTest) continue;
    const text = source.text;
    if (route.path === '/') {
      if (/\bHandleFunc\(\s*["'](?:GET\s+)?\/["']/.test(text)) matches.push(source);
      continue;
    }
    const routeRe = new RegExp('(?:HandleFunc|\\.(?:GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|Any))\\(\\s*["\\\x27](?:' + route.method + '\\s+)?' + normalizedLegacy.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replaceAll(':', '\\{[^}]+\\}') + '(?:["\\\x27]|/)', 'i');
    const literal = text.includes(`${route.method} ${route.path}`) || text.includes(`${route.method} ${normalizePath(route.path)}`);
    if (literal || routeRe.test(text)) matches.push(source);
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
  for (const { source, context } of contexts) {
    for (const [marker, key] of MARKERS) {
      if (context.includes(marker)) { markers.add(key); evidence.push(`${key}:${source.relative}`); }
    }
    if (/compatibilityHandler/.test(context)) best = source;
  }
  if (matches.some((source) => source.relative.endsWith('legacy_routes.go'))) markers.add('legacy_route');
  const concrete = contexts.filter(({ source }) => !source.relative.endsWith('legacy_routes.go'));
  const concreteText = concrete.map(({ context }) => context).join('\n');
  const hasCompatibility = concrete.some(({ source, context }) => /compatibilityHandler/.test(context) || source.relative.endsWith('compatibility.go'));
  const hasConcrete = concrete.length > 0 && !hasCompatibility;
  best = concrete[0]?.source ?? best;
  let status = 'missing';
  if (hasCompatibility) status = 'compatibility';
  else if (hasConcrete) status = 'implemented';
  else if (markers.has('migration_pending') || markers.has('status_not_implemented')) status = 'pending';
  else if (markers.has('legacy_route')) status = 'compatibility';
  if (/\[\](?:any|map\[[^\]]+\][^\]]+)?\s*\{\s*\}/.test(concreteText)) {
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
  console.error('用法: node implementation-scan.mjs [--legacy <apps/workmesh-node>] [--project <apps/workmesh-server>] [--out <implementation-status.json>]');
}

const args = process.argv.slice(2);
const option = (name, fallback) => { const i = args.indexOf(name); return i >= 0 ? args[i + 1] : fallback; };
if (args.includes('--help') || args.includes('-h')) { usage(); process.exit(0); }
const projectRoot = path.resolve(option('--project', process.cwd()));
const legacyRoot = path.resolve(option('--legacy', path.resolve(projectRoot, '../workmesh-node')));
const output = option('--out', null);
const routes = scanLegacy(legacyRoot);
if (!routes.length) throw new Error(`未发现旧 Core/Agent 路由: ${legacyRoot}`);
const sources = collectNewSources(projectRoot);
const interfaces = routes.map((route) => inspectRoute(route, sources));
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
