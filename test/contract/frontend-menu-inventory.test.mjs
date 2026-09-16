// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

import fs from 'node:fs';
import assert from 'node:assert/strict';
import { test } from 'node:test';

test('主窗口清单包含真实主菜单且默认未验收', () => {
  const report = JSON.parse(fs.readFileSync('docs/inventory/frontend-menu-inventory.json', 'utf8'));
  assert.equal(report.menuCount, 13);
  assert.equal(report.routeCount, 109);
  const paths = new Set(report.menus.map((item) => item.path));
  for (const path of ['/', '/apps', '/websites', '/ai', '/databases', '/containers', '/hosts', '/terminal', '/cronjobs', '/toolbox', '/advanced', '/logs', '/settings']) {
    assert.ok(paths.has(path), `缺少主菜单: ${path}`);
  }
  assert.ok(report.routes.every((item) => item.status === 'not-run'));
});

