// SPDX-License-Identifier: LicenseRef-WorkMesh-Pending
// Copyright (c) 2026 WorkMesh contributors

package gateway

import "context"

// Registration 描述 Gateway 返回的节点使用凭证状态。
type Registration struct {
	NodeID       string   `json:"node_id"`
	BindingID    string   `json:"binding_id"`
	Registered   bool     `json:"registered"`
	AccessToken  string   `json:"-"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// Client 是 Gateway 对接边界；本机服务可在无 Gateway 时使用 NoopClient 启动。
type Client interface {
	Register(context.Context, string, []string) (Registration, error)
	Heartbeat(context.Context, Registration) error
}

// NoopClient 用于开发和本机离线模式，不伪造已注册授权状态。
type NoopClient struct{}

func (NoopClient) Register(context.Context, string, []string) (Registration, error) {
	return Registration{Registered: false}, nil
}

// Heartbeat 在离线骨架模式下不执行网络请求。
func (NoopClient) Heartbeat(context.Context, Registration) error { return nil }
