#!/usr/bin/env node
// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

/**
 * 执行前端 HTTP/WS 合约矩阵中的安全边界和真实可提供的请求。
 *
 * 重要约束：
 * - 凭据只从 WORKMESH_TOKEN 或 WORKMESH_COOKIE 读取，不接受命令行凭据。
 * - 无请求 fixture 时不会为 POST/UPLOAD/DELETE 等业务接口生成假数据。
 * - 不修改 frontend-http-ws-contract-matrix.json，只写独立时间戳证据。
 */
import crypto from 'node:crypto';
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import tls from 'node:tls';

const args = process.argv.slice(2);
const option = (name, fallback) => {
    const index = args.indexOf(name);
    return index >= 0 ? args[index + 1] : fallback;
};

const root = path.resolve(option('--root', '.'));
const matrixPath = path.resolve(root, option('--matrix', 'docs/inventory/frontend-http-ws-contract-matrix.json'));
const timeoutMS = Number(option('--timeout-ms', process.env.WORKMESH_TIMEOUT_MS || 8000));
const dryRun = args.includes('--dry-run');
const baseURL = String(process.env.WORKMESH_BASE_URL || 'http://127.0.0.1:9999').replace(/\/$/, '');
const token = String(process.env.WORKMESH_TOKEN || '');
const cookie = String(process.env.WORKMESH_COOKIE || '');
const scope = String(process.env.WORKMESH_SCOPE || 'boundary').toLowerCase();
const fixturePath = process.env.WORKMESH_REQUEST_FIXTURES ? path.resolve(process.env.WORKMESH_REQUEST_FIXTURES) : '';
const timestamp = new Date().toISOString();
const fileTimestamp = timestamp.replace(/[-:.]/g, '').replace(/Z$/, 'Z');
const output = path.resolve(root, option('--out', `docs/inventory/frontend-contract-evidence-${fileTimestamp}.json`));

const matrix = JSON.parse(fs.readFileSync(matrixPath, 'utf8'));
const fixtures = fixturePath ? JSON.parse(fs.readFileSync(fixturePath, 'utf8')) : {};
const matrixHash = crypto.createHash('sha256').update(fs.readFileSync(matrixPath)).digest('hex');

function redactValue(value, depth = 0) {
    if (depth > 3) return '<nested>';
    if (Array.isArray(value)) return value.slice(0, 10).map((item) => redactValue(item, depth + 1));
    if (!value || typeof value !== 'object') return value;
    const result = {};
    for (const [key, item] of Object.entries(value).slice(0, 40)) {
        if (/token|password|secret|private.?key|authorization|cookie|api.?key/i.test(key)) result[key] = '<redacted>';
        else result[key] = redactValue(item, depth + 1);
    }
    return result;
}

function summarizeBody(text) {
    const compact = String(text || '').replace(/\s+/g, ' ').trim();
    if (!compact) return '';
    try {
        return JSON.stringify(redactValue(JSON.parse(compact))).slice(0, 1200);
    } catch {
        return compact.slice(0, 1200);
    }
}

function authHeaders() {
    const headers = {};
    if (token) headers.Authorization = 'Bearer <redacted>';
    if (cookie) headers.Cookie = '<redacted>';
    return headers;
}

function requestHeaders(extra = {}) {
    return {
        ...authHeaders(),
        ...extra,
    };
}

function fixtureFor(item) {
    const fixture = fixtures[item.id];
    return fixture && typeof fixture === 'object' ? fixture : null;
}

function isDynamicPath(requestPath) {
    return /:[A-Za-z0-9_-]+|\$\{|<[^>]+>/.test(requestPath);
}

function isSafeHTTP(item) {
    return item.protocol === 'http' && item.methods.length > 0 && item.methods.every((method) => ['GET', 'HEAD'].includes(method.toUpperCase()));
}

function hasCredentials() {
    return Boolean(token || cookie);
}

function boundaryCases() {
    return [
        { id: 'boundary-health', protocol: 'http', path: '/health', methods: ['GET'], expectedStatus: [200] },
        { id: 'boundary-ready', protocol: 'http', path: '/ready', methods: ['GET'], expectedStatus: [200] },
        { id: 'boundary-v2-health', protocol: 'http', path: '/api/v2/health', methods: ['GET'], expectedStatus: [200] },
        { id: 'boundary-auth-setting', protocol: 'http', path: '/api/v2/core/auth/setting', methods: ['GET'], expectedStatus: [200] },
        { id: 'boundary-auth-current', protocol: 'http', path: '/api/v2/core/auth/current', methods: ['GET'], expectedStatus: [401] },
        { id: 'boundary-dashboard-unauthorized', protocol: 'http', path: '/api/v2/dashboard/app/launcher', methods: ['GET'], expectedStatus: [401] },
        { id: 'boundary-ws-terminal-local', protocol: 'ws', path: '/api/v2/hosts/terminal/local', methods: [], expectedStatus: [401, 403] },
    ];
}

function caseDecision(item) {
    if (dryRun) return { action: 'not-run', reason: 'dry-run' };
    if (!hasCredentials()) return { action: 'run', reason: 'unauthenticated-boundary' };
    if (scope === 'boundary') {
        return item.id.startsWith('boundary-')
            ? { action: 'run', reason: 'authenticated-boundary' }
            : { action: 'not-run', reason: 'WORKMESH_SCOPE=boundary only runs boundary cases' };
    }
    if (item.protocol === 'ws') {
        if (scope === 'read' || scope === 'fixtures' || scope === 'all') return { action: 'run', reason: 'authenticated-ws-handshake' };
        return { action: 'not-run', reason: `unsupported scope: ${scope}` };
    }
    if (isSafeHTTP(item) && !isDynamicPath(item.path)) return { action: 'run', reason: 'authenticated-safe-read' };
    const fixture = fixtureFor(item);
    if (!fixture) return { action: 'blocked', reason: 'request fixture required; script will not fabricate business data' };
    if (scope !== 'fixtures' && scope !== 'all') return { action: 'not-run', reason: 'business cases require WORKMESH_SCOPE=fixtures or all' };
    if (scope === 'all' && process.env.WORKMESH_CONFIRM_DESTRUCTIVE !== 'YES') {
        return { action: 'blocked', reason: 'WORKMESH_CONFIRM_DESTRUCTIVE=YES required for all-scope mutations' };
    }
    return { action: 'run', reason: 'real request fixture supplied' };
}

function buildURL(requestPath) {
    return new URL(requestPath, `${baseURL}/`).toString();
}

function expectedStatusResult(actualStatus, expectedStatus) {
    if (!expectedStatus?.length) return 'observed';
    return expectedStatus.includes(actualStatus) ? 'pass' : 'fail';
}

async function executeHTTP(item, fixture = {}) {
    const startedAt = Date.now();
    const requestPath = fixture.path || item.path;
    if (isDynamicPath(requestPath)) return { status: 'blocked', reason: 'dynamic path needs an explicit fixture.path', requestPath };
    const method = String(fixture.method || item.methods[0] || 'GET').toUpperCase();
    const body = fixture.body;
    const headers = requestHeaders(fixture.headers || {});
    if (body !== undefined && !headers['Content-Type'] && !headers['content-type']) headers['Content-Type'] = 'application/json';
    const init = { method, headers, redirect: 'manual', signal: AbortSignal.timeout(timeoutMS) };
    if (body !== undefined) init.body = typeof body === 'string' ? body : JSON.stringify(body);
    try {
        const response = await fetch(buildURL(requestPath), init);
        const text = await response.text();
        return {
            status: expectedStatusResult(response.status, fixture.expectedStatus || item.expectedStatus),
            actualStatus: response.status,
            statusText: response.statusText,
            requestPath,
            method,
            contentType: response.headers.get('content-type') || '',
            responseSummary: summarizeBody(text),
            durationMs: Date.now() - startedAt,
            request: { headers: Object.keys(headers), bodyKeys: body && typeof body === 'object' ? Object.keys(body) : [] },
        };
    } catch (error) {
        return { status: 'blocked', reason: error instanceof Error ? error.message : String(error), requestPath, method, durationMs: Date.now() - startedAt };
    }
}

function websocketURL(requestPath) {
    const url = new URL(requestPath, `${baseURL}/`);
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    return url;
}

function closeFrame() {
    const payload = Buffer.from([0x03, 0xe8]);
    const mask = crypto.randomBytes(4);
    const frame = Buffer.alloc(8);
    frame[0] = 0x88;
    frame[1] = 0x80 | payload.length;
    mask.copy(frame, 2);
    for (let index = 0; index < payload.length; index += 1) frame[index + 6] = payload[index] ^ mask[index % 4];
    return frame;
}

function executeWebSocket(item, fixture = {}) {
    return new Promise((resolve) => {
        const startedAt = Date.now();
        const requestPath = fixture.path || item.path;
        if (isDynamicPath(requestPath)) {
            resolve({ status: 'blocked', reason: 'dynamic WS path needs an explicit fixture.path', requestPath });
            return;
        }
        const url = websocketURL(requestPath);
        const key = crypto.randomBytes(16).toString('base64');
        const port = Number(url.port || (url.protocol === 'wss:' ? 443 : 80));
        const transport = url.protocol === 'wss:' ? tls : net;
        let settled = false;
        let buffer = Buffer.alloc(0);
        const finish = (result) => {
            if (settled) return;
            settled = true;
            resolve({ ...result, requestPath, durationMs: Date.now() - startedAt, request: { headers: ['Host', 'Upgrade', 'Connection', 'Sec-WebSocket-*', ...Object.keys(authHeaders())] } });
        };
        const socket = transport.connect({ host: url.hostname, port, servername: url.hostname }, () => {
            const lines = [
                `GET ${url.pathname}${url.search} HTTP/1.1`,
                `Host: ${url.host}`,
                'Upgrade: websocket',
                'Connection: Upgrade',
                'Sec-WebSocket-Version: 13',
                `Sec-WebSocket-Key: ${key}`,
            ];
            if (token) lines.push(`Authorization: Bearer ${token}`);
            if (cookie) lines.push(`Cookie: ${cookie}`);
            socket.write(`${lines.join('\r\n')}\r\n\r\n`);
        });
        socket.setTimeout(timeoutMS, () => {
            socket.destroy();
            finish({ status: 'blocked', reason: 'WS handshake timeout' });
        });
        socket.on('data', (chunk) => {
            buffer = Buffer.concat([buffer, chunk]);
            const boundary = buffer.indexOf('\r\n\r\n');
            if (boundary < 0) return;
            const header = buffer.subarray(0, boundary).toString('utf8');
            const statusCode = Number(header.match(/^HTTP\/\d\.\d\s+(\d+)/)?.[1] || 0);
            if (statusCode === 101) {
                socket.write(closeFrame());
                setTimeout(() => socket.destroy(), 100);
                finish({
                    status: expectedStatusResult(statusCode, fixture.expectedStatus || item.expectedStatus),
                    actualStatus: statusCode,
                    handshake: '101 Switching Protocols',
                    closeSent: true,
                });
            } else {
                socket.destroy();
                finish({
                    status: expectedStatusResult(statusCode, fixture.expectedStatus || item.expectedStatus),
                    actualStatus: statusCode,
                    handshake: header.split('\r\n')[0] || 'no response',
                });
            }
        });
        socket.on('error', (error) => finish({ status: 'blocked', reason: error.message }));
        socket.on('close', () => {
            if (!settled) finish({ status: 'blocked', reason: 'WS connection closed before handshake' });
        });
    });
}

async function executeCase(item) {
    const decision = caseDecision(item);
    const base = { id: item.id, protocol: item.protocol, path: item.path, sourceStatus: item.status || 'not-run', decision };
    if (decision.action !== 'run') return { ...base, status: decision.action, evidence: null };
    const fixture = fixtureFor(item) || {};
    const result = item.protocol === 'ws' ? await executeWebSocket(item, fixture) : await executeHTTP(item, fixture);
    return { ...base, ...result, evidence: result.status === 'pass' || result.status === 'observed' ? { recordedAt: new Date().toISOString() } : null };
}

async function main() {
    const cases = hasCredentials() && scope !== 'boundary' ? [...boundaryCases(), ...matrix.cases] : boundaryCases();
    const deduped = [...new Map(cases.map((item) => [item.id, item])).values()];
    const results = [];
    for (const item of deduped) results.push(await executeCase(item));
    const counts = results.reduce((value, item) => {
        value[item.status] = (value[item.status] || 0) + 1;
        return value;
    }, {});
    const report = {
        schema: 1,
        generatedAt: timestamp,
        baseURL,
        scope,
        dryRun,
        authenticated: hasCredentials(),
        credentialSources: { token: Boolean(token), cookie: Boolean(cookie) },
        matrix: { file: path.relative(root, matrixPath).replaceAll(path.sep, '/'), sha256: matrixHash, sourceCaseCount: matrix.cases.length },
        counts,
        results,
        limitations: [
            '本证据文件独立于矩阵，不回写 frontend-http-ws-contract-matrix.json。',
            '无凭据只执行七条健康、认证边界和一个 WS 未授权握手；有凭据默认 WORKMESH_SCOPE=boundary，不会自动执行业务接口。',
            'WORKMESH_SCOPE=read 需要 Token/Cookie；没有凭据时不会因为设置 read 而绕过 boundary 安全限制。',
            'WORKMESH_SCOPE=fixtures 只执行有真实 fixture 的案例；需要请求体或动态路径的接口没有 fixture 时保持 blocked。',
            'WORKMESH_SCOPE=all 除真实 fixture 外还需要 WORKMESH_CONFIRM_DESTRUCTIVE=YES；脚本不构造模拟业务数据。',
            'WS 只验证 HTTP Upgrade 和关闭释放，不发送业务消息；终端/脚本等消息序列需要专用真实测试。',
        ],
    };
    fs.mkdirSync(path.dirname(output), { recursive: true });
    fs.writeFileSync(output, `${JSON.stringify(report, null, 2)}\n`, 'utf8');
    console.log(JSON.stringify({ output, authenticated: report.authenticated, scope, counts }, null, 2));
    if ((counts.fail || 0) > 0) process.exitCode = 1;
}

await main();
