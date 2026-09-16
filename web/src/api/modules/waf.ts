import http from '@/api';

export interface WafGlobalConfig {
    enabled: boolean;
    standardRules: boolean;
    mode: 'observe' | 'block';
    paranoiaLevel: number;
    inboundThreshold: number;
    requestBodyLimit: number;
    strictMode?: boolean;
    redis?: { enabled: boolean; host: string; port: number; password?: string; db?: number };
    frequency?: Record<string, WafFrequencyLimit>;
    frequencyLimit?: Record<string, WafFrequencyLimit>;
}

export interface WafFrequencyLimit {
    enabled: boolean;
    mode: 'global' | 'uri';
    period: number;
    count: number;
    blockTime: number;
}

export interface WafStatus {
    available: boolean;
    configured?: boolean;
    effective?: boolean;
    configValid?: boolean;
    configHash?: string;
    error?: string;
    enabled: boolean;
    standardRules: boolean;
    mode: 'observe' | 'block';
    runtime: string;
    crs: string;
    configDir: string;
}

export interface WafStandardRule {
    id: string;
    category: string;
    description: string;
    locations: string[];
}

export interface WafTestRequest {
    websiteID: number;
    method: string;
    uri: string;
    args?: string;
    headers?: Record<string, string>;
    cookies?: Record<string, string>;
    body?: string;
    remoteIP?: string;
}

export interface WafTestResult {
    blocked: boolean;
    status: number;
    matches: Array<{ id: string; name: string; source: string; action: string }>;
}

export interface WafSiteConfig {
    websiteID: number;
    alias: string;
    primaryDomain?: string;
    remark?: string;
    enabled: boolean;
    mode: 'observe' | 'block';
    detectionLevel?: number;
    frequencyEnabled?: boolean;
    rules: WafRule[];
}

export interface WafRule {
    id?: string;
    websiteID: number;
    name: string;
    location: string;
    key?: string;
    operator: string;
    value: string;
    action: 'allow' | 'log' | 'block';
    priority: number;
    enabled: boolean;
}

export interface WafAccessLists {
    whitelist: string[];
    blacklist: string[];
    enabled?: Record<string, boolean>;
    urlWhitelist?: string[];
    urlBlacklist?: string[];
    uaWhitelist?: string[];
    uaBlacklist?: string[];
    ipGroups?: Array<{ name: string; entries: string[]; enabled?: boolean }>;
    listMeta?: Record<string, { enabled: boolean; remark?: string }>;
}

export interface WafCustomRuleSet {
    rules: WafRule[];
}

export interface WafLogQuery {
    page?: number;
    pageSize?: number;
    websiteID?: number;
    host?: string;
    ip?: string;
    clientIP?: string;
    ipRegion?: string;
    ip_region?: string;
    uri?: string;
    rule?: string;
    ruleID?: string;
    action?: string;
    status?: number;
    keyword?: string;
    startTime?: string;
    endTime?: string;
}

export type WafAuditKind = 'attack' | 'log' | 'block' | 'intercept' | 'access';

export interface WafAuditItem {
    id?: string;
    host?: string;
    uri?: string;
    client_ip?: string;
    clientIP?: string;
    ipRegion?: string;
    action?: string;
    rule?: string;
    ruleID?: string;
    category?: string;
    status?: number | string;
    time?: string;
    timestamp?: string;
    [key: string]: unknown;
}

export interface WafAuditResult {
    total: number;
    items: WafAuditItem[];
    page?: number;
    pageSize?: number;
}

export const getWafStatus = () => http.get<WafStatus>('/websites/waf/status');
export interface WafOverview {
    today: { requests: number; intercepts: number; count4xx: number; count5xx: number };
    requestTrend: Array<{ day: string; count: number }>;
    interceptTrend: Array<{ day: string; count: number }>;
    sources: Array<{ source: string; count: number }>;
}
export const getWafOverview = () => http.get<WafOverview>('/websites/waf/overview');
export const updateWafGlobal = (req: WafGlobalConfig) => http.post('/websites/waf/global', req);
export const listWafStandardRules = () => http.get<WafStandardRule[]>('/websites/waf/standard-rules');
export const testWafRules = (req: WafTestRequest) => http.post<WafTestResult>('/websites/waf/test', req);
export const listWafSites = () => http.get<WafSiteConfig[]>('/websites/waf/sites');
export const updateWafSite = (
    req: Pick<WafSiteConfig, 'websiteID' | 'enabled' | 'mode'> & {
        frequencyEnabled?: boolean;
        detectionLevel?: number;
    },
) => http.post('/websites/waf/sites', req);
export const listWafRules = (websiteID: number) => http.get<WafRule[]>(`/websites/waf/sites/${websiteID}/rules`);
export const upsertWafRule = (req: WafRule) => http.post<WafRule>('/websites/waf/rules', req);
export const deleteWafRule = (req: { websiteID: number; id: string }) => http.post('/websites/waf/rules/delete', req);
export const getWafAccessLists = () => http.get<WafAccessLists>('/websites/waf/access-lists');
export const updateWafAccessLists = (req: WafAccessLists) => http.post('/websites/waf/access-lists', req);
export const getWafAudit = (kind: WafAuditKind = 'log', params?: WafLogQuery) =>
    http.get<WafAuditResult>(`/websites/waf/logs/${kind}`, params);
export const clearWafAudit = (kind: WafAuditKind = 'log', params?: WafLogQuery) =>
    http.post(`/websites/waf/logs/${kind}/clear`, params || {});

export const getWafDefaultRules = () => http.get<WafRule[]>('/websites/waf/global/default-rules');
export const updateWafDefaultRules = (rules: WafRule[]) => http.post('/websites/waf/global/default-rules', { rules });
export const getWafCustomRules = () => http.get<WafCustomRuleSet>('/websites/waf/global/custom-rules');
export const updateWafCustomRules = (rules: WafRule[]) => http.post('/websites/waf/global/custom-rules', { rules });
export const getWafGlobalConfig = () => http.get<WafGlobalConfig>('/websites/waf/global');
export const applyWafGlobalToSites = (websiteIDs?: number[]) =>
    http.post('/websites/waf/global/apply', websiteIDs?.length ? { websiteIDs } : {});
