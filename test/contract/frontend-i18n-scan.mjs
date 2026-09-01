#!/usr/bin/env node
// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors
// 校验前端所有路由页面使用的静态语言键在 12 种语言中均有定义。
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const projectRoot = path.resolve(process.argv.includes('--project') ? process.argv[process.argv.indexOf('--project') + 1] : 'apps/workmesh-server');
const webRoot = path.join(projectRoot, 'web');
const sourceRoot = path.join(webRoot, 'src');
const localeManifest = JSON.parse(fs.readFileSync(path.join(projectRoot, 'i18n', 'locales.json'), 'utf8'));
const locales = localeManifest.locales.map((entry) => entry.code);
const files = Object.fromEntries(localeManifest.locales.map((entry) => [entry.code, entry.file.replace(/\.yaml$/i, '.ts').replace(/^pt-BR\.ts$/, 'pt-br.ts').replace(/^es-ES\.ts$/, 'es-es.ts').replace(/^zh\.ts$/, 'zh.ts').replace(/^zh-Hant\.ts$/, 'zh-Hant.ts')]));
const require = createRequire(import.meta.url);
const ts = require(path.join(webRoot, 'node_modules', 'typescript'));

function walk(dir) {
  const result = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) result.push(...walk(full));
    else if (/\.(vue|ts|tsx)$/.test(entry.name)) result.push(full);
  }
  return result;
}

function sourceText(file) { return fs.readFileSync(file, 'utf8').replace(/^\uFEFF/, ''); }

function languageKeys(file) {
  const source = sourceText(file);
  const sf = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
  let root;
  sf.forEachChild((node) => {
    if (!ts.isVariableStatement(node)) return;
    for (const declaration of node.declarationList.declarations) {
      if (declaration.name.getText(sf) === 'message' && ts.isObjectLiteralExpression(declaration.initializer)) root = declaration.initializer;
    }
  });
  if (!root) throw new Error(`语言文件缺少 message 对象: ${file}`);
  const keys = new Set();
  const nameOf = (node) => (ts.isIdentifier(node) || ts.isStringLiteral(node) ? node.text : node.getText(sf));
  const walkObject = (object, prefix) => {
    for (const property of object.properties) {
      if (!property.name || ts.isSpreadAssignment(property)) continue;
      const current = [...prefix, nameOf(property.name)];
      if (ts.isPropertyAssignment(property) && ts.isObjectLiteralExpression(property.initializer)) walkObject(property.initializer, current);
      else keys.add(current.join('.'));
    }
  };
  walkObject(root, []);
  return keys;
}

const languageSets = new Map();
for (const locale of locales) languageSets.set(locale, languageKeys(path.join(sourceRoot, 'lang', 'modules', files[locale])));

// 页面专用语言资源独立于旧版主模块；其公共键会覆盖所有语言，局部语言只覆盖值。
const serverPagesFile = path.join(sourceRoot, 'lang', 'server-pages.ts');
const serverPagesAst = ts.createSourceFile(serverPagesFile, sourceText(serverPagesFile), ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
let serverPagesRoot;
serverPagesAst.forEachChild((node) => {
  if (!ts.isVariableStatement(node)) return;
  for (const declaration of node.declarationList.declarations) {
    if (declaration.name.getText(serverPagesAst) === 'common' && ts.isObjectLiteralExpression(declaration.initializer)) serverPagesRoot = declaration.initializer;
  }
});
const serverPageKeys = new Set();
if (serverPagesRoot) {
  const collect = (object, prefix) => {
    for (const property of object.properties) {
      if (!property.name || ts.isSpreadAssignment(property)) continue;
      const name = ts.isIdentifier(property.name) || ts.isStringLiteral(property.name) ? property.name.text : property.name.getText(serverPagesAst);
      const current = [...prefix, name];
      if (ts.isPropertyAssignment(property) && ts.isObjectLiteralExpression(property.initializer)) collect(property.initializer, current);
      else serverPageKeys.add(current.join('.'));
    }
  };
  collect(serverPagesRoot, []);
}
for (const set of languageSets.values()) for (const key of serverPageKeys) set.add(key);

// FU 组件语言资源独立于主 message 模块，合并后再校验其调用方。
const fuFile = path.join(sourceRoot, 'lang', 'fu.ts');
const fuAst = ts.createSourceFile(fuFile, sourceText(fuFile), ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
let fuRoot;
fuAst.forEachChild((node) => {
  if (!ts.isVariableStatement(node)) return;
  for (const declaration of node.declarationList.declarations) {
    if (declaration.name.getText(fuAst) === 'fuLocales' && ts.isObjectLiteralExpression(declaration.initializer)) fuRoot = declaration.initializer;
  }
});
const fuSets = new Map();
const flattenObject = (object, prefix, target) => {
  for (const property of object.properties) {
    if (!property.name || ts.isSpreadAssignment(property)) continue;
    const name = ts.isIdentifier(property.name) || ts.isStringLiteral(property.name) ? property.name.text : property.name.getText(fuAst);
    const current = [...prefix, name];
    if (ts.isPropertyAssignment(property) && ts.isObjectLiteralExpression(property.initializer)) flattenObject(property.initializer, current, target);
    else target.add(current.join('.'));
  }
};
for (const locale of locales) {
  const target = new Set();
  const property = fuRoot?.properties.find((entry) => entry.name && ((ts.isIdentifier(entry.name) || ts.isStringLiteral(entry.name)) ? entry.name.text : '') === locale);
  if (property && ts.isPropertyAssignment(property) && ts.isObjectLiteralExpression(property.initializer)) flattenObject(property.initializer, [], target);
  fuSets.set(locale, target);
}
const baseline = languageSets.get('zh');
const failures = [];
for (const locale of locales) {
  const set = languageSets.get(locale);
  for (const key of baseline) if (!set.has(key)) failures.push(`${locale} 缺少语言键 ${key}`);
}

const staticKeyPattern = /(?:\$t|i18n\.global\.t)\(\s*(['"])([^'"\\]+)\1\s*(?:,|\))/g;
const sourceFiles = walk(sourceRoot);
const usedKeys = new Map();
const hardcodedUi = [];
for (const file of sourceFiles) {
  const source = sourceText(file);
  for (const match of source.matchAll(staticKeyPattern)) {
    const key = match[2];
    if (key.endsWith('.') || key.includes(' ')) continue;
    if (!usedKeys.has(key)) usedKeys.set(key, file);
  }
  const relative = path.relative(projectRoot, file).replaceAll(path.sep, '/');
  const targetedPage = /web\/src\/views\//.test(relative);
  if (targetedPage && file.endsWith('.vue')) {
    for (const [lineIndex, line] of source.split(/\r?\n/).entries()) {
      const trimmed = line.trim();
      if (!/[\u3400-\u9fff]/.test(trimmed) || trimmed.startsWith('//') || trimmed.startsWith('<!--') || trimmed.startsWith('*')) continue;
      if (/中文\(|日本語|繁體/.test(trimmed)) continue;
      if (/\b(?:label|title|placeholder)=|>[^<{]*[\u3400-\u9fff][^<{]*<|ElMessage(?:Box)?\.(?:success|warning|error|confirm|alert)|Msg(?:Success|Error)\(/.test(trimmed)) {
        hardcodedUi.push(relative + ':' + (lineIndex + 1));
      }
    }
  }
}
// 路由标题同样会直接进入侧边栏/页签，禁止在路由元数据中写入中文可见文案。
for (const file of walk(path.join(sourceRoot, 'routers'))) {
  const relative = path.relative(projectRoot, file).replaceAll(path.sep, '/');
  for (const [lineIndex, line] of sourceText(file).split(/\r?\n/).entries()) {
    if (/title:\s*['"][^'"]*[\u3400-\u9fff]/.test(line)) hardcodedUi.push(relative + ':' + (lineIndex + 1));
  }
}
for (const [key, file] of usedKeys) {
  for (const locale of locales) {
    const set = key.startsWith('fu.') ? fuSets.get(locale) : languageSets.get(locale);
    if (!set.has(key)) failures.push(`${path.relative(projectRoot, file)} 使用未定义语言键 ${key}（${locale}）`);
  }
}

console.log(`前端语言模块: ${locales.length} 种，页面源码: ${sourceFiles.length} 个，静态语言键: ${usedKeys.size} 个`);
if (failures.length) {
  for (const failure of failures) console.error(`- ${failure}`);
  process.exitCode = 1;
} else {
  if (hardcodedUi.length) {
    for (const location of hardcodedUi) console.error('- 页面存在硬编码用户文案: ' + location);
    process.exitCode = 1;
  }
  if (hardcodedUi.length) process.exit(0);
  console.log('前端全页面静态语言键校验通过。');
}
