// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

/**
 * 浏览器传输 URL 工具。
 *
 * API 和流式连接必须通过当前页面的公网入口访问，节点地址只作为服务端
 * relay 的选择标识，不能在浏览器中直接拼接内部地址。
 */

export const configuredApiPath = () => {
    const value = String(import.meta.env.VITE_API_URL || '/api/v2').trim();
    if (!value) return '/api/v2';
    try {
        const parsed = new URL(value, window.location.origin);
        return parsed.pathname.replace(/\/+$/, '') || '/api/v2';
    } catch {
        return value.startsWith('/') ? value.replace(/\/+$/, '') : '/api/v2';
    }
};

const normalizePath = (path: string) => {
    const value = String(path || '').trim();
    if (!value) return configuredApiPath();
    try {
        const parsed = new URL(value, window.location.origin);
        const pathname = parsed.pathname.replace(/\/+$/, '');
        const base = configuredApiPath();
        if (pathname === base || pathname.startsWith(`${base}/`)) {
            return `${pathname}${parsed.search}`;
        }
        return `${base}${pathname.startsWith('/') ? pathname : `/${pathname}`}${parsed.search}`;
    } catch {
        const base = configuredApiPath();
        const normalized = value.replace(/^\/+/, '');
        return `${base}/${normalized}`;
    }
};

/** 构造当前公网入口下的 API URL；绝不使用配置中的远程 origin。 */
export const buildSameOriginApiUrl = (path: string, query?: URLSearchParams | Record<string, string | number | boolean | undefined>) => {
    const url = new URL(normalizePath(path), window.location.origin);
    if (query instanceof URLSearchParams) {
        query.forEach((value, key) => url.searchParams.set(key, value));
    } else if (query) {
        Object.entries(query).forEach(([key, value]) => {
            if (value !== undefined) url.searchParams.set(key, String(value));
        });
    }
    return url.toString();
};

/** 构造当前公网入口下的 WebSocket URL，并用节点 ID 选择服务端 relay 目标。 */
export const buildSameOriginWebSocketUrl = (
    path: string,
    targetNode?: string,
    query?: string | URLSearchParams | Record<string, string | number | boolean | undefined>,
) => {
    const url = new URL(normalizePath(path), window.location.origin);
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    if (typeof query === 'string' && query.trim()) {
        new URLSearchParams(query).forEach((value, key) => url.searchParams.set(key, value));
    } else if (query instanceof URLSearchParams) {
        query.forEach((value, key) => url.searchParams.set(key, value));
    } else if (query) {
        Object.entries(query).forEach(([key, value]) => {
            if (value !== undefined) url.searchParams.set(key, String(value));
        });
    }
    if (targetNode && !url.searchParams.has('operateNode')) {
        url.searchParams.set('operateNode', targetNode);
    }
    return url.toString();
};
