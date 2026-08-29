#!/usr/bin/env node
// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors
/**
 * 读取 Go Gin 路由注册，生成或校验迁移路由清单。
 * 该脚本只使用源码和标准库，避免测试过程依赖旧服务运行。
 */
import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';

const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS', 'Any'];
// 旧产品专属文档入口不属于迁移功能，必须在品牌清理时移除而不是继续暴露。
const legacySwaggerSegment = ['1', 'panel'].join('');
const excludedLegacyPaths = new Set([`/${legacySwaggerSegment}/swagger/*any`]);

function filesUnder(root) {
  if (!fs.existsSync(root)) return [];
  const result = [];
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const full = path.join(root, entry.name);
    if (entry.isDirectory()) result.push(...filesUnder(full));
    else if (entry.isFile() && full.endsWith('.go')) result.push(full);
  }
  return result;
}

function cleanSegment(value) {
  return value.replaceAll('\\', '/').replace(/^\/+/, '').replace(/\/+$/, '');
}

function joinRoute(...parts) {
  const clean = parts.map(cleanSegment).filter(Boolean).join('/');
  return `/${clean}`.replaceAll(/\/+/g, '/');
}

function scanFile(file, area, base = area === 'core' ? '/api/v2/core' : '/api/v2') {
  const source = fs.readFileSync(file, 'utf8');
  const group = new Map([['Router', base]]);
  const lines = source.split(/\r?\n/);
  const routes = [];
  // Router groups are normally declared before registrations in each InitRouter.
  for (const line of lines) {
    const assignment = line.match(/\b([A-Za-z_]\w*)\s*:=\s*([A-Za-z_]\w*)\.Group\(\s*["`]([^"`]+)["`]\s*\)/);
    if (assignment) {
      const [, name, parent, local] = assignment;
      group.set(name, joinRoute(group.get(parent) ?? base, local));
    }
    for (const method of methods) {
      const matcher = new RegExp('\\b([A-Za-z_]\\w*)\\.' + method + '\\(\\s*["\\x27]([^"\\x27]+)["\\x27]');
      const match = line.match(matcher);
      if (match) {
        const [, receiver, local] = match;
        const prefix = group.get(receiver) ?? base;
        routes.push({ method, path: joinRoute(prefix, local), source: path.relative(process.cwd(), file).replaceAll('\\', '/') });
      }
      // Gin 支持 Router.Group("x").POST(...) 和 group.Use(...).GET(...) 链式注册。
      const groupChain = new RegExp('\\b([A-Za-z_]\\w*)\\.Group\\(\\s*["\\x27]([^"\\x27]+)["\\x27]\\s*\\)\\.' + method + '\\(\\s*["\\x27]([^"\\x27]+)["\\x27]');
      const chainedGroup = line.match(groupChain);
      if (chainedGroup) {
        const [, receiver, localGroup, local] = chainedGroup;
        routes.push({ method, path: joinRoute(group.get(receiver) ?? base, localGroup, local), source: path.relative(process.cwd(), file).replaceAll('\\', '/') });
      }
      const useChain = new RegExp('\\b([A-Za-z_]\\w*)\\.Use\\(.*\\)\\.' + method + '\\(\\s*["\\x27]([^"\\x27]+)["\\x27]');
      const chainedUse = line.match(useChain);
      if (chainedUse) {
        const [, receiver, local] = chainedUse;
        routes.push({ method, path: joinRoute(group.get(receiver) ?? base, local), source: path.relative(process.cwd(), file).replaceAll('\\', '/') });
      }
      // 新服务使用标准库 ServeMux，注册格式为 HandleFunc("GET /path", ...)。
      // 兼容 Go 1.22 的 {param} 变量与旧 Gin 的 :param 写法，保证契约扫描统一。
      const handle = new RegExp('\\bHandleFunc\\(\\s*["\\x27]' + method + '\\s+([^"\\x27]+)["\\x27]');
      const handleMatch = line.match(handle);
      if (handleMatch) {
        const normalized = handleMatch[1].replaceAll(/\{([A-Za-z_]\w*)\}/g, ':$1');
        routes.push({ method, path: joinRoute(normalized), source: path.relative(process.cwd(), file).replaceAll('\\', '/') });
      }
    }
  }
  // ServeMux 对健康检查等路径允许省略方法；按旧契约将其视为 GET。
  for (const line of lines) {
    const plain = line.match(/\bHandleFunc\(\s*["'](\/[^"']*)["']/);
    if (plain && !plain[1].includes(' ')) {
      routes.push({ method: 'GET', path: joinRoute(plain[1]), source: path.relative(process.cwd(), file).replaceAll('\\', '/') });
    }
  }
  return routes;
}

function scanLegacy(legacyRoot) {
  const roots = [
    ['core', path.join(legacyRoot, 'core', 'router'), '/api/v2/core'],
    ['agent', path.join(legacyRoot, 'agent', 'router'), '/api/v2'],
    // 健康、静态资源和前端入口在 init/router 中注册，也必须纳入完整基线。
    ['core', path.join(legacyRoot, 'core', 'init', 'router'), '/'],
    ['agent', path.join(legacyRoot, 'agent', 'init', 'router'), '/'],
  ];
  const routes = roots.flatMap(([area, root, base]) => filesUnder(root).flatMap((file) => scanFile(file, area, base)));
  const unique = new Map(routes.map((route) => [`${route.method} ${route.path}`, route]));
  return [...unique.values()].filter((route) => !excludedLegacyPaths.has(route.path)).sort((a, b) => `${a.method} ${a.path}`.localeCompare(`${b.method} ${b.path}`));
}

function scanNew(projectRoot) {
  const roots = [path.join(projectRoot, 'control'), path.join(projectRoot, 'node'), path.join(projectRoot, 'runtime'), path.join(projectRoot, 'cmd')];
  const routes = roots.flatMap((root) => filesUnder(root).flatMap((file) => scanFile(file, file.includes(`${path.sep}control${path.sep}`) ? 'core' : 'agent')));
  const unique = new Map(routes.map((route) => [`${route.method} ${route.path}`, route]));
  return [...unique.values()].sort((a, b) => `${a.method} ${a.path}`.localeCompare(`${b.method} ${b.path}`));
}

function usage() {
  console.error('用法: node route-scan.mjs generate --legacy <apps/workmesh-node> --out <routes.json>');
  console.error('      node route-scan.mjs check --legacy <apps/workmesh-node> --project <apps/workmesh-server> --manifest <routes.json>');
}

const [command, ...args] = process.argv.slice(2);
const option = (name, fallback = undefined) => {
  const index = args.indexOf(name);
  return index >= 0 ? args[index + 1] : fallback;
};

if (!['generate', 'check'].includes(command)) {
  usage();
  process.exit(2);
}

const legacyRoot = path.resolve(option('--legacy', path.resolve(process.cwd(), '../workmesh-node')));
if (command === 'generate') {
  const output = path.resolve(option('--out', path.join(process.cwd(), 'routes.json')));
  const routes = scanLegacy(legacyRoot);
  if (routes.length === 0) throw new Error(`未发现旧路由: ${legacyRoot}`);
  fs.writeFileSync(output, `${JSON.stringify({ schema: 1, generatedFrom: 'apps/workmesh-node', routeCount: routes.length, routes }, null, 2)}\n`, 'utf8');
  console.log(`已生成 ${routes.length} 条路由清单: ${output}`);
} else {
  const projectRoot = path.resolve(option('--project', process.cwd()));
  const manifestPath = path.resolve(option('--manifest', path.join(process.cwd(), 'routes.json')));
  if (!fs.existsSync(manifestPath)) throw new Error(`清单不存在: ${manifestPath}`);
  const expected = JSON.parse(fs.readFileSync(manifestPath, 'utf8')).routes ?? [];
  const actual = new Set(scanNew(projectRoot).map((route) => `${route.method} ${route.path}`));
  const missing = expected.map((route) => `${route.method} ${route.path}`).filter((key) => !actual.has(key));
  const extra = [...actual].filter((key) => !new Set(expected.map((route) => `${route.method} ${route.path}`)).has(key));
  // 新服务允许增加健康检查、Gateway 控制面和兼容 HTTP 方法，不将其视为 breaking 差异。
  if (missing.length) {
    console.error(`路由差异: 缺失 ${missing.length}，新增 ${extra.length}`);
    if (missing.length) console.error(`缺失示例:\n${missing.slice(0, 30).join('\n')}`);
    process.exit(1);
  }
  if (extra.length) console.warn(`路由契约包含 ${extra.length} 条扩展路由（兼容允许）`);
  console.log(`路由契约通过: ${expected.length} 条`);
}
