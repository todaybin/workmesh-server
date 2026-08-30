#!/usr/bin/env node
// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors
/**
 * 盘点旧 Core/Agent 中不以路由为中心的隐藏能力目录。
 * 脚本只读源码，避免把新增初始化、作业或中间件文件遗漏在迁移清单之外。
 */
import fs from 'node:fs';
import path from 'node:path';

const args = new Map();
for (let i = 2; i < process.argv.length; i += 1) {
  if (process.argv[i].startsWith('--')) args.set(process.argv[i], process.argv[i + 1]);
}

const legacy = path.resolve(args.get('--legacy') || 'apps/workmesh-node');
const project = path.resolve(args.get('--project') || 'apps/workmesh-server');
const out = args.get('--out') ? path.resolve(args.get('--out')) : null;
const categories = ['init', 'middleware', 'i18n', 'log', 'cron', 'ws', 'sse', 'command', 'background'];

function filesUnder(root) {
  if (!fs.existsSync(root)) return [];
  const result = [];
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    const full = path.join(root, entry.name);
    if (entry.isDirectory()) result.push(...filesUnder(full));
    else if (entry.isFile() && /\.(go|yaml|yml|json|toml)$/.test(entry.name)) result.push(full);
  }
  return result;
}

const checklistPath = path.join(project, 'docs', 'migration', 'hidden-function-checklist.md');
const checklist = fs.existsSync(checklistPath) ? fs.readFileSync(checklistPath, 'utf8') : '';
const sectionHeadings = {
  init: '## 运行时初始化与生命周期',
  middleware: '## 安全中间件与授权',
  i18n: '## 国际化、错误和任务基础设施',
  log: '## 国际化、错误和任务基础设施',
  cron: '## 后台作业与数据维护',
  ws: '## 协议与执行通道',
  sse: '## 协议与执行通道',
  command: '## 2026-08-30 核心认证与执行入口',
  background: '## 后台作业与数据维护',
};
const report = {
  schema: 1,
  legacy: path.relative(process.cwd(), legacy).replaceAll('\\', '/'),
  checklist: path.relative(process.cwd(), checklistPath).replaceAll('\\', '/'),
  categories: {},
  totals: { files: 0, categories: categories.length },
};

for (const category of categories) {
  const files = [];
  for (const area of ['core', 'agent']) {
    const root = path.join(legacy, area, category);
    if (fs.existsSync(root)) {
      for (const file of filesUnder(root)) {
        files.push(path.relative(legacy, file).replaceAll('\\', '/'));
      }
    }
  }
  // ws/sse/命令和后台作业并不总有独立目录，按源码特征补充扫描。
  if (['ws', 'sse', 'command', 'background'].includes(category)) {
    for (const area of ['core', 'agent']) {
      const root = path.join(legacy, area);
      for (const file of filesUnder(root)) {
        const relative = path.relative(legacy, file).replaceAll('\\', '/');
        const source = fs.readFileSync(file, 'utf8');
        const matched = {
          ws: /websocket|gorilla\/websocket|Upgrader|WSHub/i.test(source) || /(^|\/)(ws|websocket)(\/|\.)/i.test(relative),
          sse: /text\/event-stream|EventSource|Server-Sent Events|SSE/i.test(source) || /(^|\/)(sse|event)(\/|\.)/i.test(relative),
          command: /exec\.Command(?:Context)?|RunCommand|命令执行|command runner/i.test(source) || relative.startsWith(`${area}/cmd/`),
          background: /time\.NewTicker|time\.Tick|go\s+func\s*\(|cron|scheduler|后台任务/i.test(source) || /(^|\/)(cron|job|jobs|task|scheduler)(\/|\.)/i.test(relative),
        }[category];
        if (matched) files.push(relative);
      }
    }
  }
  files.sort();
  report.categories[category] = {
    sourceFiles: files,
    sourceCount: files.length,
    checklistSectionPresent: checklist.includes(sectionHeadings[category]),
  };
  report.totals.files += files.length;
}

const missingSections = Object.entries(report.categories).filter(([, value]) => !value.checklistSectionPresent).map(([key]) => key);
if (out) {
  fs.mkdirSync(path.dirname(out), { recursive: true });
  fs.writeFileSync(out, `${JSON.stringify(report, null, 2)}\n`, 'utf8');
}
console.log(`隐藏能力扫描：${report.totals.files} 个源码文件，${report.totals.categories} 个目录类别`);
for (const [category, value] of Object.entries(report.categories)) {
  console.log(`- ${category}: ${value.sourceCount} 个文件，清单章节=${value.checklistSectionPresent ? '存在' : '缺失'}`);
}
if (missingSections.length > 0) {
  console.error(`隐藏能力清单缺少章节：${missingSections.join(', ')}`);
  process.exitCode = 1;
}
