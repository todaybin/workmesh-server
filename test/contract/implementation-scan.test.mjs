#!/usr/bin/env node
// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.resolve(here, '../../../..');
const scanner = path.join(here, 'implementation-scan.mjs');

test('实现扫描器区分真实处理器、迁移占位和固定能力响应', () => {
  const temporaryRoot = path.join(repositoryRoot, '.tmp');
  fs.mkdirSync(temporaryRoot, { recursive: true });
  const fixture = fs.mkdtempSync(path.join(temporaryRoot, 'implementation-scan-'));
  const project = path.join(fixture, 'project');
  const apiDirectory = path.join(project, 'node', 'api');
  fs.mkdirSync(apiDirectory, { recursive: true });

  try {
    fs.writeFileSync(path.join(apiDirectory, 'actual.go'), `package api

import "net/http"

func registerActual(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/actual", func(http.ResponseWriter, *http.Request) {})
	for _, p := range []string{"/api/v2/hosts/terminal/local"} {
		mux.HandleFunc("GET "+p, func(w http.ResponseWriter, r *http.Request) {
			_ = map[string]any{"supported": true, "stream": "websocket", "status": "ready"}
		})
	}
	for _, item := range []struct{ path, typ string }{
		{"/api/v2/websites/cors/update", "cors"},
	} {
		_ = item.typ
		mux.HandleFunc("POST "+item.path, func(http.ResponseWriter, *http.Request) {})
	}
}

func registerUnmigratedRoutes(mux *http.ServeMux) {
	paths := []string{"/api/v2/dead"}
	for _, p := range paths {
		mux.HandleFunc("GET "+p, func(http.ResponseWriter, *http.Request) {})
	}
}
`, 'utf8');
    fs.writeFileSync(path.join(apiDirectory, 'legacy_routes.go'), `package api

import "net/http"

func registerLegacy(mux *http.ServeMux) {
	for _, pattern := range []string{
		"GET /api/v2/actual",
		"GET /api/v2/dead",
		"GET /api/v2/hosts/terminal/local",
		"POST /api/v2/websites/cors/update",
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotImplemented) // MIGRATION_PENDING
		})
	}
}
`, 'utf8');
    const manifest = path.join(fixture, 'routes.json');
    fs.writeFileSync(manifest, `${JSON.stringify({ routes: [
      { method: 'GET', path: '/api/v2/actual', source: 'legacy/actual.go' },
      { method: 'GET', path: '/api/v2/dead', source: 'legacy/dead.go' },
      { method: 'GET', path: '/api/v2/hosts/terminal/local', source: 'legacy/terminal.go' },
      { method: 'POST', path: '/api/v2/websites/cors/update', source: 'legacy/website.go' },
    ] }, null, 2)}\n`, 'utf8');
    const output = path.join(fixture, 'result.json');

    execFileSync(process.execPath, [scanner, '--project', project, '--manifest', manifest, '--out', output], {
      cwd: repositoryRoot,
      encoding: 'utf8',
    });
    const report = JSON.parse(fs.readFileSync(output, 'utf8'));
    const status = Object.fromEntries(report.interfaces.map((item) => [`${item.method} ${item.path}`, item.new.status]));

    assert.equal(status['GET /api/v2/actual'], 'implemented');
    assert.equal(status['GET /api/v2/dead'], 'pending');
    assert.equal(status['GET /api/v2/hosts/terminal/local'], 'partial');
    assert.equal(status['POST /api/v2/websites/cors/update'], 'implemented');
  } finally {
    fs.rmSync(fixture, { recursive: true, force: true });
  }
});
