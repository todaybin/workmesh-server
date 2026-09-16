package service

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

func (s *WebsiteService) loadRuleFile(name string, dst *[]model.WAFRule) error {
	if err := readJSON(filepath.Join(s.wafRoot(), name), dst); err != nil {
		if strings.Contains(err.Error(), "no such file") {
			*dst = []model.WAFRule{}
			return nil
		}
		return err
	}
	if *dst == nil {
		*dst = []model.WAFRule{}
	}
	return nil
}

func (s *WebsiteService) listGlobalRules(name string, dst *[]model.WAFRule) ([]model.WAFRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadRuleFile(name, dst); err != nil {
		return nil, err
	}
	return append([]model.WAFRule(nil), *dst...), nil
}

func (s *WebsiteService) updateGlobalRules(name string, rules []model.WAFRule, dst *[]model.WAFRule) ([]model.WAFRule, error) {
	normalized := make([]model.WAFRule, len(rules))
	for index, rule := range rules {
		clean, err := validateWAFRule(rule)
		if err != nil {
			return nil, err
		}
		normalized[index] = clean
	}
	rules = normalized
	s.mu.Lock()
	defer s.mu.Unlock()
	backups, err := s.snapshotWAFRuntime()
	if err != nil {
		return nil, err
	}
	oldRules := append([]model.WAFRule(nil), (*dst)...)
	if err := atomicJSON(filepath.Join(s.wafRoot(), name), rules); err != nil {
		return nil, err
	}
	*dst = append([]model.WAFRule(nil), rules...)
	if err := s.persistWAFRuntime(); err != nil {
		*dst = oldRules
		return nil, errors.Join(err, s.rollbackWAFRuntime(backups))
	}
	return append([]model.WAFRule(nil), rules...), nil
}

func validateWAFRule(rule model.WAFRule) (model.WAFRule, error) {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Location = strings.TrimSpace(rule.Location)
	rule.Key = strings.TrimSpace(rule.Key)
	rule.Operator = strings.TrimSpace(rule.Operator)
	rule.Action = strings.TrimSpace(rule.Action)
	if strings.TrimSpace(rule.Name) == "" || len(rule.Name) > 120 ||
		strings.TrimSpace(rule.Value) == "" || len(rule.Value) > 2048 {
		return model.WAFRule{}, fmt.Errorf("WAF 规则名称和值无效")
	}
	validLocation := map[string]bool{
		"ip": true, "ipGroup": true, "ip-group": true, "uri": true,
		"args": true, "header": true, "cookie": true, "method": true, "body": true,
	}
	validOperator := map[string]bool{
		"contains": true, "regex": true, "equals": true,
		"ip-cidr": true, "ip-group": true,
	}
	validAction := map[string]bool{"allow": true, "log": true, "block": true}
	if !validLocation[rule.Location] || !validOperator[rule.Operator] || !validAction[rule.Action] {
		return model.WAFRule{}, errors.New("WAF 规则字段无效")
	}
	if len(rule.Key) > 256 || strings.ContainsAny(rule.Key, "\x00\r\n") {
		return model.WAFRule{}, errors.New("WAF 规则字段名无效")
	}
	if rule.Priority <= 0 {
		rule.Priority = 100
	}
	if rule.Priority > 10000 {
		return model.WAFRule{}, errors.New("WAF 规则优先级无效")
	}
	if rule.Operator == "regex" {
		if len(rule.Value) > 256 {
			return model.WAFRule{}, errors.New("正则表达式长度不能超过 256")
		}
		if _, err := regexp.Compile(rule.Value); err != nil {
			return model.WAFRule{}, fmt.Errorf("正则表达式无效: %w", err)
		}
	}
	if rule.Operator == "ip-cidr" {
		if _, _, err := net.ParseCIDR(rule.Value); err != nil {
			return model.WAFRule{}, errors.New("IP 网段无效")
		}
	}
	if strings.TrimSpace(rule.ID) == "" {
		rule.ID = fmt.Sprintf("rule-%d", time.Now().UnixNano())
	}
	if len(rule.ID) > 128 || strings.ContainsAny(rule.ID, "\x00\r\n") {
		return model.WAFRule{}, errors.New("WAF 规则 ID 无效")
	}
	return rule, nil
}

func (s *WebsiteService) ListDefaultWAFRules() ([]model.WAFRule, error) {
	return s.listGlobalRules("default-rules.json", &s.wafDefaultRules)
}
func (s *WebsiteService) UpdateDefaultWAFRules(rules []model.WAFRule) ([]model.WAFRule, error) {
	return s.updateGlobalRules("default-rules.json", rules, &s.wafDefaultRules)
}
func (s *WebsiteService) ListCustomWAFRules() ([]model.WAFRule, error) {
	return s.listGlobalRules("custom-rules.json", &s.wafCustomRules)
}
func (s *WebsiteService) UpdateCustomWAFRules(rules []model.WAFRule) ([]model.WAFRule, error) {
	return s.updateGlobalRules("custom-rules.json", rules, &s.wafCustomRules)
}

// ApplyDefaultWAFToWebsites explicitly copies global defaults into each site;
// it never touches SQLite and never overwrites unrelated site settings.
func (s *WebsiteService) ApplyDefaultWAFToWebsites() error {
	rules, err := s.ListDefaultWAFRules()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	backups, err := s.snapshotWAFRuntime()
	if err != nil {
		return err
	}
	oldSites := make(map[uint]model.WAFSite, len(s.wafSites))
	for id, site := range s.wafSites {
		oldSites[id] = cloneWAFSite(site)
	}
	for _, website := range s.websites {
		if _, ok := s.wafSites[website.ID]; !ok {
			s.wafSites[website.ID] = defaultWAFSite(website.ID, website.Alias)
		}
	}
	for id, site := range s.wafSites {
		site.Rules = append([]model.WAFRule(nil), rules...)
		s.wafSites[id] = site
	}
	for id, site := range s.wafSites {
		website, websiteErr := s.getWebsiteLocked(id)
		if websiteErr != nil {
			s.wafSites = oldSites
			return websiteErr
		}
		if website.Alias == "" {
			website.Alias = site.Alias
		}
		base := s.SitePath(website, "waf")
		if err := atomicJSON(filepath.Join(base, "rules.json"), site.Rules); err != nil {
			s.wafSites = oldSites
			return errors.Join(err, s.rollbackWAFRuntime(backups))
		}
	}
	if err := s.persistWAFRuntime(); err != nil {
		s.wafSites = oldSites
		return errors.Join(err, s.rollbackWAFRuntime(backups))
	}
	return nil
}
