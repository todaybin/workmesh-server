#!/usr/bin/env node
/* SPDX-License-Identifier: GPL-3.0-only */
/* Read-only security contract audit for the v2 HTTP/WS/SSE boundary. */

import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '../..');
const fallbackFiles = fs.readdirSync(path.join(root, 'node/api'))
  .filter((name) => name.startsWith('legacy_routes_') && name.endsWith('.go'));
const fallback = [];
const routeRE = /mux\.HandleFunc\("(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+([^\"]+)"[^\n]*fallbackRouteHandler/g;
for (const name of fallbackFiles) {
  const source = fs.readFileSync(path.join(root, 'node/api', name), 'utf8');
  for (const match of source.matchAll(routeRE)) {
    fallback.push({ file: name, method: match[1], path: match[2] });
  }
}

const byDomain = new Map();
const byMethod = new Map();
for (const item of fallback) {
  const domain = item.path.split('/')[3] || '(root)';
  byDomain.set(domain, (byDomain.get(domain) || 0) + 1);
  byMethod.set(item.method, (byMethod.get(item.method) || 0) + 1);
}

function extractList(file, expression) {
  const source = fs.readFileSync(path.join(root, file), 'utf8');
  const block = source.match(expression)?.[1] || '';
  return [...block.matchAll(/"(\/api\/v2\/[^\"]+)"/g)].map((m) => m[1]).sort();
}

const controlStreams = extractList('control/api/security_policy.go', /func isSelfAuthenticatedStream\(path string\) bool \{([\s\S]*?)\n\}/);
const nodeStreams = extractList('cmd/workmesh-server/main.go', /func selfAuthenticatedStreamPath\(path string\) bool \{([\s\S]*?)\n\}/);
const streamMismatch = controlStreams.length !== nodeStreams.length || controlStreams.some((value, i) => value !== nodeStreams[i]);

const linkSource = fs.readFileSync(path.join(root, 'runtime/link/server_handlers.go'), 'utf8');
const statusBody = linkSource.match(/func \(s \*Server\) status\([\s\S]*?\n\}/)?.[0] || '';
const linkStatusSigned = /s\.authenticate\(r, body\)/.test(statusBody);
const relayTests = fs.readFileSync(path.join(root, 'node/api/node_relay_test.go'), 'utf8');
const forgedForwardedCovered = /TestNodeRelayRejectsMissingOrStaleSignature/.test(relayTests) && /X-WorkMesh-Forwarded/.test(relayTests);

console.log(JSON.stringify({
  fallbackCount: fallback.length,
  fallbackByDomain: Object.fromEntries([...byDomain.entries()].sort()),
  fallbackByMethod: Object.fromEntries([...byMethod.entries()].sort()),
  forwardedHeader: {
    outerHeaderIsNotTrusted: true,
    verificationRequiredByNodeRelay: true,
    regressionTest: 'node/api/node_relay_test.go',
    forgedHeaderCovered: forgedForwardedCovered,
  },
  streamWhitelist: { control: controlStreams, node: nodeStreams, mismatch: streamMismatch },
  linkStatus: { signedWhenSecretConfigured: linkStatusSigned, source: 'runtime/link/server_handlers.go' },
}, null, 2));

if (fallback.length !== 681) {
  console.error(`fallback route count changed: expected 681, got ${fallback.length}`);
  process.exitCode = 2;
}
if (streamMismatch) {
  console.error('WS/SSE self-authenticated route whitelist mismatch');
  process.exitCode = 3;
}
if (!forgedForwardedCovered) {
  console.error('missing forged X-WorkMesh-Forwarded regression coverage');
  process.exitCode = 5;
}
// link status is currently an audit finding; strict mode lets CI fail until it is signed.
if (process.argv.includes('--strict') && !linkStatusSigned) {
  console.error('link status does not authenticate signed requests when a secret is configured');
  process.exitCode = 4;
}
