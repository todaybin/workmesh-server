#!/usr/bin/env node
// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

/**
 * 执行不依赖管理员密码的真实 HTTP 冒烟测试，记录状态码和脱敏响应摘要。
 * 该脚本只写独立证据文件，不会把 759 条路由清单中的 not-run 改成 pass。
 */
import fs from 'node:fs';
import path from 'node:path';

const args = process.argv.slice(2);
const option = (name, fallback = undefined) => {
  const index = args.indexOf(name);
  return index >= 0 ? args[index + 1] : fallback;
};

const baseURL = String(option('--base', 'http://127.0.0.1:9999')).replace(/\/$/, '');
const securityEntrance = String(option('--security-entrance', '')).replace(/^\/+|\/+$/g, '');
const output = path.resolve(option('--out', 'docs/inventory/http-smoke-20260905.json'));
const timeoutMS = Number(option('--timeout-ms', '5000'));

function requestCase(name, method, requestPath, expectedStatus, body = undefined, headers = {}) {
  return { name, method, path: requestPath, expectedStatus, body, headers };
}

function cases() {
  const result = [
    requestCase('健康检查', 'GET', '/health', 200),
    requestCase('就绪检查', 'GET', '/ready', 200),
    requestCase('v2 健康检查', 'GET', '/api/v2/health', 200),
    requestCase('认证设置公开接口', 'GET', '/api/v2/core/auth/setting', 200),
    requestCase('未登录当前用户', 'GET', '/api/v2/core/auth/current', 401),
    requestCase('未登录概览接口', 'GET', '/api/v2/dashboard/app/launcher', 401),
    requestCase('错误密码登录', 'POST', '/api/v2/core/auth/login', 401, { name: '__contract_probe__', password: 'invalid' }, { 'Content-Type': 'application/json' }),
    requestCase('默认入口隐藏', 'GET', '/', 404),
  ];
  if (securityEntrance) {
    result.push(requestCase('安全入口页面', 'GET', `/${securityEntrance}`, 200));
    const cookie = Buffer.from(securityEntrance, 'utf8').toString('base64');
    result.push(requestCase('安全入口后的登录页', 'GET', '/login', 200, undefined, { Cookie: `SecurityEntrance=${cookie}` }));
  }
  return result;
}

function summarizeBody(text) {
  const compact = String(text ?? '').replace(/\s+/g, ' ').trim();
  if (!compact) return '';
  try {
    const value = JSON.parse(compact);
    if (value && typeof value === 'object') {
      const safe = { ...value };
      for (const key of ['token', 'password', 'secret', 'apiKey', 'privateKey', 'authorization']) {
        if (key in safe) safe[key] = '<redacted>';
      }
      return JSON.stringify(safe).slice(0, 500);
    }
  } catch {
    // HTML 和纯文本只记录截断摘要。
  }
  return compact.slice(0, 500);
}

async function execute(item) {
  const startedAt = new Date().toISOString();
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMS);
  try {
    const response = await fetch(`${baseURL}${item.path}`, {
      method: item.method,
      headers: item.headers,
      body: item.body === undefined ? undefined : JSON.stringify(item.body),
      redirect: 'manual',
      signal: controller.signal,
    });
    const text = await response.text();
    return {
      name: item.name,
      method: item.method,
      path: item.path,
      expectedStatus: item.expectedStatus,
      actualStatus: response.status,
      status: response.status === item.expectedStatus ? 'pass' : 'fail',
      responseSummary: summarizeBody(text),
      contentType: response.headers.get('content-type') || '',
      startedAt,
    };
  } catch (error) {
    return {
      name: item.name,
      method: item.method,
      path: item.path,
      expectedStatus: item.expectedStatus,
      actualStatus: null,
      status: 'blocked',
      responseSummary: error instanceof Error ? error.message : String(error),
      contentType: '',
      startedAt,
    };
  } finally {
    clearTimeout(timer);
  }
}

const results = [];
for (const item of cases()) results.push(await execute(item));
const report = {
  schema: 1,
  generatedAt: new Date().toISOString(),
  baseURL,
  securityEntranceConfigured: Boolean(securityEntrance),
  status: results.every((item) => item.status === 'pass') ? 'pass' : 'fail',
  counts: results.reduce((counts, item) => ({ ...counts, [item.status]: (counts[item.status] || 0) + 1 }), {}),
  results,
  limitations: [
    '仅覆盖无需有效管理员密码的公开接口、安全边界和错误密码分支。',
    '不修改 route-inventory 或 frontend-api-inventory 的 not-run 状态。',
    '登录后业务、WS 消息、文件流、Docker、ACME 和跨节点任务必须使用独立真实凭据继续验收。',
  ],
};
fs.mkdirSync(path.dirname(output), { recursive: true });
fs.writeFileSync(output, `${JSON.stringify(report, null, 2)}\n`, 'utf8');
console.log(`HTTP smoke: ${report.status} (${JSON.stringify(report.counts)}) -> ${output}`);
if (report.status !== 'pass') process.exitCode = 1;
