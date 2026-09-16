// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { test } from 'node:test';

// 路由版本必须来自源文件，不把所有 v2 路由伪装为 v1 迁移。
test('保留真实来源、原路径、版本迁移及哈希', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'route-evidence-'));
  const outputRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'route-output-'));
  try {
    for (const [area, version] of [['core', 'v1/core'], ['agent', 'v2']]) {
      fs.mkdirSync(path.join(root, area, 'init/router'), { recursive: true });
      fs.mkdirSync(path.join(root, area, 'router'), { recursive: true });
      fs.writeFileSync(path.join(root, area, 'init/router/router.go'), `package router\nfunc Routers() { PrivateGroup := Router.Group("/api/${version}") }`);
      fs.writeFileSync(path.join(root, area, 'router/routes.go'), 'package router\nfunc InitRouter(Router *gin.RouterGroup) {\n group := Router.Group("logs")\n group.POST("/search", handler)\n}\n');
    }
    const output = path.join(outputRoot, 'routes.json');
    const result = spawnSync(process.execPath, ['test/contract/route-scan.mjs', 'generate', '--legacy', root, '--out', output], { encoding: 'utf8' });
    assert.equal(result.status, 0, result.stderr);
    const manifest = JSON.parse(fs.readFileSync(output, 'utf8'));
    assert.equal(manifest.generatedFrom, root);
    const core = manifest.routes.find((route) => route.path === '/api/v2/core/logs/search');
    assert.equal(core.originalPath, '/api/v1/core/logs/search');
    assert.equal(core.originalVersion, 'v1');
    assert.equal(core.versionMigration, 'v1-to-v2');
    assert.match(core.sourceSHA256, /^[a-f0-9]{64}$/);
    assert.equal(core.status, 'not-run');
    assert.equal(manifest.routes.find((route) => route.path === '/api/v2/logs/search').versionMigration, 'unchanged');
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
    fs.rmSync(outputRoot, { recursive: true, force: true });
  }
});
