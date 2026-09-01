// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadReadsServerJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	content := `{"listenAddress":"127.0.0.1","listenPort":18443,"publicURL":"https://panel.example.com","dataDir":"./state","requestTimeout":"12s","shutdownTimeout":"4s","limits":{"maxConcurrentTasks":3,"maxConcurrentConversions":1,"maxSSEStreams":16,"maxAIJobs":2,"maxLogBytes":1048576,"cacheTTL":"2m"},"backgroundTasks":{"enabled":false}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	clearConfigEnvironment(t)
	t.Setenv("WORKMESH_SERVER_CONFIG", path)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:18443" || cfg.PublicURL != "https://panel.example.com" || cfg.DataDir != "./state" {
		t.Fatalf("文件配置未生效: %#v", cfg)
	}
	if cfg.BackgroundTasks.Enabled || cfg.Limits.MaxConcurrentTasks != 3 || cfg.Limits.CacheTTL != 2*time.Minute {
		t.Fatalf("资源配置未生效: %#v", cfg)
	}
}

func TestLoadEnvironmentOverridesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	content := `{"listenAddress":"127.0.0.1","listenPort":9999,"dataDir":"./data","requestTimeout":"30s","shutdownTimeout":"10s","limits":{"maxConcurrentTasks":4,"maxConcurrentConversions":2,"maxSSEStreams":64,"maxAIJobs":4,"maxLogBytes":8388608,"cacheTTL":"5m"},"backgroundTasks":{"enabled":true}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	clearConfigEnvironment(t)
	t.Setenv("WORKMESH_SERVER_CONFIG", path)
	t.Setenv("WORKMESH_SERVER_ADDR", "0.0.0.0:18080")
	t.Setenv("WORKMESH_DATA_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv("WORKMESH_REQUEST_TIMEOUT", "9")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "0.0.0.0:18080" || cfg.RequestTimeout != 9*time.Second || cfg.DataDir != os.Getenv("WORKMESH_DATA_DIR") {
		t.Fatalf("环境变量覆盖未生效: %#v", cfg)
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	content := `{"listenAddress":"127.0.0.1","listenPort":70000,"dataDir":"./data","requestTimeout":"30s","shutdownTimeout":"10s","limits":{"maxConcurrentTasks":4,"maxConcurrentConversions":2,"maxSSEStreams":64,"maxAIJobs":4,"maxLogBytes":8388608,"cacheTTL":"5m"}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	clearConfigEnvironment(t)
	t.Setenv("WORKMESH_SERVER_CONFIG", path)
	if _, err := Load(); err == nil {
		t.Fatal("非法端口应阻止服务启动")
	}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"WORKMESH_SERVER_ADDR", "WORKMESH_SERVER_LISTEN_ADDRESS", "WORKMESH_SERVER_PORT", "WORKMESH_PUBLIC_URL", "WORKMESH_DATA_DIR", "WORKMESH_REQUEST_TIMEOUT", "WORKMESH_SHUTDOWN_TIMEOUT"} {
		t.Setenv(key, "")
	}
}
