// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"testing"

	"github.com/todaybin/workmesh-server/node/model"
)

func TestSSLServiceCreateAndList(t *testing.T) {
	service := NewSSLService()
	item, err := service.Create(context.Background(), model.WebsiteSSLCreateRequest{PrimaryDomain: "example.com", Provider: "letsencrypt", AutoRenew: true})
	if err != nil || item.ID == 0 || item.Status != "pending" {
		t.Fatalf("创建证书配置失败: %+v, %v", item, err)
	}
	list := service.List(context.Background(), "example")
	if len(list) != 1 || list[0].PrivateKey != "" {
		t.Fatalf("证书列表异常或泄漏私钥: %+v", list)
	}
}

func TestSSLServiceRejectsInvalidCertificate(t *testing.T) {
	service := NewSSLService()
	if _, err := service.Upload(context.Background(), model.WebsiteSSLUploadRequest{Certificate: "not-pem"}); err == nil {
		t.Fatal("无效证书应被拒绝")
	}
}
