#!/usr/bin/env node
// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

/**
 * 从真实前端路由模块提取主菜单、子菜单和页面入口，作为主窗口验收清单。
 * 这是源码清单，不把页面标记为已验证；页面状态统一从 not-run 开始。
 */
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

const args = process.argv.slice(2);
const option = (name, fallback) => {
  const index = args.indexOf(name);
  return index >= 0 ? args[index + 1] : fallback;
};
const root = path.resolve(option('--frontend', 'web/src'));
const routerRoot = path.join(root, 'routers/modules');
const output = path.resolve(option('--out', 'docs/inventory/frontend-menu-inventory.json'));

function routeFiles() {
  return fs.readdirSync(routerRoot).filter((file) => file.endsWith('.ts')).sort();
}

function extract(file) {
  const absolute = path.join(routerRoot, file);
  const lines = fs.readFileSync(absolute, 'utf8').split(/\r?\n/);
  const records = [];
  const stack = [];
  for (let index = 0; index < lines.length; index += 1) {
    const match = lines[index].match(/^(\s*)path:\s*['"]([^'"]+)['"]/);
    if (!match) continue;
    const indent = match[1].length;
    while (stack.length && stack.at(-1).indent >= indent) stack.pop();
    const rawPath = match[2];
    const parent = stack.at(-1)?.path || '';
    const fullPath = rawPath.startsWith('/') ? rawPath : `${parent.replace(/\/$/, '')}/${rawPath}`;
    const detail = lines.slice(index + 1, index + 20).join('\n');
    const name = detail.match(/\bname:\s*['"]([^'"]+)['"]/)?.[1] || '';
    const title = detail.match(/\btitle:\s*['"]([^'"]+)['"]/)?.[1] || '';
    const view = detail.match(/import\(['"]([^'"]+)['"]\)/)?.[1] || '';
    const record = {
      module: file,
      path: fullPath,
      rawPath,
      name,
      title,
      view,
      level: stack.length,
      status: 'not-run',
    };
    records.push(record);
    stack.push({ indent, path: fullPath });
  }
  const source = fs.readFileSync(absolute);
  return { records, source: { file: `src/routers/modules/${file}`, sha256: crypto.createHash('sha256').update(source).digest('hex') } };
}

const extracted = routeFiles().map(extract);
const routes = extracted.flatMap((item) => item.records);
const menus = [
  { module: 'router.ts', path: '/', rawPath: '/', name: 'Home-Menu', title: 'menu.home', view: '@/views/home/index.vue', level: 0, status: 'not-run' },
  ...routes.filter((route) => route.level === 0 && route.rawPath.startsWith('/') && route.path !== '/error'),
];
const report = {
  schema: 1,
  generatedAt: new Date().toISOString(),
  generatedFrom: root,
  menuCount: menus.length,
  routeCount: routes.length,
  statuses: { 'not-run': routes.length },
  menus,
  routes,
  sources: extracted.map((item) => item.source),
  limitations: [
    '标题是 i18n key，不在清单中复制语言包文本；实际显示文字由语言包决定。',
    '动态权限、xpack 路由、运行时菜单和弹窗操作需要登录后人工/浏览器验收。',
    'not-run 只代表尚未完成真实页面和接口闭环测试。',
  ],
};
fs.mkdirSync(path.dirname(output), { recursive: true });
fs.writeFileSync(output, `${JSON.stringify(report, null, 2)}\n`, 'utf8');
console.log(`前端主菜单清单: ${menus.length} 个主菜单，${routes.length} 个路由 -> ${output}`);
