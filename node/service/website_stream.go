// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

// persistStreamConfig stores the typed stream form in the shared SQLite
// settings table so a restart can rebuild stream.conf without mock defaults.
func persistStreamConfig(executor storage.SQLExecutor, websiteID uint, value map[string]any, updatedAt time.Time) error {
	if executor == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = executor.Exec(`INSERT INTO website_settings(website_id,config_type,content,updated_at) VALUES(?,?,?,?) ON CONFLICT(website_id,config_type) DO UPDATE SET content=excluded.content,updated_at=excluded.updated_at`, websiteID, "stream", string(encoded), formatTime(updatedAt))
	return err
}

// UpdateStream 更新 TCP/UDP 站点的真实监听端口和上游服务器，并生成
// OpenResty stream.d 通配配置实际加载的 stream.conf。
func (s *WebsiteService) UpdateStream(websiteID uint, ports string, udp bool, algorithm string, rawServers []map[string]any) (map[string]any, error) {
	if websiteID == 0 {
		return nil, errors.New("网站 ID 无效")
	}
	portList, err := normalizeStreamPorts(ports)
	if err != nil {
		return nil, err
	}
	servers, err := normalizeStreamServers(rawServers)
	if err != nil {
		return nil, err
	}
	algorithm = normalizeStreamAlgorithm(algorithm)

	s.mu.Lock()
	defer s.mu.Unlock()
	site, err := s.getWebsiteLocked(websiteID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(site.Type, "stream") {
		return nil, errors.New("只有 TCP/UDP 网站支持流配置")
	}
	if len(servers) == 0 {
		return nil, errors.New("TCP/UDP 网站必须设置至少一个真实上游服务器")
	}

	path := s.SitePath(site, "stream.conf")
	old, readErr := os.ReadFile(path)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, readErr
	}
	content := renderStreamConfig(websiteID, portList, udp, algorithm, servers)
	if err := writeWebsiteAtomic(path, []byte(content), 0o640); err != nil {
		return nil, err
	}

	oldSite := site
	oldConfig := cloneWebsiteConfig(s.configs[websiteID], "stream")
	site.StreamPorts = strings.Join(portList, ",")
	site.UDP = udp
	site.Algorithm = algorithm
	site.Servers = append([]model.WebsiteStreamServer(nil), servers...)
	site.UpdatedAt = time.Now().UTC()
	for i := range s.websites {
		if s.websites[i].ID == websiteID {
			s.websites[i] = site
			break
		}
	}
	if s.configs[websiteID] == nil {
		s.configs[websiteID] = map[string]any{}
	}
	value := map[string]any{"streamPorts": site.StreamPorts, "udp": udp, "algorithm": algorithm, "servers": servers}
	s.configs[websiteID]["stream"] = value
	rollback := func() error {
		var rollbackErr error
		site = oldSite
		for i := range s.websites {
			if s.websites[i].ID == websiteID {
				s.websites[i] = oldSite
				break
			}
		}
		if oldConfig == nil {
			delete(s.configs[websiteID], "stream")
		} else {
			s.configs[websiteID]["stream"] = oldConfig
		}
		if len(old) == 0 {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		} else {
			if err := writeWebsiteAtomic(path, old, 0o640); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
		return rollbackErr
	}
	if err := s.persist("websites", s.websites); err != nil {
		return nil, errors.Join(err, rollback())
	}
	if repository, repositoryErr := s.sqliteRepository(); repositoryErr == nil {
		if err := repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
			return persistStreamConfig(tx, websiteID, value, site.UpdatedAt)
		}); err != nil {
			rollbackErr := rollback()
			if restoreErr := s.persist("websites", s.websites); restoreErr != nil {
				rollbackErr = errors.Join(rollbackErr, restoreErr)
			}
			if oldConfig == nil {
				if restoreErr := repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
					_, err := tx.Exec(`DELETE FROM website_settings WHERE website_id=? AND config_type=?`, websiteID, "stream")
					return err
				}); restoreErr != nil {
					rollbackErr = errors.Join(rollbackErr, restoreErr)
				}
			} else if previous, marshalErr := json.Marshal(oldConfig); marshalErr == nil {
				if restoreErr := repository.WithTx(context.Background(), func(tx storage.SQLExecutor) error {
					_, err := tx.Exec(`INSERT INTO website_settings(website_id,config_type,content,updated_at) VALUES(?,?,?,?) ON CONFLICT(website_id,config_type) DO UPDATE SET content=excluded.content,updated_at=excluded.updated_at`, websiteID, "stream", string(previous), formatTime(oldSite.UpdatedAt))
					return err
				}); restoreErr != nil {
					rollbackErr = errors.Join(rollbackErr, restoreErr)
				}
			} else {
				rollbackErr = errors.Join(rollbackErr, marshalErr)
			}
			return nil, errors.Join(err, rollbackErr)
		}
	} else if s.db != nil {
		rollbackErr := rollback()
		if restoreErr := s.persist("websites", s.websites); restoreErr != nil {
			rollbackErr = errors.Join(rollbackErr, restoreErr)
		}
		return nil, errors.Join(repositoryErr, rollbackErr)
	}
	return map[string]any{"websiteID": websiteID, "streamPorts": site.StreamPorts, "udp": udp, "algorithm": algorithm, "servers": servers}, nil
}

func normalizeStreamPorts(raw string) ([]string, error) {
	seen := map[string]bool{}
	ports := make([]string, 0, 4)
	for _, part := range strings.Split(strings.TrimSpace(raw), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		port, err := strconv.Atoi(part)
		if err != nil || port < 1 || port > 65535 {
			return nil, errors.New("TCP/UDP 端口无效")
		}
		part = strconv.Itoa(port)
		if !seen[part] {
			seen[part] = true
			ports = append(ports, part)
		}
	}
	if len(ports) == 0 {
		return nil, errors.New("TCP/UDP 网站必须设置端口")
	}
	return ports, nil
}

func normalizeStreamAlgorithm(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "least_conn", "least-conn":
		return "least_conn"
	case "hash", "ip_hash", "ip-hash":
		return "hash"
	default:
		return "round_robin"
	}
}

func normalizeStreamServers(raw []map[string]any) ([]model.WebsiteStreamServer, error) {
	servers := make([]model.WebsiteStreamServer, 0, len(raw))
	for _, item := range raw {
		server := strings.TrimSpace(mapString(item, "server", "address", "url"))
		if server == "" {
			return nil, errors.New("TCP/UDP 上游服务器地址不能为空")
		}
		server, err := validateNginxSettingValue(server, false)
		if err != nil {
			return nil, fmt.Errorf("TCP/UDP 上游服务器无效: %w", err)
		}
		entry := model.WebsiteStreamServer{Server: server, Weight: mapInt(item, "weight"), FailTimeout: mapInt(item, "failTimeout"), FailTimeoutUnit: mapString(item, "failTimeoutUnit"), MaxFails: mapInt(item, "maxFails"), MaxConns: mapInt(item, "maxConns"), Flag: mapString(item, "flag")}
		if entry.Weight < 0 || entry.FailTimeout < 0 || entry.MaxFails < 0 || entry.MaxConns < 0 {
			return nil, errors.New("TCP/UDP 上游参数不能为负数")
		}
		if entry.FailTimeoutUnit == "" {
			entry.FailTimeoutUnit = "s"
		}
		if entry.FailTimeoutUnit != "ms" && entry.FailTimeoutUnit != "s" && entry.FailTimeoutUnit != "m" && entry.FailTimeoutUnit != "h" {
			return nil, errors.New("TCP/UDP 失败超时单位无效")
		}
		servers = append(servers, entry)
	}
	if len(servers) == 0 {
		return nil, errors.New("TCP/UDP 网站必须设置至少一个真实上游服务器")
	}
	return servers, nil
}

func renderStreamConfig(websiteID uint, ports []string, udp bool, algorithm string, servers []model.WebsiteStreamServer) string {
	name := fmt.Sprintf("workmesh_stream_%d", websiteID)
	var b strings.Builder
	b.WriteString("# workmesh-managed stream\n")
	b.WriteString("upstream " + name + " {\n")
	if algorithm == "least_conn" {
		b.WriteString("    least_conn;\n")
	} else if algorithm == "hash" {
		b.WriteString("    hash $remote_addr consistent;\n")
	}
	for _, server := range servers {
		b.WriteString("    server " + server.Server)
		if server.Weight > 0 {
			b.WriteString(fmt.Sprintf(" weight=%d", server.Weight))
		}
		if server.MaxFails > 0 {
			b.WriteString(fmt.Sprintf(" max_fails=%d", server.MaxFails))
		}
		if server.FailTimeout > 0 {
			b.WriteString(fmt.Sprintf(" fail_timeout=%d%s", server.FailTimeout, server.FailTimeoutUnit))
		}
		if server.MaxConns > 0 {
			b.WriteString(fmt.Sprintf(" max_conns=%d", server.MaxConns))
		}
		b.WriteString(";\n")
	}
	b.WriteString("}\n")
	for _, port := range ports {
		b.WriteString("server {\n    listen " + port)
		if udp {
			b.WriteString(" udp")
		}
		b.WriteString(";\n    proxy_pass " + name + ";\n}\n")
	}
	return b.String()
}

func mapString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key]; ok && value != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func mapInt(item map[string]any, key string) int {
	value, ok := item[key]
	if !ok {
		return 0
	}
	if number, ok := value.(float64); ok {
		return int(number)
	}
	number, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
	return number
}

func cloneWebsiteConfig(config map[string]any, key string) map[string]any {
	value, ok := config[key].(map[string]any)
	if !ok {
		return nil
	}
	clone := make(map[string]any, len(value))
	for k, v := range value {
		clone[k] = v
	}
	return clone
}
