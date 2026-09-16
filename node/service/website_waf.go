// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// wafRoot is the sole WorkMesh WAF configuration root. It is deliberately
// independent from 1Panel and from the website SQLite database.
func (s *WebsiteService) wafRoot() string {
	if root := strings.TrimSpace(os.Getenv("WORKMESH_WAF_ROOT")); root != "" {
		return filepath.Clean(root)
	}
	if root := strings.TrimSpace(os.Getenv("WORKMESH_WAF_GLOBAL_DIR")); root != "" {
		return filepath.Clean(root)
	}
	// The production OpenResty container mounts this host directory at
	// /opt/workmesh/waf. Keep tests and custom deployments on the service-local
	// default, but align the built-in production data root with that mount.
	if filepath.Clean(s.root) == "/opt/workmesh-server/data" {
		return "/opt/workmesh-server/openresty-waf/waf/data"
	}
	return filepath.Join(s.root, "waf")
}

func (s *WebsiteService) wafGeneratedRoot() string {
	if root := strings.TrimSpace(os.Getenv("WORKMESH_WAF_GENERATED_DIR")); root != "" {
		return filepath.Clean(root)
	}
	root := s.wafRoot()
	if filepath.Base(root) == "data" {
		return filepath.Join(filepath.Dir(root), "generated")
	}
	return filepath.Join(root, "generated")
}

func atomicJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".waf-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0o640); err == nil {
		_, err = tmp.Write(append(data, '\n'))
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

type wafFileBackup struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

// wafRuntimeConfigured distinguishes a real deployment from the file-only
// development mode used by unit tests. A configured runtime must pass -t and
// reload before a WAF write is reported as successful.
func wafRuntimeConfigured() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("WORKMESH_WAF_ENFORCE"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	reloadSetting := strings.ToLower(strings.TrimSpace(os.Getenv("WORKMESH_WAF_RELOAD")))
	reloadConfigured := reloadSetting != "" &&
		reloadSetting != "0" &&
		reloadSetting != "false" &&
		reloadSetting != "no" &&
		reloadSetting != "off"
	return reloadConfigured ||
		strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_BIN")) != "" ||
		strings.TrimSpace(os.Getenv("WORKMESH_OPENRESTY_CONTAINER")) != ""
}

// wafRuntimeRequired 判断当前服务是否必须确认 OpenResty 真正加载了配置。
// 生产数据根目录默认开启该门槛，避免未配置环境变量时只写 JSON 而不 reload。
func (s *WebsiteService) wafRuntimeRequired() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("WORKMESH_WAF_ENFORCE"))) {
	case "0", "false", "no", "off":
		return false
	}
	if wafRuntimeConfigured() {
		return true
	}
	return s != nil && filepath.Clean(s.root) == "/opt/workmesh-server/data"
}

func (s *WebsiteService) snapshotWAFRuntime() ([]wafFileBackup, error) {
	paths := []string{
		filepath.Join(s.wafRoot(), "global.json"),
		filepath.Join(s.wafRoot(), "access-lists.json"),
		filepath.Join(s.wafRoot(), "default-rules.json"),
		filepath.Join(s.wafRoot(), "custom-rules.json"),
		filepath.Join(s.wafRoot(), "runtime.json"),
		filepath.Join(s.wafGeneratedRoot(), "modsecurity-mode.conf"),
		filepath.Join(s.wafGeneratedRoot(), "custom-rules.conf"),
		filepath.Join(s.wafGeneratedRoot(), "standard-rules.conf"),
		filepath.Join(s.wafGeneratedRoot(), "global-rules.conf"),
	}
	for _, site := range s.websites {
		website := model.Website{ID: site.ID, Alias: site.Alias, PrimaryDomain: s.domainForWebsite(site.ID), SiteDir: site.SiteDir}
		base := s.SitePath(website, "waf")
		paths = append(paths, filepath.Join(base, "config.json"), filepath.Join(base, "rules.json"))
	}
	backups := make([]wafFileBackup, 0, len(paths))
	seen := map[string]struct{}{}
	for _, path := range paths {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			backups = append(backups, wafFileBackup{path: path})
			continue
		}
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		backups = append(backups, wafFileBackup{path: path, data: data, mode: info.Mode().Perm(), exists: true})
	}
	return backups, nil
}

func restoreWAFRuntime(backups []wafFileBackup) error {
	var first error
	for _, backup := range backups {
		var err error
		if backup.exists {
			mode := backup.mode
			if mode == 0 {
				mode = 0o640
			}
			err = atomicWriteFile(backup.path, backup.data, mode)
		} else {
			err = os.Remove(backup.path)
			if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
		}
		if err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (s *WebsiteService) rollbackWAFRuntime(backups []wafFileBackup) error {
	restoreErr := restoreWAFRuntime(backups)
	if !s.wafRuntimeRequired() {
		return restoreErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return errors.Join(restoreErr, s.reloadWAFRuntime(ctx))
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".waf-runtime-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(mode); err == nil {
		_, err = tmp.Write(data)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *WebsiteService) wafRuntimeHash() (string, error) {
	backups, err := s.snapshotWAFRuntime()
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, backup := range backups {
		if !backup.exists || filepath.Base(backup.path) == "runtime.json" {
			continue
		}
		_, _ = hash.Write([]byte(backup.path))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(backup.data)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (s *WebsiteService) persistWAFRuntime() error {
	if err := s.writeWAFGeneratedConfig(); err != nil {
		return err
	}
	if !s.wafRuntimeRequired() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := s.reloadWAFRuntime(ctx); err != nil {
		return err
	}
	if err := s.verifyWAFRuntimeLoaded(ctx); err != nil {
		return err
	}
	hash, err := s.wafRuntimeHash()
	if err != nil {
		return err
	}
	manifest := map[string]any{
		"effective": true, "configHash": hash, "appliedAt": time.Now().UTC(),
		"configRoot": s.wafRoot(), "siteRoot": s.websiteRoot,
	}
	return atomicJSON(filepath.Join(s.wafRoot(), "runtime.json"), manifest)
}

func (s *WebsiteService) reloadWAFRuntime(ctx context.Context) error {
	// WAF mutations call this while holding s.mu. Use the lock-safe probe
	// helper and the current in-memory OpenResty config directly.
	status := s.probeOpenResty(ctx, s.openresty)
	if !status.Available {
		return fmt.Errorf("WAF OpenResty 不可用: %s", status.Error)
	}
	if !status.ConfigValid {
		return fmt.Errorf("WAF OpenResty 配置无效: %s", status.Error)
	}
	if strings.HasPrefix(status.Binary, "proc://") {
		return errors.New("WAF OpenResty 位于无法控制的独立命名空间，不能确认 reload 已生效")
	}
	if err := s.verifyWAFRuntimeLoaded(ctx); err != nil {
		return err
	}
	var output []byte
	var err error
	if strings.HasPrefix(status.Binary, "docker://") {
		name := strings.TrimPrefix(status.Binary, "docker://")
		output, err = runContainerOpenRestyCommand(ctx, dockerBinaryOrName(), name, "-s", "reload")
	} else {
		output, err = exec.CommandContext(ctx, status.Binary, "-s", "reload").CombinedOutput()
	}
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("WAF OpenResty reload 失败: %s", message)
	}
	confirmed := s.probeOpenResty(ctx, s.openresty)
	if !confirmed.Available || !confirmed.ConfigValid {
		return fmt.Errorf("WAF OpenResty reload 后确认失败: %s", confirmed.Error)
	}
	return nil
}

func (s *WebsiteService) verifyWAFRuntimeLoaded(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	for _, path := range []string{
		filepath.Join(s.wafGeneratedRoot(), "modsecurity-mode.conf"),
		filepath.Join(s.wafGeneratedRoot(), "custom-rules.conf"),
		filepath.Join(s.wafGeneratedRoot(), "standard-rules.conf"),
		filepath.Join(s.wafGeneratedRoot(), "global-rules.conf"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("WAF OpenResty 生成配置缺失: %s: %w", path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("WAF OpenResty 生成配置不是文件: %s", path)
		}
	}
	status := s.probeOpenResty(ctx, s.openresty)
	if !status.Available || !status.ConfigValid {
		return fmt.Errorf("WAF OpenResty 生效确认失败: %s", status.Error)
	}
	var output []byte
	var err error
	if strings.HasPrefix(status.Binary, "docker://") {
		name := strings.TrimPrefix(status.Binary, "docker://")
		output, err = runContainerOpenRestyCommand(ctx, dockerBinaryOrName(), name, "-T")
	} else if strings.HasPrefix(status.Binary, "proc://") {
		return errors.New("WAF OpenResty 位于无法控制的独立命名空间，不能确认配置已加载")
	} else {
		output, err = exec.CommandContext(ctx, status.Binary, "-T").CombinedOutput()
	}
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("WAF OpenResty 配置展开失败: %s", message)
	}
	config := string(output)
	required := []string{
		"modsecurity_rules_file /etc/workmesh-waf/modsecurity.conf",
		"workmesh_waf/access.lua",
	}
	for _, needle := range required {
		if !strings.Contains(config, needle) {
			return fmt.Errorf("WAF OpenResty 配置未加载 WorkMesh WAF 钩子: 缺少 %s", needle)
		}
	}
	return nil
}

// WAFRuntimeStatus is the status shown by the UI. effective is true only when
// the current file hash matches a manifest written after a successful reload.
func (s *WebsiteService) WAFRuntimeStatus() map[string]any {
	status := map[string]any{"configured": s.wafRuntimeRequired(), "effective": false}
	if !s.wafRuntimeRequired() {
		return status
	}
	probe := s.ProbeOpenResty(context.Background())
	status["available"], status["configValid"], status["binary"] = probe.Available, probe.ConfigValid, probe.Binary
	var wiringErr error
	if probe.Available && probe.ConfigValid {
		wiringErr = s.verifyWAFRuntimeLoaded(context.Background())
	}
	hash, err := s.wafRuntimeHash()
	if err != nil {
		status["error"] = err.Error()
		return status
	}
	var manifest struct {
		Effective  bool   `json:"effective"`
		ConfigHash string `json:"configHash"`
	}
	if readJSON(filepath.Join(s.wafRoot(), "runtime.json"), &manifest) == nil &&
		manifest.Effective && manifest.ConfigHash == hash && probe.Available && probe.ConfigValid && wiringErr == nil {
		status["effective"] = true
		status["configHash"] = hash
	} else {
		status["configHash"] = hash
		if wiringErr != nil {
			status["error"] = wiringErr.Error()
		} else {
			status["error"] = "WAF 配置尚未确认生效"
		}
	}
	return status
}

func (s *WebsiteService) persistWAFSites() error {
	backups, err := s.snapshotWAFRuntime()
	if err != nil {
		return err
	}
	for _, site := range s.wafSites {
		website, websiteErr := s.getWebsiteLocked(site.WebsiteID)
		if websiteErr != nil {
			return websiteErr
		}
		if website.Alias == "" {
			website.Alias = site.Alias
		}
		base := s.SitePath(website, "waf")
		if err := atomicJSON(filepath.Join(base, "config.json"), struct {
			WebsiteID        uint                               `json:"websiteID"`
			Alias            string                             `json:"alias"`
			Enabled          bool                               `json:"enabled"`
			Mode             string                             `json:"mode"`
			DetectionLevel   int                                `json:"detectionLevel"`
			FrequencyEnabled bool                               `json:"frequencyEnabled"`
			RateLimits       map[string]model.WAFFrequencyLimit `json:"rateLimits,omitempty"`
			Rules            []model.WAFRule                    `json:"rules"`
		}{site.WebsiteID, site.Alias, site.Enabled, site.Mode, site.DetectionLevel, site.FrequencyEnabled, site.RateLimits, site.Rules}); err != nil {
			return errors.Join(err, s.rollbackWAFRuntime(backups))
		}
		rules := site.Rules
		if rules == nil {
			rules = []model.WAFRule{}
		}
		if err := atomicJSON(filepath.Join(base, "rules.json"), rules); err != nil {
			return errors.Join(err, s.rollbackWAFRuntime(backups))
		}
	}
	if err := s.persistWAFRuntime(); err != nil {
		return errors.Join(err, s.rollbackWAFRuntime(backups))
	}
	return nil
}

func (s *WebsiteService) domainForWebsite(id uint) string {
	for _, site := range s.websites {
		if site.ID == id {
			return site.PrimaryDomain
		}
	}
	return strconv.FormatUint(uint64(id), 10)
}

// ListWAFSites 返回所有网站 WAF 配置；已存在网站但尚无配置时自动补齐默认项。
func (s *WebsiteService) ListWAFSites() []model.WAFSite {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, site := range s.websites {
		if _, ok := s.wafSites[site.ID]; !ok {
			s.wafSites[site.ID] = defaultWAFSite(site.ID, site.Alias)
		}
	}
	result := make([]model.WAFSite, 0, len(s.wafSites))
	for _, site := range s.wafSites {
		result = append(result, site)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].WebsiteID < result[j].WebsiteID })
	return result
}

// UpdateWAFSite 更新网站 WAF 开关和模式。
func (s *WebsiteService) UpdateWAFSite(id uint, enabled bool, mode string) (model.WAFSite, error) {
	return s.UpdateWAFSiteSettings(id, enabled, mode, nil)
}

func (s *WebsiteService) UpdateWAFSiteSettings(id uint, enabled bool, mode string, frequencyEnabled *bool) (model.WAFSite, error) {
	return s.UpdateWAFSiteSettingsWithDetection(id, enabled, mode, frequencyEnabled, nil)
}

// UpdateWAFSiteSettingsWithDetection updates the website WAF controls and
// writes detectionLevel to the same runtime file consumed by OpenResty.
func (s *WebsiteService) UpdateWAFSiteSettingsWithDetection(id uint, enabled bool, mode string, frequencyEnabled *bool, detectionLevel *int) (model.WAFSite, error) {
	if id == 0 || (mode != "observe" && mode != "block") {
		return model.WAFSite{}, errors.New("网站 WAF 参数无效")
	}
	if detectionLevel != nil && (*detectionLevel < 1 || *detectionLevel > 4) {
		return model.WAFSite{}, errors.New("网站 WAF 检测强度无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	backups, err := s.snapshotWAFRuntime()
	if err != nil {
		return model.WAFSite{}, err
	}
	website, err := s.getWebsiteLocked(id)
	if err != nil {
		return model.WAFSite{}, err
	}
	oldSite, hadOldSite := s.wafSites[id]
	oldSite = cloneWAFSite(oldSite)
	site := s.wafSites[id]
	if !hadOldSite {
		site = defaultWAFSite(id, website.Alias)
	}
	site.WebsiteID, site.Enabled, site.Mode = id, enabled, mode
	if site.DetectionLevel < 1 {
		site.DetectionLevel = 1
	}
	if site.Alias == "" {
		site.Alias = website.Alias
	}
	if site.Rules == nil {
		site.Rules = []model.WAFRule{}
	}
	if frequencyEnabled != nil {
		site.FrequencyEnabled = *frequencyEnabled
		if !site.FrequencyEnabled {
			site.RateLimits = map[string]model.WAFFrequencyLimit{}
		}
	}
	if detectionLevel != nil {
		site.DetectionLevel = *detectionLevel
	}
	s.wafSites[id] = site
	if err := s.persistWAFSites(); err != nil {
		if hadOldSite {
			s.wafSites[id] = oldSite
		} else {
			delete(s.wafSites, id)
		}
		return model.WAFSite{}, errors.Join(err, s.rollbackWAFRuntime(backups))
	}
	return site, nil
}

func cloneWAFSite(site model.WAFSite) model.WAFSite {
	site.Rules = append([]model.WAFRule(nil), site.Rules...)
	if site.RateLimits != nil {
		site.RateLimits = make(map[string]model.WAFFrequencyLimit, len(site.RateLimits))
		for name, limit := range site.RateLimits {
			site.RateLimits[name] = limit
		}
	}
	return site
}

func (s *WebsiteService) getWebsiteLocked(id uint) (model.Website, error) {
	for _, site := range s.websites {
		if site.ID == id {
			return site, nil
		}
	}
	return model.Website{}, os.ErrNotExist
}

// ListRules 返回按优先级排序的规则。
func (s *WebsiteService) ListRules(id uint) ([]model.WAFRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.getWebsiteLocked(id); err != nil {
		return nil, err
	}
	rules := append([]model.WAFRule(nil), s.wafSites[id].Rules...)
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
	return rules, nil
}

// UpsertRule 新增或更新结构化规则。
func (s *WebsiteService) UpsertRule(id uint, rule model.WAFRule) (model.WAFRule, error) {
	if id == 0 {
		return model.WAFRule{}, errors.New("WAF 规则参数无效")
	}
	var err error
	if rule, err = validateWAFRule(rule); err != nil {
		return model.WAFRule{}, err
	}
	if strings.TrimSpace(rule.ID) == "" {
		rule.ID = fmt.Sprintf("rule-%d", time.Now().UnixNano())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	backups, err := s.snapshotWAFRuntime()
	if err != nil {
		return model.WAFRule{}, err
	}
	if _, err := s.getWebsiteLocked(id); err != nil {
		return model.WAFRule{}, err
	}
	oldSite, hadOldSite := s.wafSites[id]
	oldSite = cloneWAFSite(oldSite)
	site := s.wafSites[id]
	if !hadOldSite {
		website, websiteErr := s.getWebsiteLocked(id)
		if websiteErr != nil {
			return model.WAFRule{}, websiteErr
		}
		site = defaultWAFSite(id, website.Alias)
	}
	site.WebsiteID, site.Rules = id, append([]model.WAFRule(nil), site.Rules...)
	found := false
	for i := range site.Rules {
		if site.Rules[i].ID == rule.ID {
			site.Rules[i], found = rule, true
			break
		}
	}
	if !found {
		site.Rules = append(site.Rules, rule)
	}
	s.wafSites[id] = site
	if err := s.persistWAFSites(); err != nil {
		if hadOldSite {
			s.wafSites[id] = oldSite
		} else {
			delete(s.wafSites, id)
		}
		return model.WAFRule{}, errors.Join(err, s.rollbackWAFRuntime(backups))
	}
	return rule, nil
}

// DeleteRule 删除指定网站规则。
func (s *WebsiteService) DeleteRule(id uint, ruleID string) error {
	if id == 0 || strings.TrimSpace(ruleID) == "" {
		return errors.New("WAF 规则参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	backups, err := s.snapshotWAFRuntime()
	if err != nil {
		return err
	}
	if _, err := s.getWebsiteLocked(id); err != nil {
		return err
	}
	oldSite, hadOldSite := s.wafSites[id]
	oldSite = cloneWAFSite(oldSite)
	site := s.wafSites[id]
	filtered := site.Rules[:0]
	for _, rule := range site.Rules {
		if rule.ID != ruleID {
			filtered = append(filtered, rule)
		}
	}
	site.Rules = filtered
	s.wafSites[id] = site
	if err := s.persistWAFSites(); err != nil {
		if hadOldSite {
			s.wafSites[id] = oldSite
		} else {
			delete(s.wafSites, id)
		}
		return errors.Join(err, s.rollbackWAFRuntime(backups))
	}
	return nil
}

// GetGlobal、UpdateGlobal 读取和更新节点级配置。
func (s *WebsiteService) GetGlobal() model.WAFGlobalConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.global
}
func (s *WebsiteService) UpdateGlobal(cfg model.WAFGlobalConfig) (model.WAFGlobalConfig, error) {
	if cfg.Mode != "observe" && cfg.Mode != "block" {
		return model.WAFGlobalConfig{}, errors.New("WAF 模式无效")
	}
	if cfg.ParanoiaLevel == 0 {
		cfg.ParanoiaLevel = 1
	}
	if cfg.RequestBodyLimit == 0 {
		cfg.RequestBodyLimit = 1 << 20
	}
	if cfg.ParanoiaLevel < 1 || cfg.ParanoiaLevel > 4 || cfg.InboundThreshold < 1 || cfg.InboundThreshold > 99 || cfg.RequestBodyLimit < 0 || cfg.RequestBodyLimit > 10485760 {
		return model.WAFGlobalConfig{}, errors.New("WAF 全局参数无效")
	}
	if cfg.Redis != nil {
		cfg.Redis.Host = strings.TrimSpace(cfg.Redis.Host)
		if cfg.Redis.Enabled && (cfg.Redis.Host == "" || len(cfg.Redis.Host) > 255 || strings.ContainsAny(cfg.Redis.Host, "\x00\r\n;{}") || cfg.Redis.Port < 1 || cfg.Redis.Port > 65535 || cfg.Redis.DB < 0 || cfg.Redis.DB > 255) {
			return model.WAFGlobalConfig{}, errors.New("WAF Redis 参数无效")
		}
	}
	sourceLimits := cfg.FrequencyLimit
	if len(sourceLimits) == 0 {
		sourceLimits = cfg.Frequency
	}
	cleanLimits := map[string]model.WAFFrequencyLimit{}
	for name, limit := range sourceLimits {
		name = strings.TrimSpace(name)
		if name == "" || len(name) > 64 || strings.ContainsAny(name, "\x00\r\n;{}") {
			return model.WAFGlobalConfig{}, errors.New("WAF 频率限制名称无效")
		}
		if limit.Mode == "" {
			limit.Mode = "global"
		}
		if limit.Mode != "global" && limit.Mode != "uri" {
			return model.WAFGlobalConfig{}, errors.New("WAF 频率限制模式无效")
		}
		if limit.Period < 1 || limit.Period > 86400 || limit.Count < 1 || limit.Count > 1000000 || limit.BlockTime < 1 || limit.BlockTime > 525600 {
			return model.WAFGlobalConfig{}, errors.New("WAF 频率限制参数无效")
		}
		cleanLimits[name] = limit
	}
	if len(cleanLimits) > 0 {
		cfg.Frequency = cleanLimits
		cfg.FrequencyLimit = cleanLimits
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	backups, err := s.snapshotWAFRuntime()
	if err != nil {
		return model.WAFGlobalConfig{}, err
	}
	oldGlobal := s.global
	s.global = cfg
	if err := atomicJSON(filepath.Join(s.wafRoot(), "global.json"), cfg); err != nil {
		s.global = oldGlobal
		return model.WAFGlobalConfig{}, err
	}
	if err := s.persistWAFRuntime(); err != nil {
		s.global = oldGlobal
		return model.WAFGlobalConfig{}, errors.Join(err, s.rollbackWAFRuntime(backups))
	}
	return cfg, nil
}
func (s *WebsiteService) GetLists() model.WAFAccessLists {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lists
}

// UpdateLists 校验 IP/CIDR、去重并持久化黑白名单。
func (s *WebsiteService) UpdateLists(lists model.WAFAccessLists) (model.WAFAccessLists, error) {
	clean := func(items []string) ([]string, error) {
		seen := map[string]bool{}
		out := []string{}
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if net.ParseIP(item) == nil {
				if _, _, err := net.ParseCIDR(item); err != nil {
					return nil, fmt.Errorf("IP 或网段无效: %s", item)
				}
			}
			if !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
		return out, nil
	}
	whitelist, err := clean(lists.Whitelist)
	if err != nil {
		return model.WAFAccessLists{}, err
	}
	blacklist, err := clean(lists.Blacklist)
	if err != nil {
		return model.WAFAccessLists{}, err
	}
	cleanText := func(items []string) []string {
		out := make([]string, 0, len(items))
		seen := map[string]bool{}
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item != "" && !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
		return out
	}
	meta := map[string]model.WAFListMeta{}
	metaFields := map[string][]string{
		"blacklist":    blacklist,
		"whitelist":    whitelist,
		"urlBlacklist": cleanText(lists.URLBlacklist),
		"urlWhitelist": cleanText(lists.URLWhitelist),
		"uaBlacklist":  cleanText(lists.UABlacklist),
		"uaWhitelist":  cleanText(lists.UAWhitelist),
	}
	for field, values := range metaFields {
		for _, value := range values {
			key := field + ":" + value
			if item, ok := lists.ListMeta[key]; ok {
				item.Remark = strings.TrimSpace(item.Remark)
				if len(item.Remark) > 500 {
					return model.WAFAccessLists{}, errors.New("黑白名单备注过长")
				}
				meta[key] = item
			}
		}
	}
	groups := make([]model.WAFIPGroup, 0, len(lists.IPGroups))
	for _, group := range lists.IPGroups {
		name := strings.TrimSpace(group.Name)
		if name == "" || len(name) > 120 {
			return model.WAFAccessLists{}, errors.New("IP 组名称无效")
		}
		entries, entryErr := clean(group.Entries)
		if entryErr != nil {
			return model.WAFAccessLists{}, entryErr
		}
		groups = append(groups, model.WAFIPGroup{Name: name, Entries: entries, Enabled: group.Enabled})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	backups, err := s.snapshotWAFRuntime()
	if err != nil {
		return model.WAFAccessLists{}, err
	}
	oldLists := s.lists
	s.lists = model.WAFAccessLists{
		Whitelist: whitelist, Blacklist: blacklist,
		Enabled:      normalizeWAFListEnabled(lists.Enabled),
		URLWhitelist: cleanText(lists.URLWhitelist), URLBlacklist: cleanText(lists.URLBlacklist),
		UAWhitelist: cleanText(lists.UAWhitelist), UABlacklist: cleanText(lists.UABlacklist), IPGroups: groups,
		ListMeta: meta,
	}
	if err := atomicJSON(filepath.Join(s.wafRoot(), "access-lists.json"), s.lists); err != nil {
		s.lists = oldLists
		return model.WAFAccessLists{}, err
	}
	if err := s.persistWAFRuntime(); err != nil {
		s.lists = oldLists
		return model.WAFAccessLists{}, errors.Join(err, s.rollbackWAFRuntime(backups))
	}
	return s.lists, nil
}

func normalizeWAFListEnabled(input map[string]bool) map[string]bool {
	out := map[string]bool{
		"blacklist": true, "whitelist": true,
		"urlBlacklist": true, "urlWhitelist": true,
		"uaBlacklist": true, "uaWhitelist": true, "ipGroups": true,
	}
	for key, value := range input {
		if _, ok := out[key]; ok {
			out[key] = value
		}
	}
	return out
}
