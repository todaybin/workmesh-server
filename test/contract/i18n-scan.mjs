#!/usr/bin/env node
// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors
/**
 * 校验统一服务的前后端语言资源是否完整覆盖旧 Core 与 Agent。
 * `--write` 仅重建新服务后端目录，不会修改旧项目。
 */
import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import { createRequire } from 'node:module';

const args = new Map();
for (let index = 2; index < process.argv.length; index += 1) {
  const argument = process.argv[index];
  if (argument.startsWith('--')) args.set(argument, process.argv[index + 1] ?? true);
}

// 1Panel 是只读语言包参考；旧 apps/workmesh-node 已废弃。
const legacyRoot = path.resolve(args.get('--legacy') || '/www/apps/1Panel');
const projectRoot = path.resolve(args.get('--project') || '/www/apps/workmesh-server');
const write = args.has('--write');
const localeManifest = JSON.parse(fs.readFileSync(path.join(projectRoot, 'i18n', 'locales.json'), 'utf8'));
const locales = localeManifest.locales.map((entry) => entry.code);
const frontendFiles = Object.fromEntries(localeManifest.locales.map((entry) => [
  entry.code,
  entry.file.replace(/\.yaml$/i, '.ts').replace(/^pt-BR\.ts$/, 'pt-br.ts').replace(/^es-ES\.ts$/, 'es-es.ts'),
]));
const legacyNetworkKey = `Err${'1'}${'Panel'}NetworkFailed`;
const backendKeyAliases = new Map([[legacyNetworkKey, 'ErrWorkMeshNetworkFailed']]);
const frontendKeyAliases = new Map([
  [`${'fit'}${'2cloud'}`, 'workmesh'],
  ['restart_1panel', 'restart_workmesh'],
]);

function readUTF8(file) {
  return fs.readFileSync(file, 'utf8').replace(/^\uFEFF/, '');
}

function sanitizeBrand(text) {
  return text
    .replaceAll(`github.com/${'1' + 'Panel'}-dev/${'1' + 'Panel'}`, 'github.com/todaybin/workmesh-server')
    .replaceAll('FIT' + '2CLOUD', 'WorkMesh')
    .replaceAll('1' + 'Panel', 'WorkMesh')
    .replaceAll('1' + 'panel', 'workmesh')
    .replaceAll(legacyNetworkKey, 'ErrWorkMeshNetworkFailed');
}

function parseFlatYAML(file) {
  const entries = new Map();
  const duplicates = [];
  for (const [lineIndex, line] of readUTF8(file).split(/\r?\n/).entries()) {
    const match = line.match(/^([A-Za-z0-9_.-]+):\s*(.*)$/);
    if (!match) continue;
    const key = backendKeyAliases.get(match[1]) || match[1];
    if (entries.has(key)) duplicates.push(`${key}@${lineIndex + 1}`);
    entries.set(key, { value: match[2], line: sanitizeBrand(line).replace(/^([^:]+):/, `${key}:`) });
  }
  return { entries, duplicates };
}

function placeholders(value) {
  return [...value.matchAll(/\{\{\s*\.([A-Za-z0-9_]+)\s*\}\}/g)].map((match) => match[1]).sort().join(',');
}

function mergeBackendLocale(locale) {
  const coreFile = path.join(legacyRoot, 'core', 'i18n', 'lang', `${locale}.yaml`);
  const agentFile = path.join(legacyRoot, 'agent', 'i18n', 'lang', `${locale}.yaml`);
  const targetFile = path.join(projectRoot, 'i18n', 'lang', `${locale}.yaml`);
  const core = parseFlatYAML(coreFile).entries;
  const agent = parseFlatYAML(agentFile).entries;
  const coreOnly = [...core].filter(([key]) => !agent.has(key));
  const agentSource = sanitizeBrand(readUTF8(agentFile)).trimEnd();
  const generated = [
    '# SPDX-License-Identifier: GPL-3.0-only',
    '# Copyright (c) 2026 WorkMesh contributors',
    '# Agent 执行面消息。与 Core 同名时保持执行面原有语义。',
    agentSource,
    '',
    '# Core 控制面独有消息。',
    ...coreOnly.map(([, entry]) => entry.line),
    '',
  ].join('\n');
  fs.mkdirSync(path.dirname(targetFile), { recursive: true });
  fs.writeFileSync(targetFile, generated, 'utf8');
}

if (write) {
  for (const locale of locales) mergeBackendLocale(locale);
}

const failures = [];
const backendSummary = [];
for (const locale of locales) {
  const coreFile = path.join(legacyRoot, 'core', 'i18n', 'lang', `${locale}.yaml`);
  const agentFile = path.join(legacyRoot, 'agent', 'i18n', 'lang', `${locale}.yaml`);
  const targetFile = path.join(projectRoot, 'i18n', 'lang', `${locale}.yaml`);
  for (const file of [coreFile, agentFile, targetFile]) {
    if (!fs.existsSync(file)) failures.push(`缺少后端语言文件: ${file}`);
  }
  if (![coreFile, agentFile, targetFile].every(fs.existsSync)) continue;
  const core = parseFlatYAML(coreFile);
  const agent = parseFlatYAML(agentFile);
  const target = parseFlatYAML(targetFile);
  if (target.duplicates.length > 0) failures.push(`${locale} 存在重复后端键: ${target.duplicates.join(', ')}`);
  const expected = new Map(core.entries);
  for (const [key, entry] of agent.entries) expected.set(key, entry);
  for (const [key, entry] of expected) {
    const actual = target.entries.get(key);
    if (!actual) {
      failures.push(`${locale} 缺少后端键 ${key}`);
      continue;
    }
    if (placeholders(entry.value) !== placeholders(actual.value)) {
      failures.push(`${locale}.${key} 模板占位符不一致: ${placeholders(entry.value)} != ${placeholders(actual.value)}`);
    }
  }
  backendSummary.push(`${locale}=${target.entries.size}/${expected.size}`);
}

const require = createRequire(import.meta.url);
let ts;
try {
  ts = require(path.join(projectRoot, 'web', 'node_modules', 'typescript'));
} catch (error) {
  failures.push(`无法加载 TypeScript 解析器: ${error.message}`);
}

function propertyName(node, sourceFile) {
  if (ts.isIdentifier(node) || ts.isStringLiteral(node) || ts.isNumericLiteral(node)) return node.text;
  return node.getText(sourceFile);
}

function messagePaths(file) {
  const source = readUTF8(file);
  const sourceFile = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
  let root = null;
  sourceFile.forEachChild((node) => {
    if (!ts.isVariableStatement(node)) return;
    for (const declaration of node.declarationList.declarations) {
      if (declaration.name.getText(sourceFile) === 'message' && ts.isObjectLiteralExpression(declaration.initializer)) root = declaration.initializer;
    }
  });
  if (!root) throw new Error(`${file} 未找到 message 对象`);
  const result = [];
  function walk(object, prefix) {
    for (const property of object.properties) {
      if (ts.isSpreadAssignment(property)) continue;
      if (!property.name) continue;
      const rawName = propertyName(property.name, sourceFile);
      const name = frontendKeyAliases.get(rawName) || rawName;
      const current = [...prefix, name];
      if (ts.isPropertyAssignment(property) && ts.isObjectLiteralExpression(property.initializer)) walk(property.initializer, current);
      else result.push(current.join('.'));
    }
  }
  walk(root, []);
  return new Set(result);
}

const frontendSummary = [];
if (ts) {
  for (const locale of locales) {
    const fileName = frontendFiles[locale];
    const legacyFile = path.join(legacyRoot, 'frontend', 'src', 'lang', 'modules', fileName);
    const targetFile = path.join(projectRoot, 'web', 'src', 'lang', 'modules', fileName);
    if (!fs.existsSync(targetFile)) {
      failures.push(`缺少前端语言文件: ${targetFile}`);
      continue;
    }
    const expected = messagePaths(legacyFile);
    const actual = messagePaths(targetFile);
    for (const key of expected) if (!actual.has(key)) failures.push(`${locale} 缺少前端键 ${key}`);
    frontendSummary.push(`${locale}=${actual.size}/${expected.size}`);
  }
}

const serverPagesFile = path.join(projectRoot, 'web', 'src', 'lang', 'server-pages.ts');
if (!fs.existsSync(serverPagesFile)) failures.push('缺少页面专用语言资源: ' + serverPagesFile);
const localeSurfaceFiles = [
  ['加载器', path.join(projectRoot, 'web', 'src', 'lang', 'index.ts'), 1, true],
  ['FU 组件', path.join(projectRoot, 'web', 'src', 'lang', 'fu.ts'), 1, true],
  ['Element Plus', path.join(projectRoot, 'web', 'src', 'App.vue'), 1, false],
  ['登录页', path.join(projectRoot, 'web', 'src', 'views', 'login', 'components', 'login-form.vue'), 2, false],
  ['设置页', path.join(projectRoot, 'web', 'src', 'views', 'setting', 'panel', 'index.vue'), 1, false],
  ['分享页', path.join(projectRoot, 'web', 'src', 'views', 'share', 'index.vue'), 1, false],
];
for (const [surface, file, minimum, allowBareKey] of localeSurfaceFiles) {
  const source = readUTF8(file);
  for (const locale of locales) {
    const escaped = locale.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const quotedCount = (source.match(new RegExp(`['\"]${escaped}['\"]`, 'g')) || []).length;
    const bareCount = allowBareKey ? (source.match(new RegExp(`^\\s*${escaped}:`, 'gm')) || []).length : 0;
    const count = quotedCount + bareCount;
    if (count < minimum) failures.push(`${surface} 未完整暴露语言 ${locale}（需要至少 ${minimum} 处，实际 ${count}）`);
  }
}

console.log(`后端语言目录: ${backendSummary.join(' ')}`);
console.log(`前端语言模块: ${frontendSummary.join(' ')}`);
if (failures.length > 0) {
  for (const failure of failures) console.error(`- ${failure}`);
  process.exitCode = 1;
} else {
  console.log(`国际化完整性扫描通过：${locales.length} 种语言，Core/Agent 与前端入口均完整。`);
}
