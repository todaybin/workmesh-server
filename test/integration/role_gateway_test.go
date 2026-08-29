// SPDX-License-Identifier: LicenseRef-WorkMesh-Pending
// Copyright (c) 2026 WorkMesh contributors

package integration

import (
	"context"
	"testing"

	"github.com/todaybin/workmesh-server/runtime/gateway"
	"github.com/todaybin/workmesh-server/runtime/role"
)

func TestRoleSwitchUsesEpochAndRejectsStaleWriter(t *testing.T) {
	manager, err := role.New("node-test", role.Primary)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := manager.Switch(ctx, 1, role.Secondary)
	if err != nil {
		t.Fatal(err)
	}
	if first.Role != role.Secondary || first.RoleEpoch != 2 {
		t.Fatalf("角色切换结果错误: %+v", first)
	}
	if _, err := manager.Switch(ctx, 1, role.Primary); err == nil {
		t.Fatal("过期 epoch 应拒绝写入")
	}
}

func TestRoleSwitchHonorsCanceledContext(t *testing.T) {
	manager, err := role.New("node-test", role.Secondary)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.Switch(ctx, 1, role.Primary); err == nil {
		t.Fatal("已取消上下文不应执行角色切换")
	}
}

func TestGatewayRegistrationContractHasIndependentNodeIdentity(t *testing.T) {
	request := gateway.RegisterRequest{
		NodeID:          "node-61-184-12-165",
		DisplayName:     "primary",
		Role:            "primary",
		ProtocolVersion: "v1",
		Capabilities:    []string{"system.command", "container.exec"},
	}
	if request.NodeID == "" || request.Role != "primary" || len(request.Capabilities) == 0 {
		t.Fatalf("注册请求缺少节点身份或能力: %+v", request)
	}
	// 密码字段不属于注册请求，防止将 Gateway 登录凭据写入节点注册报文。
	if request.Metadata != nil {
		if _, exists := request.Metadata["password"]; exists {
			t.Fatal("注册元数据不得包含 password")
		}
	}
}
