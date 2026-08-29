// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

export type GatewayRegistration = 'registered' | 'pending' | 'revoked' | 'unregistered'
export type NodeRole = 'primary' | 'secondary'
export type CapabilityRoute = 'local' | 'gateway' | 'auto'

export interface GatewayStatus {
  registration: GatewayRegistration
  nodeId: string
  gatewayId?: string
  role: NodeRole
  connected: boolean
  authorizationExpiresAt?: string
  lastSeenAt?: string
  reason?: string
}

export interface GatewayLoginRequest {
  username: string
  password: string
  gatewayUrl?: string
}

export interface GatewayLoginResponse {
  authorizationUrl?: string
  registration: GatewayRegistration
  message?: string
}

export interface CapabilityRequest {
  capability: string
  route: CapabilityRoute
  projectId?: string
  environment?: string
}

export interface ApiEnvelope<T> {
  code: number
  data: T
  message?: string
  details?: Record<string, unknown>
}
