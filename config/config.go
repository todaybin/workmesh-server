// SPDX-License-Identifier: LicenseRef-WorkMesh-Pending
// Copyright (c) 2026 WorkMesh contributors

package config

import (
	"os"
	"strconv"
	"time"
)

// Config 描述 WorkMesh Server 的最小运行配置。
type Config struct {
	ListenAddr      string
	DataDir         string
	GatewayURL      string
	NodeID          string
	Role            string
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

// Load 从环境变量读取配置，未设置时使用低开销的本机默认值。
func Load() Config {
	return Config{
		ListenAddr:      env("WORKMESH_SERVER_ADDR", ":9999"),
		DataDir:         env("WORKMESH_DATA_DIR", "./data"),
		GatewayURL:      env("WORKMESH_GATEWAY_URL", ""),
		NodeID:          env("WORKMESH_NODE_ID", "local"),
		Role:            env("WORKMESH_NODE_ROLE", "secondary"),
		RequestTimeout:  durationEnv("WORKMESH_REQUEST_TIMEOUT", 30*time.Second),
		ShutdownTimeout: durationEnv("WORKMESH_SHUTDOWN_TIMEOUT", 10*time.Second),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}
