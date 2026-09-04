import http from '@/api';
import { ResPage } from '../interface';
import { Log } from '../interface/log';
import { TimeoutEnum } from '@/enums/http-enum';

export const getOperationLogs = (info: Log.SearchOpLog) => {
    return http.post<ResPage<Log.OperationLog>>(`/core/logs/operation`, info);
};

export const getLoginLogs = (info: Log.SearchLgLog, currentNode?: string) => {
    return http.post<ResPage<Log.LoginLogs>>(
        `/core/logs/login`,
        info,
        undefined,
        currentNode ? { CurrentNode: currentNode } : undefined,
    );
};

export const getSystemFiles = (node?: string) => {
    const params = node ? `?operateNode=${node}` : '';
    return http.get<Array<string>>(`/logs/system/files${params}`);
};

export const getSystemLogStatus = (node?: string) => {
    const query = node ? `?operateNode=${node}` : '';
    return http.get<Log.SystemLogStatus>(`/logs/system/status${query}`);
};

export const readSystemLogs = (params: Log.SystemLogSearch, node?: string) => {
    const query = node ? `?operateNode=${node}` : '';
    return http.post<Log.SystemLog>(`/logs/system/read${query}`, params);
};

export const listRunningServices = (node?: string) => {
    const query = node ? `?operateNode=${node}` : '';
    return http.get<string[]>(`/logs/system/services${query}`);
};

export const cleanLogs = (param: Log.CleanLog) => {
    return http.post(`/core/logs/clean`, param);
};

export const searchTasks = (req: Log.SearchTaskReq, node?: string) => {
    const params = node ? `?operateNode=${node}` : '';
    return http.post<ResPage<Log.Task>>(`/logs/tasks/search${params}`, req);
};

const taskLogRequests = new Map<string, ReturnType<typeof http.post<any>>>();

export const readTaskLogByLine = (req: Log.TaskLogReadReq, node?: string) => {
    const params = node ? `?operateNode=${node}` : '';
    const key = `${node || ''}:${JSON.stringify(req)}`;
    const existing = taskLogRequests.get(key);
    if (existing) {
        return existing;
    }
    const request = http.post<any>(`/logs/tasks/read${params}`, req, TimeoutEnum.T_40S);
    taskLogRequests.set(key, request);
    request.finally(() => {
        if (taskLogRequests.get(key) === request) {
            taskLogRequests.delete(key);
        }
    });
    return request;
};

let executingTaskRequest: ReturnType<typeof http.get<number>> | undefined;
let executingTaskRequestedAt = 0;

export const countExecutingTask = () => {
    const now = Date.now();
    if (executingTaskRequest && now - executingTaskRequestedAt < 300) {
        return executingTaskRequest;
    }
    executingTaskRequestedAt = now;
    executingTaskRequest = http.get<number>(`/logs/tasks/executing/count`);
    executingTaskRequest.finally(() => {
        if (Date.now() - executingTaskRequestedAt >= 300) {
            executingTaskRequest = undefined;
        }
    });
    return executingTaskRequest;
};
