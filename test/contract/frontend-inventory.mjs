#!/usr/bin/env node
// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors
/**
 * 从 1Panel 前端静态提取 API/WS 字符串，作为参数与真实测试清单的入口。
 */
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

const args = process.argv.slice(2);
const option = (name, fallback) => {
  const index = args.indexOf(name);
  return index >= 0 ? args[index + 1] : fallback;
};
const root = path.resolve(option('--frontend', '/www/apps/1Panel/frontend'));
const output = path.resolve(option('--out', 'docs/inventory/frontend-api-inventory.json'));

function filesUnder(directory) {
  if (!fs.existsSync(directory)) return [];
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      if (['node_modules', 'dist', '.git', '.cache'].includes(entry.name)) return [];
      return filesUnder(file);
    }
    return /\.(ts|tsx|vue|js)$/.test(entry.name) ? [file] : [];
  });
}

function extract(file) {
  const source = fs.readFileSync(file, 'utf8');
  const records = [];
  const pattern = /\b(?:http|request|fetch)\.(get|post|put|patch|delete|download|upload|postLocalNode|postWithConfig)\s*<[^>]*>??\s*\(\s*(['"`])([^'"`\n]+)\2/g;
  const directPattern = /(['"`])((?:wss?:\/\/[^'"`\s]+|\/api\/v2\/[^'"`\s]+))\1/g;
  for (const match of source.matchAll(pattern)) {
    const method = match[1].toUpperCase();
    const value = match[3].replace(/\$\{[^}]+\}/g, ':param');
    const normalized = value.startsWith('/api/') ? value : `/api/v2/${value.replace(/^\/+/, '')}`;
    records.push({ path: normalized, method, source: path.relative(process.cwd(), file).replaceAll('\\', '/') });
  }
  for (const match of source.matchAll(directPattern)) {
    const value = match[2].replace(/\$\{[^}]+\}/g, ':param');
    records.push({ path: value, method: 'GET', source: path.relative(process.cwd(), file).replaceAll('\\', '/') });
  }
  const endpointPattern = /endpoint\s*:\s*(['"`])((?:wss?:\/\/|\/api\/v2\/)[^'"`]+)\1/g;
  for (const match of source.matchAll(endpointPattern)) records.push({ path: match[2], method: 'WS', source: path.relative(process.cwd(), file).replaceAll('\\', '/') });
  return { source, records };
}

const entries = new Map();
const sources = [];
for (const file of filesUnder(root)) {
  const { source, records } = extract(file);
  sources.push({ file: path.relative(root, file).replaceAll('\\', '/'), sha256: crypto.createHash('sha256').update(source).digest('hex') });
  for (const record of records) {
    const protocol = record.method === 'WS' || record.path.startsWith('ws') ? 'ws' : 'http';
    const key = `${protocol} ${record.path}`;
    const current = entries.get(key) ?? { protocol, path: record.path, methods: [], sources: [], status: 'not-run' };
    if (record.method !== 'WS' && !current.methods.includes(record.method)) current.methods.push(record.method);
    if (!current.sources.includes(record.source)) current.sources.push(record.source);
    entries.set(key, current);
  }
}
const routes = [...entries.values()].sort((a, b) => `${a.protocol} ${a.path}`.localeCompare(`${b.protocol} ${b.path}`));
const report = { schema: 1, generatedAt: new Date().toISOString(), generatedFrom: root, sourceCount: sources.length, routeCount: routes.length, statuses: { 'not-run': routes.length }, routes, sources, limitations: ['静态字符串提取不推导变量拼接、运行时路由、HTTP 方法和 type/operate 参数分支', '本清单只表示前端存在调用字面量，不代表后端实现或真实请求已通过', 'WS 可能由 endpoint 参数间接传入，需结合页面和终端组件人工复核'] };
fs.mkdirSync(path.dirname(output), { recursive: true });
fs.writeFileSync(output, `${JSON.stringify(report, null, 2)}\n`, 'utf8');
console.log(`已生成 ${routes.length} 条前端 HTTP/WS 调用清单: ${output}`);
