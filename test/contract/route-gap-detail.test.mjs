// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { test } from 'node:test';

test('网站路由扫描识别动态段、下载适配器、循环注册和统一分发器', () => {
  const result = spawnSync(process.execPath, ['test/contract/route-gap-detail.mjs'], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  const rows = result.stdout.split('\n').filter((line) => line.startsWith('| websites |'));
  const row = (route) => rows.find((line) => line.includes(`\`${route}\``));

  assert.match(row('/api/v2/websites/:param') ?? '', /\| not-run \| [^|]*node\/api\/website_crud\.go/);
  assert.match(row('/api/v2/websites/:param/config/:param') ?? '', /\| not-run \| [^|]*node\/api\/website_config_routes\.go/);
  assert.match(row('/api/v2/websites/leech') ?? '', /\| not-run \| [^|]*node\/api\/website_config_routes\.go/);
  assert.match(row('/api/v2/websites/auths') ?? '', /\| not-run \| [^|]*node\/api\/website_extensions_store\.go/);
  assert.match(row('/api/v2/websites/ssl/download') ?? '', /\| DOWNLOAD \| POST \|.*\| not-run \| [^|]*node\/api\/ssl\.go/);
  assert.match(row('/api/v2/websites/operate:param') ?? '', /\| not-run \| [^|]*node\/api\/website_advanced_routes_helpers\.go/);
});

test('领域前缀分发器覆盖容器和主机子路由', () => {
  const result = spawnSync(process.execPath, ['test/contract/route-gap-detail.mjs'], { encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);
  const rows = result.stdout.split('\n');
  for (const route of [
    '/api/v2/containers/daemonjson',
    '/api/v2/containers/image/pull',
    '/api/v2/containers/files/content',
    '/api/v2/hosts/diagnostics/summary',
    '/api/v2/hosts/firewall/rules',
  ]) {
    const row = rows.find((line) => line.includes(`\`${route}\``));
    assert.ok(row, `missing route ${route}`);
    assert.doesNotMatch(row, /\| 501 \|/, `prefix dispatcher incorrectly reported as 501: ${route}`);
    assert.match(row, /\| not-run \|/);
  }
});
