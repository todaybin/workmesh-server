import http from '@/api';

export interface WorkMeshGatewayStatus {
    configured: boolean;
    bindingRequired?: boolean;
    gatewayUrl?: string;
    nodeId?: string;
    displayName?: string;
    account?: string;
    status: 'not_configured' | 'running' | 'error' | string;
    lastError?: string;
}

export interface WorkMeshGatewayRegisterRequest {
    gatewayUrl: string;
    nodeId: string;
    registrationToken: string;
    bootstrapToken: string;
    displayName?: string;
    endpointUrl?: string;
}

export interface WorkMeshGatewayRegisterResponse {
    registered: boolean;
    gatewayUrl: string;
    nodeId: string;
    status: string;
}

let gatewayStatusCache: ReturnType<typeof http.get<WorkMeshGatewayStatus>> | null = null;

/** 获取网关绑定状态并在路由守卫与页面间复用结果，避免重复请求。 */
export const getWorkMeshGatewayStatus = () => {
    if (!gatewayStatusCache) {
        gatewayStatusCache = http.get<WorkMeshGatewayStatus>('/workmesh/gateway/status').catch((error) => {
            gatewayStatusCache = null;
            throw error;
        });
    }
    return gatewayStatusCache;
};

/** 网关绑定或解绑完成后清理缓存，使下一次读取获得最新状态。 */
export const clearWorkMeshGatewayStatusCache = () => {
    gatewayStatusCache = null;
};

export const unbindWorkMeshGateway = () => http.post<{ unbound: boolean; status: string }>('/workmesh/gateway/unbind');

export const registerWorkMeshGateway = (params: WorkMeshGatewayRegisterRequest) =>
    http.post<WorkMeshGatewayRegisterResponse>('/workmesh/gateway/register', params);
