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

export const getWorkMeshGatewayStatus = () => http.get<WorkMeshGatewayStatus>('/workmesh/gateway/status');

export const unbindWorkMeshGateway = () => http.post<{ unbound: boolean; status: string }>('/workmesh/gateway/unbind');

export const registerWorkMeshGateway = (params: WorkMeshGatewayRegisterRequest) =>
    http.post<WorkMeshGatewayRegisterResponse>('/workmesh/gateway/register', params);
