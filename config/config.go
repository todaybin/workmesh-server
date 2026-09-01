// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package config 提供 WorkMesh Server 的文件配置和环境变量兼容读取。
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultConfigPath = "config/server.json"

// Limits 描述单进程运行时的资源上限。
type Limits struct {
	MaxConcurrentTasks       int
	MaxConcurrentConversions int
	MaxSSEStreams            int
	MaxAIJobs                int
	MaxLogBytes              int64
	CacheTTL                 time.Duration
}

// BackgroundTasks 描述是否启动节点级周期维护任务。
type BackgroundTasks struct {
	Enabled bool `json:"enabled"`
}

// Config 描述 WorkMesh Server 的运行配置。
type Config struct {
	ConfigPath      string
	ListenAddress   string
	ListenPort      int
	ListenAddr      string
	PublicURL       string
	DataDir         string
	Limits          Limits
	BackgroundTasks BackgroundTasks
	GatewayURL      string
	GatewayID       string
	GatewaySecret   string
	GatewayUsername string
	GatewayPassword string
	NodeID          string
	Role            string
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

type fileLimits struct {
	MaxConcurrentTasks       int    `json:"maxConcurrentTasks"`
	MaxConcurrentConversions int    `json:"maxConcurrentConversions"`
	MaxSSEStreams            int    `json:"maxSSEStreams"`
	MaxAIJobs                int    `json:"maxAIJobs"`
	MaxLogBytes              int64  `json:"maxLogBytes"`
	CacheTTL                 string `json:"cacheTTL"`
}
type fileConfig struct {
	ListenAddress   string          `json:"listenAddress"`
	ListenPort      int             `json:"listenPort"`
	PublicURL       string          `json:"publicURL"`
	DataDir         string          `json:"dataDir"`
	RequestTimeout  string          `json:"requestTimeout"`
	ShutdownTimeout string          `json:"shutdownTimeout"`
	Limits          fileLimits      `json:"limits"`
	BackgroundTasks BackgroundTasks `json:"backgroundTasks"`
}

// Load 读取 server.json，并使用环境变量覆盖部署相关配置。
func Load() (Config, error) {
	path := strings.TrimSpace(os.Getenv("WORKMESH_SERVER_CONFIG"))
	if path == "" {
		path = defaultConfigPath
	}
	cfg := defaultConfig(path)
	if err := loadFile(path, &cfg); err != nil {
		return Config{}, err
	}
	applyEnvironment(&cfg)
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	cfg.ListenAddr = net.JoinHostPort(cfg.ListenAddress, strconv.Itoa(cfg.ListenPort))
	return cfg, nil
}

func defaultConfig(path string) Config {
	return Config{ConfigPath: path, ListenAddress: "127.0.0.1", ListenPort: 9999, DataDir: "./data",
		Limits:          Limits{MaxConcurrentTasks: 4, MaxConcurrentConversions: 2, MaxSSEStreams: 64, MaxAIJobs: 4, MaxLogBytes: 8 << 20, CacheTTL: 5 * time.Minute},
		BackgroundTasks: BackgroundTasks{Enabled: true}, NodeID: "local", Role: "secondary", RequestTimeout: 30 * time.Second, ShutdownTimeout: 10 * time.Second}
}

func loadFile(path string, cfg *Config) error {
	file, err := os.Open(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		if strings.TrimSpace(os.Getenv("WORKMESH_SERVER_ADDR")) != "" {
			return nil
		}
		return fmt.Errorf("服务配置文件不存在: %s（可通过 WORKMESH_SERVER_CONFIG 指定路径）", path)
	}
	if err != nil {
		return fmt.Errorf("打开服务配置失败: %w", err)
	}
	defer file.Close()
	raw := fileConfig{ListenAddress: cfg.ListenAddress, ListenPort: cfg.ListenPort, PublicURL: cfg.PublicURL, DataDir: cfg.DataDir, RequestTimeout: cfg.RequestTimeout.String(), ShutdownTimeout: cfg.ShutdownTimeout.String(), Limits: fileLimits{MaxConcurrentTasks: cfg.Limits.MaxConcurrentTasks, MaxConcurrentConversions: cfg.Limits.MaxConcurrentConversions, MaxSSEStreams: cfg.Limits.MaxSSEStreams, MaxAIJobs: cfg.Limits.MaxAIJobs, MaxLogBytes: cfg.Limits.MaxLogBytes, CacheTTL: cfg.Limits.CacheTTL.String()}, BackgroundTasks: cfg.BackgroundTasks}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return fmt.Errorf("解析服务配置失败: %w", err)
	}
	requestTimeout, err := time.ParseDuration(strings.TrimSpace(raw.RequestTimeout))
	if err != nil {
		return fmt.Errorf("requestTimeout 无效: %w", err)
	}
	shutdownTimeout, err := time.ParseDuration(strings.TrimSpace(raw.ShutdownTimeout))
	if err != nil {
		return fmt.Errorf("shutdownTimeout 无效: %w", err)
	}
	cacheTTL, err := time.ParseDuration(strings.TrimSpace(raw.Limits.CacheTTL))
	if err != nil {
		return fmt.Errorf("limits.cacheTTL 无效: %w", err)
	}
	cfg.ListenAddress, cfg.ListenPort = strings.TrimSpace(raw.ListenAddress), raw.ListenPort
	cfg.PublicURL, cfg.DataDir = strings.TrimSpace(raw.PublicURL), strings.TrimSpace(raw.DataDir)
	cfg.RequestTimeout, cfg.ShutdownTimeout = requestTimeout, shutdownTimeout
	cfg.Limits = Limits{MaxConcurrentTasks: raw.Limits.MaxConcurrentTasks, MaxConcurrentConversions: raw.Limits.MaxConcurrentConversions, MaxSSEStreams: raw.Limits.MaxSSEStreams, MaxAIJobs: raw.Limits.MaxAIJobs, MaxLogBytes: raw.Limits.MaxLogBytes, CacheTTL: cacheTTL}
	cfg.BackgroundTasks = raw.BackgroundTasks
	return nil
}

func applyEnvironment(cfg *Config) {
	if value := strings.TrimSpace(os.Getenv("WORKMESH_SERVER_ADDR")); value != "" {
		if host, port, err := net.SplitHostPort(value); err == nil {
			cfg.ListenAddress = host
			cfg.ListenPort, _ = strconv.Atoi(port)
		} else {
			cfg.ListenPort = 0
		}
	}
	if value := strings.TrimSpace(os.Getenv("WORKMESH_SERVER_LISTEN_ADDRESS")); value != "" {
		cfg.ListenAddress = value
	}
	if value := strings.TrimSpace(os.Getenv("WORKMESH_SERVER_PORT")); value != "" {
		cfg.ListenPort, _ = strconv.Atoi(value)
	}
	if value := strings.TrimSpace(os.Getenv("WORKMESH_PUBLIC_URL")); value != "" {
		cfg.PublicURL = value
	}
	if value := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR")); value != "" {
		cfg.DataDir = value
	}
	cfg.GatewayURL = os.Getenv("WORKMESH_GATEWAY_URL")
	cfg.GatewayID = os.Getenv("WORKMESH_GATEWAY_ID")
	cfg.GatewaySecret = os.Getenv("WORKMESH_GATEWAY_SECRET")
	cfg.GatewayUsername = os.Getenv("WORKMESH_GATEWAY_USERNAME")
	cfg.GatewayPassword = os.Getenv("WORKMESH_GATEWAY_PASSWORD")
	if value := strings.TrimSpace(os.Getenv("WORKMESH_NODE_ID")); value != "" {
		cfg.NodeID = value
	}
	if value := strings.TrimSpace(os.Getenv("WORKMESH_NODE_ROLE")); value != "" {
		cfg.Role = value
	}
	if value := strings.TrimSpace(os.Getenv("WORKMESH_REQUEST_TIMEOUT")); value != "" {
		cfg.RequestTimeout = parseEnvironmentDuration(value)
	}
	if value := strings.TrimSpace(os.Getenv("WORKMESH_SHUTDOWN_TIMEOUT")); value != "" {
		cfg.ShutdownTimeout = parseEnvironmentDuration(value)
	}
}
func parseEnvironmentDuration(value string) time.Duration {
	if d, err := time.ParseDuration(value); err == nil {
		return d
	}
	if sec, err := strconv.Atoi(value); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	return 0
}

func (cfg Config) validate() error {
	if strings.ContainsAny(cfg.ListenAddress, "\x00\r\n") {
		return errors.New("listenAddress 无效")
	}
	if cfg.ListenPort < 1 || cfg.ListenPort > 65535 {
		return errors.New("listenPort 必须在 1 到 65535 之间")
	}
	if cfg.DataDir == "" {
		return errors.New("dataDir 不能为空")
	}
	if cfg.PublicURL != "" {
		parsed, err := url.Parse(cfg.PublicURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return errors.New("publicURL 必须是无凭据、查询和片段的 HTTP(S) URL")
		}
	}
	if cfg.RequestTimeout <= 0 || cfg.ShutdownTimeout <= 0 {
		return errors.New("请求和关闭超时必须大于 0")
	}
	if cfg.Limits.MaxConcurrentTasks < 1 || cfg.Limits.MaxConcurrentConversions < 1 || cfg.Limits.MaxSSEStreams < 1 || cfg.Limits.MaxAIJobs < 1 || cfg.Limits.MaxLogBytes < 1 || cfg.Limits.CacheTTL <= 0 {
		return errors.New("limits 中的资源上限必须大于 0")
	}
	return nil
}

// ApplyRuntimeEnvironment 为尚未完成依赖注入的旧服务同步统一配置。
func (cfg Config) ApplyRuntimeEnvironment() error {
	for key, value := range map[string]string{"WORKMESH_DATA_DIR": cfg.DataDir, "WORKMESH_SERVER_ADDR": cfg.ListenAddr, "WORKMESH_PUBLIC_URL": cfg.PublicURL} {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("同步运行配置 %s 失败: %w", key, err)
		}
	}
	return nil
}
