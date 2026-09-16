export type AppInstallId = number | string;

export function isAppActive(value: unknown): boolean {
    if (value === true || value === 1) return true;
    if (typeof value !== 'string') return false;
    return ['1', 'true', 'running', 'active', 'enabled'].includes(value.trim().toLowerCase());
}

export function isAppPresent(data: Record<string, unknown>, installId?: AppInstallId): boolean {
    if (isAppActive(data.isExist) || isAppActive(data.isActive)) return true;
    if (installId !== undefined || String(data.containerName || '').trim() !== '') return true;
    return [
        'running',
        'stopped',
        'restarting',
        'paused',
        'error',
        'installing',
        'upgrading',
        'rebuilding',
    ].includes(String(data.status || '').trim().toLowerCase());
}

export function normalizeAppInstallId(value: unknown): AppInstallId | undefined {
    if (typeof value === 'number') {
        return Number.isFinite(value) && value > 0 ? value : undefined;
    }
    if (typeof value !== 'string') {
        return undefined;
    }
    const normalized = value.trim();
    if (!normalized || normalized === '0') {
        return undefined;
    }
    if (/^\d+$/.test(normalized)) {
        const numeric = Number(normalized);
        return Number.isSafeInteger(numeric) && numeric > 0 ? numeric : undefined;
    }
    return normalized;
}

export function getAppInstallId(...values: unknown[]): AppInstallId | undefined {
    for (const value of values) {
        const normalized = normalizeAppInstallId(value);
        if (normalized !== undefined) {
            return normalized;
        }
    }
    return undefined;
}

export type AppOperationTarget =
    | { kind: 'installed'; installId: AppInstallId }
    | { kind: 'openresty' };

export function resolveAppOperationTarget(
    appKey: unknown,
    data: Record<string, unknown>,
): AppOperationTarget | undefined {
    const installId = getAppInstallId(data.appInstallId, data.appInstallID, data.installId, data.id);
    if (installId !== undefined) {
        return { kind: 'installed', installId };
    }
    const key = String(appKey || '').trim().toLowerCase();
    const detected = isAppPresent(data, installId);
    if (detected && (key === 'openresty' || key === 'nginx')) {
        return { kind: 'openresty' };
    }
    return undefined;
}
