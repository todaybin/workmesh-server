/**
 * Gateway 注册与能力路由的前端契约适配。
 * 业务执行仍由服务端完成，页面只负责调用这些标准接口。
 */
import { getGatewayStatus, loginGateway } from './client'
import type {
  CapabilityRequest,
  CapabilityRoute,
  GatewayLoginRequest,
  GatewayLoginResponse,
  GatewayStatus,
} from './contracts'

export type { CapabilityRequest, CapabilityRoute, GatewayLoginRequest, GatewayLoginResponse, GatewayStatus }

export const gatewayApi = {
  status(): Promise<GatewayStatus> {
    return getGatewayStatus()
  },
  login(payload: GatewayLoginRequest): Promise<GatewayLoginResponse> {
    return loginGateway(payload)
  },
  async resolveRoute(request: CapabilityRequest): Promise<CapabilityRoute> {
    const response = await fetch('/api/v2/workmesh/gateway/capabilities/route', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(request),
    })
    if (!response.ok) throw new Error(response.statusText || '能力路由解析失败')
    const body = (await response.json()) as { code?: number; data?: { route?: CapabilityRoute }; message?: string }
    if (body.code !== undefined && body.code !== 200) throw new Error(body.message || '能力路由解析失败')
    const route = body.data?.route
    if (route !== 'local' && route !== 'gateway' && route !== 'auto') throw new Error('服务端返回了无效的能力路由')
    return route
  },
}
