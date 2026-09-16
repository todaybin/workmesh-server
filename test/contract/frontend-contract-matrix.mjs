#!/usr/bin/env node
// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

/**
 * 从当前前端源码生成 HTTP/WS 合约测试矩阵。
 *
 * 该脚本只生成静态测试计划，不发送业务请求，也不修改任何清单中的
 * not-run 状态。真实请求必须由后续带凭据的执行器逐条写入独立证据文件。
 */
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';

const args = process.argv.slice(2);
const option = (name, fallback) => {
    const index = args.indexOf(name);
    return index >= 0 ? args[index + 1] : fallback;
};

const root = path.resolve(option('--root', '.'));
const menuFile = path.resolve(root, option('--menu', 'docs/inventory/frontend-menu-inventory.json'));
const apiFile = path.resolve(root, option('--api', 'docs/inventory/frontend-api-inventory.json'));
const output = path.resolve(root, option('--out', 'docs/inventory/frontend-http-ws-contract-matrix.json'));
const markdownOutput = path.resolve(root, option('--markdown', 'docs/inventory/frontend-http-ws-contract-matrix.md'));
const frontendRoot = path.join(root, 'web/src');
const apiModuleRoot = path.join(frontendRoot, 'api/modules');

const menuInventory = JSON.parse(fs.readFileSync(menuFile, 'utf8'));
const apiInventory = JSON.parse(fs.readFileSync(apiFile, 'utf8'));

const sha256 = (file) => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const sourceLine = (source, offset) => source.slice(0, offset).split(/\r?\n/).length;
const stripV2 = (value) => String(value || '').replace(/^\/api\/v2/, '') || '/';
const normalizePath = (value) => {
    const stripped = stripV2(value).replace(/\$\{[^}]+\}/g, ':param').replace(/\/+/g, '/');
    return stripped.startsWith('/') ? stripped : `/${stripped}`;
};

const apiModuleSource = new Map();
for (const file of fs.readdirSync(apiModuleRoot).filter((file) => file.endsWith('.ts')).sort()) {
    const absolute = path.join(apiModuleRoot, file);
    apiModuleSource.set(file, { absolute, relative: path.relative(root, absolute).replaceAll(path.sep, '/'), source: fs.readFileSync(absolute, 'utf8') });
}

function skipSpaces(source, position) {
    let current = position;
    while (/\s/.test(source[current] || '')) current += 1;
    return current;
}

function scanBalanced(source, position, opening, closing) {
    if (source[position] !== opening) return position;
    let depth = 0;
    let quote = '';
    for (let current = position; current < source.length; current += 1) {
        const char = source[current];
        const previous = source[current - 1];
        if (quote) {
            if (char === quote && previous !== '\\') quote = '';
            continue;
        }
        if (char === '`' || char === '"' || char === "'") {
            quote = char;
            continue;
        }
        if (char === opening) depth += 1;
        if (char === closing) {
            depth -= 1;
            if (depth === 0) return current;
        }
    }
    return source.length - 1;
}

function readQuoted(source, position) {
    const quote = source[position];
    if (!['`', '"', "'"].includes(quote)) return { value: '', end: position };
    let current = position + 1;
    for (; current < source.length; current += 1) {
        if (source[current] === quote && source[current - 1] !== '\\') {
            return { value: source.slice(position + 1, current), end: current };
        }
    }
    return { value: source.slice(position + 1), end: source.length - 1 };
}

function extractFunctionDeclarations(source) {
    const declarations = [];
    const pattern = /export\s+const\s+([A-Za-z0-9_$]+)\s*=\s*\(/g;
    let match;
    while ((match = pattern.exec(source))) {
        const open = source.indexOf('(', match.index + match[0].length - 1);
        const close = scanBalanced(source, open, '(', ')');
        const signature = source.slice(open + 1, close).replace(/\s+/g, ' ').trim();
        declarations.push({ name: match[1], start: match.index, end: close, signature });
    }
    return declarations;
}

function extractApiCalls(moduleName, descriptor) {
    const { source } = descriptor;
    const functions = extractFunctionDeclarations(source);
    const methods = ['get', 'post', 'put', 'delete', 'patch', 'request', 'postLocalNode', 'getLocalNode', 'upload', 'download'];
    const callPattern = new RegExp(`http\\.(${methods.join('|')})`, 'g');
    const calls = [];
    let match;
    while ((match = callPattern.exec(source))) {
        const functionInfo = [...functions].reverse().find((item) => item.start < match.index);
        let position = skipSpaces(source, match.index + match[0].length);
        let responseType = null;
        if (source[position] === '<') {
            const genericEnd = scanBalanced(source, position, '<', '>');
            responseType = source.slice(position + 1, genericEnd).replace(/\s+/g, ' ').trim();
            position = genericEnd + 1;
        }
        position = skipSpaces(source, position);
        if (source[position] !== '(') continue;
        position = skipSpaces(source, position + 1);
        const quoted = readQuoted(source, position);
        if (!quoted.value) continue;
        const normalized = normalizePath(quoted.value);
        const callEnd = source.indexOf('\n', quoted.end);
        const preview = source.slice(match.index, callEnd < 0 ? quoted.end + 180 : callEnd).replace(/\s+/g, ' ').trim();
        const functionSourceEnd = source.indexOf('\nexport ', match.index + 1);
        const functionSource = source.slice(functionInfo?.start || match.index, functionSourceEnd < 0 ? source.length : functionSourceEnd);
        calls.push({
            module: moduleName,
            function: functionInfo?.name || null,
            method: match[1].toUpperCase(),
            path: normalized,
            literalPath: quoted.value,
            responseType: responseType || null,
            signature: functionInfo?.signature || null,
            callPreview: preview,
            sourceFile: descriptor.relative,
            line: sourceLine(source, match.index),
            functionSource,
        });
    }
    return calls;
}

const callsByModule = new Map();
for (const [moduleName, descriptor] of apiModuleSource) callsByModule.set(moduleName, extractApiCalls(moduleName, descriptor));

function interfaceSources(moduleName) {
    const descriptor = apiModuleSource.get(moduleName);
    if (!descriptor) return [];
    const files = [];
    for (const match of descriptor.source.matchAll(/from\s+['"]\.\.\/interface\/([^'"/]+)['"]/g)) {
        const file = path.join(frontendRoot, 'api/interface', `${match[1]}.ts`);
        if (fs.existsSync(file)) files.push({ file: path.relative(root, file).replaceAll(path.sep, '/'), sha256: sha256(file) });
    }
    for (const match of descriptor.source.matchAll(/from\s+['"]@\/api\/interface\/([^'"/]+)['"]/g)) {
        const file = path.join(frontendRoot, 'api/interface', `${match[1]}.ts`);
        if (fs.existsSync(file)) files.push({ file: path.relative(root, file).replaceAll(path.sep, '/'), sha256: sha256(file) });
    }
    return [...new Map(files.map((item) => [item.file, item])).values()];
}

function requestVariants(call) {
    const fields = ['type', 'operate', 'logType', 'source', 'scope'];
    return fields.filter((field) => new RegExp(`\\b${field}\\b`).test(call.signature || '') || new RegExp(`\\b${field}\\b`).test(call.functionSource || '')).map((field) => ({
        field,
        source: call.sourceFile,
        status: 'not-run',
    }));
}

function matchCall(endpoint) {
    const sourceModule = (endpoint.sources || []).map((source) => source.match(/\/api\/modules\/([^/]+)\.ts$/)?.[1]).filter(Boolean).map((name) => `${name}.ts`);
    const endpointPath = normalizePath(endpoint.path);
    const candidates = sourceModule.flatMap((module) => callsByModule.get(module) || []);
    const exact = candidates.find((call) => call.path === endpointPath);
    if (exact) return exact;
    const compatible = candidates.filter((call) => endpointPath.startsWith(call.path) || call.path.startsWith(endpointPath));
    compatible.sort((left, right) => Math.abs(left.path.length - endpointPath.length) - Math.abs(right.path.length - endpointPath.length));
    return compatible[0] || null;
}

const menuByName = new Map(menuInventory.menus.map((menu) => [menu.name, menu]));
const menuByPath = new Map(menuInventory.menus.map((menu) => [menu.path, menu]));
const routeByName = new Map(menuInventory.routes.map((route) => [route.name, route]));

function rootMenuForRoute(route) {
    let current = route;
    const seen = new Set();
    while (current?.parentName && !seen.has(current.name)) {
        seen.add(current.name);
        const parent = routeByName.get(current.parentName);
        if (!parent) break;
        current = parent;
    }
    return menuByName.get(current?.name) || menuByPath.get(current?.path) || null;
}

const endpointMenus = new Map();
for (const route of menuInventory.routes) {
    const menu = rootMenuForRoute(route);
    if (!menu) continue;
    for (const reference of route.apiReferences || []) {
        const key = `${reference.protocol}:${reference.path}:${(reference.methods || []).join(',')}`;
        if (!endpointMenus.has(key)) endpointMenus.set(key, new Map());
        endpointMenus.get(key).set(menu.name, menu);
    }
}

function fallbackMenu(endpoint) {
    const prefix = normalizePath(endpoint.path).split('/')[1];
    const mapping = {
        ai: 'AI-Menu', apps: 'App-Menu', containers: 'Container-Menu', cronjobs: 'Cronjob-Menu',
        databases: 'Database-Menu', hosts: 'System-Menu', logs: 'Log-Menu', settings: 'Setting-Menu',
        toolbox: 'Toolbox-Menu', websites: 'Website-Menu', openresty: 'Website-Menu',
    };
    return menuByName.get(mapping[prefix]) || menuByName.get('Home-Menu') || null;
}

function operationName(endpointPath) {
    const value = normalizePath(endpointPath);
    const last = value.split('/').filter(Boolean).at(-1) || 'root';
    const known = new Set(['search', 'list', 'get', 'info', 'create', 'update', 'del', 'delete', 'operate', 'status', 'check', 'config', 'setting', 'run', 'sync', 'upload', 'download', 'logs', 'log', 'install', 'uninstall', 'start', 'stop', 'restart', 'tree', 'options']);
    return known.has(last.toLowerCase()) ? last.toLowerCase() : 'endpoint';
}

const endpointCases = apiInventory.routes.map((endpoint, index) => {
    const sourceModule = (endpoint.sources || []).map((source) => source.match(/\/api\/modules\/([^/]+)\.ts$/)?.[1]).filter(Boolean).map((name) => `${name}.ts`);
    const call = matchCall(endpoint);
    const key = `${endpoint.protocol}:${endpoint.path}:${(endpoint.methods || []).join(',')}`;
    const fallback = fallbackMenu(endpoint);
    const relatedMenus = [...(endpointMenus.get(key)?.values() || [])];
    if (fallback && !relatedMenus.some((item) => item.name === fallback.name)) relatedMenus.unshift(fallback);
    const menu = fallback || relatedMenus[0] || null;
    const status = endpoint.status || 'not-run';
    return {
        id: `frontend-contract-${String(index + 1).padStart(3, '0')}`,
        menu: menu ? { name: menu.name, path: menu.path, title: menu.title } : { name: 'Unmapped', path: null, title: null },
        menus: relatedMenus.map((item) => ({ name: item.name, path: item.path, title: item.title })),
        protocol: endpoint.protocol,
        path: endpoint.path,
        methods: endpoint.methods || [],
        operation: operationName(endpoint.path),
        sourceModules: sourceModule,
        sourceFiles: endpoint.sources || [],
        request: {
            function: call?.function || null,
            signature: call?.signature || null,
            callPreview: call?.callPreview || null,
            sourceFile: call?.sourceFile || null,
            line: call?.line || null,
            variants: call ? requestVariants(call) : [],
        },
        response: {
            type: call?.responseType || null,
            interfaceSources: sourceModule.flatMap(interfaceSources),
            inferred: Boolean(call?.responseType),
        },
        status,
        evidence: null,
    };
});

function familyKey(endpointOrPath) {
    const normalized = normalizePath(typeof endpointOrPath === 'string' ? endpointOrPath : endpointOrPath.path);
    const parts = normalized.split('/').filter(Boolean);
    if (parts.length < 2) return normalized;
    const operation = parts.at(-1).toLowerCase();
    const known = new Set(['search', 'list', 'get', 'info', 'create', 'update', 'del', 'delete', 'operate', 'status', 'check', 'config', 'setting', 'run', 'sync', 'upload', 'download', 'logs', 'log', 'install', 'uninstall', 'start', 'stop', 'restart', 'tree', 'options']);
    return known.has(operation) ? `/${parts.slice(0, -1).join('/')}` : normalized;
}

const familiesMap = new Map();
for (const item of endpointCases) {
    const key = `${item.protocol}:${familyKey(item.path)}`;
    if (!familiesMap.has(key)) familiesMap.set(key, []);
    familiesMap.get(key).push(item);
}
const operationFamilies = [...familiesMap.entries()].filter(([, variants]) => variants.length > 1).map(([key, variants]) => ({
    key,
    menu: variants[0].menu,
    basePath: familyKey(variants[0].path),
    variantCount: variants.length,
    variants: variants.map((item) => ({ id: item.id, path: item.path, methods: item.methods, operation: item.operation, requestVariants: item.request.variants, status: item.status })),
    status: variants.every((item) => item.status === 'not-run') ? 'not-run' : 'mixed',
    testPlan: [
        '使用真实 SQLite 和真实服务准备合法最小请求参数。',
        '逐个执行每个 path/method/type/operate/logType/source/scope 变体。',
        '记录 HTTP 状态码、响应 envelope、字段类型、任务 ID 和 SQLite 副作用。',
        '失败时保留脱敏请求摘要、响应摘要和可回滚资源标识。',
    ],
}));

const menuSummary = menuInventory.menus.map((menu) => {
    const cases = endpointCases.filter((item) => item.menus.some((itemMenu) => itemMenu.name === menu.name));
    const routes = menuInventory.routes.filter((route) => route.module === menu.module || rootMenuForRoute(route)?.name === menu.name);
    return {
        name: menu.name,
        path: menu.path,
        title: menu.title,
        routeCount: menu.name === 'Home-Menu' ? 1 : routes.length,
        endpointCount: cases.length,
        httpCount: cases.filter((item) => item.protocol === 'http').length,
        wsCount: cases.filter((item) => item.protocol === 'ws').length,
        status: cases.length === 0 ? 'not-run' : 'not-run',
    };
});

const statusCounts = (items) => items.reduce((counts, item) => {
    counts[item.status] = (counts[item.status] || 0) + 1;
    return counts;
}, {});

const report = {
    schema: 1,
    generatedAt: new Date().toISOString(),
    generatedFrom: '/www/apps/workmesh-server/web/src',
    sourceInventories: {
        menu: path.relative(root, menuFile).replaceAll(path.sep, '/'),
        api: path.relative(root, apiFile).replaceAll(path.sep, '/'),
    },
    policy: {
        sendsRequests: false,
        businessData: 'real SQLite only during execution; this generator creates no data',
        statusMeaning: 'not-run means no complete real request/response/side-effect evidence exists',
        passRule: 'only an independent execution report with actual request, status code, response fields and side-effect evidence may record pass',
    },
    summary: {
        menuCount: menuSummary.length,
        endpointCount: endpointCases.length,
        httpCount: endpointCases.filter((item) => item.protocol === 'http').length,
        wsCount: endpointCases.filter((item) => item.protocol === 'ws').length,
        operationFamilyCount: operationFamilies.length,
        status: statusCounts(endpointCases),
    },
    menus: menuSummary,
    operationFamilies,
    cases: endpointCases,
    limitations: [
        '接口清单仍来自静态源码扫描；此文件是可执行验收计划，不是业务通过报告。',
        '请求参数和响应类型只记录当前前端 API 模块/接口类型的来源，不能推导合法业务样例或所有动态字段。',
        '共享组件、运行时拼接 URL、服务端动态菜单和 WS 消息序列必须在真实登录会话中补充执行证据。',
    ],
};

fs.mkdirSync(path.dirname(output), { recursive: true });
fs.writeFileSync(output, `${JSON.stringify(report, null, 2)}\n`, 'utf8');

const menuRows = menuSummary.map((item) => `| ${item.title} | \`${item.path}\` | ${item.routeCount} | ${item.endpointCount} | ${item.httpCount} | ${item.wsCount} | ${item.status} |`).join('\n');
const familyRows = operationFamilies.slice(0, 120).map((family) => `| ${family.menu.title} | \`${family.basePath}\` | ${family.variantCount} | ${family.variants.map((variant) => `\`${variant.path}\``).join('<br>')} | ${family.status} |`).join('\n');
const sourceRows = endpointCases.slice(0, 389).map((item) => `| ${item.id} | ${item.menu.title} | ${item.protocol.toUpperCase()} | \`${item.path}\` | ${item.methods.join(', ') || 'WS'} | ${item.request.function || '—'} | ${item.response.type || '—'} | ${item.status} |`).join('\n');
const markdown = `# 前端 HTTP/WS 合约测试矩阵

> 生成时间：${report.generatedAt}  
> 生成器：\`test/contract/frontend-contract-matrix.mjs\`  
> 本文和 JSON 只生成验收计划，不发送业务请求；所有条目保持 \`not-run\`。

## 统计

| 项目 | 数量/状态 |
| --- | ---: |
| 主菜单 | ${report.summary.menuCount} |
| 接口案例 | ${report.summary.endpointCount} |
| HTTP | ${report.summary.httpCount} |
| WS | ${report.summary.wsCount} |
| 同路径/操作族 | ${report.summary.operationFamilyCount} |
| 当前状态 | \`not-run: ${report.summary.status['not-run'] || 0}\` |

## 13 个主菜单接口分组

同一个共享 API 可能被多个菜单页面调用，因此各菜单的接口数是“引用数”，不应直接相加作为 389 条唯一接口总数；唯一接口总数以 JSON 的 \`summary.endpointCount\` 为准。

| 主菜单 | 路径 | 路由数 | 接口数 | HTTP | WS | 状态 |
| --- | --- | ---: | ---: | ---: | ---: | --- |
${menuRows}

## 同一路由多操作/参数变体族

这些分组用于后续真实测试时逐项检查同一路径的 method、operation 以及 \`type/operate/logType/source/scope\` 变体；静态发现不代表服务端已通过。

| 菜单 | 基础路径 | 变体数 | 变体 | 状态 |
| --- | --- | ---: | --- | --- |
${familyRows || '| — | — | 0 | — | not-run |'}

## 案例字段来源

| ID | 菜单 | 协议 | 路径 | 方法 | 前端函数 | 响应类型 | 状态 |
| --- | --- | --- | --- | --- | --- | --- | --- |
${sourceRows}

完整字段、请求函数签名、调用摘要、参数维度、响应接口文件、菜单映射和执行证据占位符请查看同目录 JSON。

## 执行规则

1. 先使用有效登录会话和真实 SQLite，不得使用固定成功、空数组或 JSON 模拟业务数据。
2. 按菜单分组执行 HTTP/WS；同一接口的 method、路径参数、\`type\`、\`operate\`、\`logType\`、\`source\`、\`scope\` 必须分别记录。
3. 每条案例必须保存脱敏请求摘要、HTTP 状态码、响应 envelope、关键字段类型、任务 ID/WS 消息序列、SQLite 或外部副作用。
4. 只有真实执行器生成独立证据后，才能把案例状态从 \`not-run\` 改成 \`pass\`、\`fail\` 或 \`blocked\`；本生成器不会覆盖测试状态。
5. 上传、下载、SSE、终端 PTY、断线释放和重连等流式能力要记录连接建立、消息顺序、关闭码和资源释放。

## 当前静态限制

- 请求参数字段来自当前 API 模块函数签名和调用表达式；不能自动生成合法业务样例。
- 响应字段来源只记录当前泛型和接口文件；后端实际 envelope、错误分支和动态字段必须真实请求验证。
- WS 的 4 条调用必须追加消息协议、心跳、断线、权限和资源释放测试，不能只验证握手成功。
`;
fs.writeFileSync(markdownOutput, markdown, 'utf8');

console.log(JSON.stringify({
    output,
    markdownOutput,
    menuCount: report.summary.menuCount,
    endpointCount: report.summary.endpointCount,
    httpCount: report.summary.httpCount,
    wsCount: report.summary.wsCount,
    operationFamilyCount: report.summary.operationFamilyCount,
    status: report.summary.status,
}, null, 2));
