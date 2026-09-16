# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

GO ?= go
NPM ?= npm
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)
GOCACHE ?= /tmp/workmesh-go-cache
GOMODCACHE ?= $(shell $(GO) env GOMODCACHE)
GOPROXY ?= off
GOSUMDB ?= off

BUILD_DIR ?= .build
SERVER_BINARY ?= $(BUILD_DIR)/workmesh-server
RELEASE_DIR ?= release
RELEASE_BINARY ?= $(RELEASE_DIR)/workmesh-server-$(GOOS)-$(GOARCH)

.PHONY: build build-frontend build-server build-release deploy-release run test clean-frontend

build: build-server

# 前端构建结果写入 internal/webassets/dist，由 Go 的 embed.FS 在下一步嵌入。
build-frontend:
	$(NPM) --prefix web run build:pro

# Go 编译前必须先完成前端构建，避免生成不含真实管理端页面的二进制。
build-server: build-frontend
	mkdir -p $(BUILD_DIR)
	GOWORK=off GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) GOPROXY=$(GOPROXY) GOSUMDB=$(GOSUMDB) CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -trimpath -ldflags '-s -w' -o $(SERVER_BINARY) ./cmd/workmesh-server

# 生产发布入口：一个二进制包含 Go 服务和前端资源。
build-release: build-server
	mkdir -p $(RELEASE_DIR)
	install -m 0750 "$(SERVER_BINARY)" "$(RELEASE_BINARY)"
	sha256sum "$(RELEASE_BINARY)" >"$(RELEASE_BINARY).sha256"
	@printf 'release binary: %s\n' "$(RELEASE_BINARY)"
	@cat "$(RELEASE_BINARY).sha256"

deploy-release: build-release
	WORKMESH_SERVER_BINARY="$(CURDIR)/$(RELEASE_BINARY)" \
		bash deploy/install/activate-release.sh

run: build-frontend
	GOWORK=off $(GO) run ./cmd/workmesh-server

test:
	GOWORK=off $(GO) test ./...

clean-frontend:
	rm -rf internal/webassets/dist/*
	touch internal/webassets/dist/.keep
