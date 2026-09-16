import assert from 'node:assert/strict';
import test from 'node:test';
import {
    getAppInstallId,
    isAppActive,
    isAppPresent,
    normalizeAppInstallId,
    resolveAppOperationTarget,
} from '../src/utils/app-install.ts';
import { buildSseRequestHeaders, SseParser } from '../src/utils/sse.ts';

test('normalizes numeric and string install IDs without accepting empty values', () => {
    assert.equal(normalizeAppInstallId(42), 42);
    assert.equal(normalizeAppInstallId(' 42 '), 42);
    assert.equal(normalizeAppInstallId('legacy-openresty-id'), 'legacy-openresty-id');
    assert.equal(normalizeAppInstallId(0), undefined);
    assert.equal(normalizeAppInstallId('0'), undefined);
    assert.equal(normalizeAppInstallId(''), undefined);
    assert.equal(normalizeAppInstallId({}), undefined);
    assert.equal(getAppInstallId(0, '', ' 42 '), 42);
});

test('accepts boolean, numeric and legacy string active probe values', () => {
    assert.equal(isAppActive(true), true);
    assert.equal(isAppActive(1), true);
    assert.equal(isAppActive('true'), true);
    assert.equal(isAppActive('Running'), true);
    assert.equal(isAppActive('false'), false);
    assert.equal(isAppActive(0), false);
});

test('keeps stopped installed applications present for lifecycle controls', () => {
    assert.equal(isAppPresent({ isExist: true, isActive: false, status: 'Stopped' }), true);
    assert.equal(isAppPresent({ status: 'Stopped' }, 12), true);
    assert.equal(isAppPresent({ isExist: false, status: 'NotInstalled' }), false);
});

test('uses the OpenResty API when detection succeeds without an install ID', () => {
    assert.deepEqual(resolveAppOperationTarget('openresty', { isExist: true }), { kind: 'openresty' });
    assert.deepEqual(resolveAppOperationTarget('openresty', { isActive: 'running' }), { kind: 'openresty' });
    assert.deepEqual(resolveAppOperationTarget('nginx', { containerName: 'workmesh-openresty-waf' }), {
        kind: 'openresty',
    });
    assert.equal(resolveAppOperationTarget('mysql', { isExist: true }), undefined);
});

test('prefers installed-app lifecycle operation when an install ID is available', () => {
    assert.deepEqual(resolveAppOperationTarget('openresty', { isExist: true, appInstallId: '7' }), {
        kind: 'installed',
        installId: 7,
    });
});

test('parses fragmented SSE logs and named lifecycle events', () => {
    const parser = new SseParser();
    const events = [
        ...parser.push('id: 1\r\ndata: first line\r\n'),
        ...parser.push('data: second line\r\n\r\n:event heartbeat\r\n'),
        ...parser.push('event: error\ndata: {"message":"container missing"}\n\n'),
        ...parser.push('event: close\ndata: {"exitCode":0}\n\n'),
    ];

    assert.deepEqual(events, [
        { event: 'message', data: 'first line\nsecond line', id: '1' },
        { event: 'error', data: '{"message":"container missing"}', id: '1' },
        { event: 'close', data: '{"exitCode":0}', id: '1' },
    ]);
});

test('flushes a final SSE line without a trailing newline', () => {
    const parser = new SseParser();
    assert.deepEqual(parser.push('data: tail'), []);
    assert.deepEqual(parser.finish(), [{ event: 'message', data: 'tail', id: undefined }]);
});

test('builds same-origin SSE headers with encoded node and resumable cursor', () => {
    assert.deepEqual(buildSseRequestHeaders('primary main', 17), {
        Accept: 'text/event-stream',
        'Cache-Control': 'no-cache',
        CurrentNode: 'primary%20main',
        'Last-Event-ID': '17',
    });
    assert.deepEqual(buildSseRequestHeaders('', 0), {
        Accept: 'text/event-stream',
        'Cache-Control': 'no-cache',
        CurrentNode: '',
    });
    assert.equal('Last-Event-ID' in buildSseRequestHeaders('primary', -1), false);
});
