import http from '@/api';

export interface MonitorQuery {
    websiteID?: number;
    startTime?: string;
    endTime?: string;
    page?: number;
    pageSize?: number;
    type?: string;
    ip?: string;
    uri?: string;
    status?: number;
    method?: string;
    spider?: boolean;
}
export interface MonitorConfig {
    websiteID?: number;
    enabled: boolean;
    storeDays: number;
    storeSize: number;
    excludeStatus: string;
    excludeExt: string;
    excludeURI: string;
    excludeIP: string;
    excludeUA: string;
    cdnType: string;
    realIPHeader: string;
}
export interface MonitorDailyStat {
    day: string;
    pv: number;
    uv: number;
    ip: number;
    flow: number;
    spider: number;
    req: number;
    count4xx: number;
    count5xx: number;
}
export interface MonitorVisitorLocation {
    name: string;
    value: number;
}
export interface MonitorQPS {
    qps: number;
    flow: number;
    updatedAt?: string;
    source?: string;
}
const root = '/websites/monitor';
export const monitorStat = (q: MonitorQuery) => http.post<MonitorDailyStat[]>(`${root}/stat`, q);
export const monitorVisitors = (q: MonitorQuery) => http.post<MonitorDailyStat[]>(`${root}/visitors`, q);
export const monitorVisitorsLoc = (q: MonitorQuery) => http.post<MonitorVisitorLocation[]>(`${root}/visitors/loc`, q);
export const monitorQPS = (q: MonitorQuery) => http.post<MonitorQPS>(`${root}/qps`, q);
export const monitorRank = (q: MonitorQuery) => http.post(`${root}/rank`, q);
export const monitorTrend = (q: MonitorQuery) => http.post(`${root}/trend`, q);
export const monitorLogs = (q: MonitorQuery) => http.post(`${root}/logs/search`, q);
export const monitorLogDetail = (q: MonitorQuery) => http.post(`${root}/logs/detail`, q);
export const clearMonitorLogs = (q: MonitorQuery) => http.post(`${root}/logs/clear`, q);
export const monitorLogsStat = (q: MonitorQuery) => http.post(`${root}/logs/stat`, q);
export const monitorWebsites = (q: MonitorQuery = {}) => http.post(`${root}/websites`, q);
export const getMonitorConfig = (websiteID = 0) =>
    websiteID ? http.post(`${root}/config/site`, { websiteID }) : http.get(`${root}/config/global`);
export const updateMonitorConfig = (config: MonitorConfig) =>
    config.websiteID ? http.post(`${root}/config/site/update`, config) : http.post(`${root}/config/global`, config);
