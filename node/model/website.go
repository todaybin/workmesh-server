// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package model

import "time"

// Website 是节点本地网站的最小元数据；配置内容由 OpenRestyConfig 单独保存。
type Website struct {
	ID            uint      `json:"id"`
	PrimaryDomain string    `json:"primaryDomain"`
	Alias         string    `json:"alias"`
	Type          string    `json:"type"`
	Remark        string    `json:"remark,omitempty"`
	SiteDir       string    `json:"siteDir,omitempty"`
	Status        string    `json:"status"`
	Favorite      bool      `json:"favorite"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// WebsiteCreateRequest 创建网站时使用的兼容字段集合。
type WebsiteCreateRequest struct {
	PrimaryDomain string `json:"primaryDomain"`
	Alias         string `json:"alias"`
	Type          string `json:"type"`
	Remark        string `json:"remark"`
	SiteDir       string `json:"siteDir"`
}

// WebsiteUpdateRequest 更新网站元数据。
type WebsiteUpdateRequest struct {
	ID            uint   `json:"id"`
	PrimaryDomain string `json:"primaryDomain"`
	Alias         string `json:"alias"`
	Remark        string `json:"remark"`
	SiteDir       string `json:"siteDir"`
	Favorite      bool   `json:"favorite"`
}

// WebsiteDeleteRequest 删除网站。
type WebsiteDeleteRequest struct {
	ID uint `json:"id"`
}

// WAFRule 是网站自定义防护规则。
type WAFRule struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
	Key      string `json:"key,omitempty"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
	Action   string `json:"action"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
}

// WAFSite 是网站级 WAF 配置和规则集合。
type WAFSite struct {
	WebsiteID uint      `json:"websiteID"`
	Alias     string    `json:"alias"`
	Enabled   bool      `json:"enabled"`
	Mode      string    `json:"mode"`
	Rules     []WAFRule `json:"rules"`
}

// WAFGlobalConfig 是节点级 WAF 配置。
type WAFGlobalConfig struct {
	Enabled          bool   `json:"enabled"`
	StandardRules    bool   `json:"standardRules"`
	Mode             string `json:"mode"`
	ParanoiaLevel    int    `json:"paranoiaLevel"`
	InboundThreshold int    `json:"inboundThreshold"`
	RequestBodyLimit int64  `json:"requestBodyLimit"`
}

// WAFAccessLists 保存节点级黑白名单。
type WAFAccessLists struct {
	Whitelist []string `json:"whitelist"`
	Blacklist []string `json:"blacklist"`
}

// WAFStandardRule 是内置规则的展示信息。
type WAFStandardRule struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Locations   []string `json:"locations"`
}

// OpenRestyConfig 保存本机 OpenResty 控制面配置，不直接执行高权限命令。
type OpenRestyConfig struct {
	Version       string            `json:"version"`
	Enabled       bool              `json:"enabled"`
	DefaultHTTPS  bool              `json:"defaultHttps"`
	ConfigContent string            `json:"configContent,omitempty"`
	Modules       []OpenRestyModule `json:"modules"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// OpenRestyModule 描述一个可加载模块。
type OpenRestyModule struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	BuildMode string `json:"buildMode,omitempty"`
	LoadOrder int    `json:"loadOrder,omitempty"`
}
