// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { test } from 'node:test';

const root = process.cwd();
const file = path.join(root, 'docs/inventory/frontend-http-ws-contract-matrix.json');

test('前端 HTTP/WS 合约矩阵保留完整范围和 not-run 状态', () => {
    const report = JSON.parse(fs.readFileSync(file, 'utf8'));
    assert.equal(report.summary.menuCount, 13);
    assert.equal(report.summary.endpointCount, 389);
    assert.equal(report.summary.httpCount, 385);
    assert.equal(report.summary.wsCount, 4);
    assert.equal(report.cases.length, 389);
    assert.equal(report.summary.status['not-run'], 389);
    assert.ok(report.operationFamilies.length > 0);
    assert.ok(report.cases.every((item) => item.status === 'not-run'));
    assert.ok(report.cases.every((item) => item.evidence === null));
    assert.ok(report.cases.every((item) => item.menu?.name));
});

test('矩阵案例 ID、协议和来源字段完整', () => {
    const report = JSON.parse(fs.readFileSync(file, 'utf8'));
    const ids = new Set(report.cases.map((item) => item.id));
    assert.equal(ids.size, report.cases.length);
    assert.ok(report.cases.every((item) => ['http', 'ws'].includes(item.protocol)));
    assert.ok(report.cases.every((item) => Array.isArray(item.sourceFiles)));
    assert.ok(report.cases.every((item) => item.request && item.response));
});
