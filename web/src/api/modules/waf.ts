import http from '@/api';

export interface WafGlobalConfig {
    enabled: boolean;
    standardRules: boolean;
    mode: 'observe' | 'block';
    paranoiaLevel: number;
    inboundThreshold: number;
    requestBodyLimit: number;
}

export interface WafStatus {
    available: boolean;
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
    enabled: boolean;
    mode: 'observe' | 'block';
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
}

export const getWafStatus = () => http.get<WafStatus>('/websites/waf/status');
export const updateWafGlobal = (req: WafGlobalConfig) => http.post('/websites/waf/global', req);
export const listWafStandardRules = () => http.get<WafStandardRule[]>('/websites/waf/standard-rules');
export const testWafRules = (req: WafTestRequest) => http.post<WafTestResult>('/websites/waf/test', req);
export const listWafSites = () => http.get<WafSiteConfig[]>('/websites/waf/sites');
export const updateWafSite = (req: Pick<WafSiteConfig, 'websiteID' | 'enabled' | 'mode'>) =>
    http.post('/websites/waf/sites', req);
export const listWafRules = (websiteID: number) => http.get<WafRule[]>(`/websites/waf/sites/${websiteID}/rules`);
export const upsertWafRule = (req: WafRule) => http.post<WafRule>('/websites/waf/rules', req);
export const deleteWafRule = (req: { websiteID: number; id: string }) => http.post('/websites/waf/rules/delete', req);
export const getWafAccessLists = () => http.get<WafAccessLists>('/websites/waf/access-lists');
export const updateWafAccessLists = (req: WafAccessLists) => http.post('/websites/waf/access-lists', req);
export const getWafAudit = (kind: 'attack' | 'log' | 'block' = 'log') =>
    http.post<{ total: number; items: Array<Record<string, any>> }>(
        `/xpack/waf/${kind === 'attack' ? 'attack/stat' : kind === 'block' ? 'block/search' : 'log/search'}`,
        {},
    );
