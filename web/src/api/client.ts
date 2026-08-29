// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

import type {
  ApiEnvelope,
  GatewayLoginRequest,
  GatewayLoginResponse,
  GatewayStatus,
} from './contracts'

const jsonHeaders = { 'Content-Type': 'application/json' }

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: 'include',
    ...init,
    headers: { ...jsonHeaders, ...(init?.headers ?? {}) },
  })
  const body = (await response.json()) as ApiEnvelope<T> | T
  if (!response.ok) {
    const message = typeof body === 'object' && body && 'message' in body ? body.message : response.statusText
    throw new Error(String(message || '请求失败'))
  }
  if (typeof body === 'object' && body && 'code' in body && 'data' in body) {
    const envelope = body as ApiEnvelope<T>
    if (envelope.code !== 200) throw new Error(envelope.message || '服务端返回错误')
    return envelope.data
  }
  return body as T
}

export function getGatewayStatus(): Promise<GatewayStatus> {
  return request<GatewayStatus>('/api/v2/core/gateway/status')
}

export function loginGateway(payload: GatewayLoginRequest): Promise<GatewayLoginResponse> {
  return request<GatewayLoginResponse>('/api/v2/core/gateway/login', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}
