// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { test } from 'node:test';

const root = process.cwd();
const script = path.join(root, 'test/contract/frontend-contract-executor.mjs');
const matrix = path.join(root, 'docs/inventory/frontend-http-ws-contract-matrix.json');

function fileHash(file) {
    return crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
}

test('无凭据 dry-run 只保留七条 HTTP/WS 边界测试且不修改矩阵', () => {
    const output = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'workmesh-executor-')), 'evidence.json');
    const before = fileHash(matrix);
    const result = spawnSync(process.execPath, [script, '--dry-run', '--out', output], {
        cwd: root,
        encoding: 'utf8',
        env: { ...process.env, WORKMESH_TOKEN: '', WORKMESH_COOKIE: '' },
    });
    assert.equal(result.status, 0, result.stderr);
    const report = JSON.parse(fs.readFileSync(output, 'utf8'));
    assert.equal(report.authenticated, false);
    assert.equal(report.counts['not-run'], 7);
    assert.equal(report.results.length, 7);
    assert.equal(fileHash(matrix), before);
});

test('没有 fixture 时不会执行需要业务请求体的接口', () => {
    const output = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'workmesh-executor-fixture-')), 'evidence.json');
    const result = spawnSync(process.execPath, [script, '--out', output], {
        cwd: root,
        encoding: 'utf8',
        env: { ...process.env, WORKMESH_TOKEN: 'test-token-not-used-by-default-scope', WORKMESH_COOKIE: '', WORKMESH_SCOPE: 'boundary' },
    });
    assert.equal(result.status, 0, result.stderr);
    const report = JSON.parse(fs.readFileSync(output, 'utf8'));
    assert.equal(report.authenticated, true);
    assert.equal(report.scope, 'boundary');
    assert.equal(report.results.length, 7);
    assert.ok(report.results.every((item) => item.decision.reason.includes('boundary')));
});
