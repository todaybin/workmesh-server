// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package model

import "time"

// Website 是节点本地网站的最小元数据；配置内容由 OpenRestyConfig 单独保存。
type Website struct {
	ID            uint   `json:"id"`
	PrimaryDomain string `json:"primaryDomain"`
	Alias         string `json:"alias"`
	Type          string `json:"type"`
	Remark        string `json:"remark,omitempty"`
	SiteDir       string `json:"siteDir,omitempty"`
	// SitePath 和 Root 保留旧 Web 客户端的目录字段，始终与 SiteDir 指向同一站点管理目录。
	SitePath        string          `json:"sitePath,omitempty"`
	Root            string          `json:"root,omitempty"`
	Status          string          `json:"status"`
	HttpConfig      string          `json:"httpConfig,omitempty"`
	Proxy           string          `json:"proxy,omitempty"`
	ProxyType       string          `json:"proxyType,omitempty"`
	ErrorLog        bool            `json:"errorLog"`
	AccessLog       bool            `json:"accessLog"`
	DefaultServer   bool            `json:"defaultServer"`
	IPV6            bool            `json:"IPV6"`
	Rewrite         string          `json:"rewrite,omitempty"`
	WebsiteSSLID    uint            `json:"websiteSSLId,omitempty"`
	RuntimeID       string          `json:"runtimeID,omitempty"`
	AppInstallID    uint            `json:"appInstallId,omitempty"`
	FtpID           uint            `json:"ftpId,omitempty"`
	ParentWebsiteID uint            `json:"parentWebsiteID,omitempty"`
	User            string          `json:"user,omitempty"`
	Group           string          `json:"group,omitempty"`
	DbType          string          `json:"dbType,omitempty"`
	DbID            uint            `json:"dbID,omitempty"`
	StreamPorts     string          `json:"streamPorts,omitempty"`
	UDP             bool            `json:"udp"`
	Favorite        bool            `json:"favorite"`
	WebsiteGroupID  uint            `json:"webSiteGroupId"`
	Protocol        string          `json:"protocol,omitempty"`
	ExpireDate      time.Time       `json:"expireDate,omitempty"`
	SSLExpireDate   *time.Time      `json:"sslExpireDate,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	Domains         []WebsiteDomain `json:"domains,omitempty"`
}

// WebsiteCreateRequest 创建网站时使用的兼容字段集合。
type WebsiteCreateRequest struct {
	Name            string          `json:"name"`
	AppType         string          `json:"appType"`
	PrimaryDomain   string          `json:"primaryDomain"`
	Alias           string          `json:"alias"`
	Type            string          `json:"type"`
	Remark          string          `json:"remark"`
	SiteDir         string          `json:"siteDir"`
	WebsiteGroupID  uint            `json:"webSiteGroupId"`
	Protocol        string          `json:"protocol"`
	HttpConfig      string          `json:"httpConfig"`
	Proxy           string          `json:"proxy"`
	ProxyType       string          `json:"proxyType"`
	WebsiteSSLID    uint            `json:"websiteSSLId"`
	SSLID           uint            `json:"SSLID"`
	RuntimeID       string          `json:"runtimeID"`
	AppInstallID    uint            `json:"appInstallId"`
	FtpID           uint            `json:"ftpId"`
	ParentWebsiteID uint            `json:"parentWebsiteID"`
	ErrorLog        *bool           `json:"errorLog"`
	AccessLog       *bool           `json:"accessLog"`
	DefaultServer   bool            `json:"defaultServer"`
	IPV6            bool            `json:"IPV6"`
	Rewrite         string          `json:"rewrite"`
	User            string          `json:"user"`
	Group           string          `json:"group"`
	DbType          string          `json:"dbType"`
	DbID            uint            `json:"dbID"`
	StreamPorts     string          `json:"streamPorts"`
	UDP             bool            `json:"udp"`
	Domains         []WebsiteDomain `json:"domains"`
}

// WebsiteUpdateRequest 更新网站元数据。
type WebsiteUpdateRequest struct {
	ID              uint       `json:"id"`
	Type            string     `json:"type,omitempty"`
	PrimaryDomain   string     `json:"primaryDomain"`
	Alias           string     `json:"alias"`
	Remark          string     `json:"remark"`
	SiteDir         string     `json:"siteDir"`
	Favorite        bool       `json:"favorite"`
	WebsiteGroupID  uint       `json:"webSiteGroupId"`
	ExpireDate      *time.Time `json:"expireDate"`
	IPV6            *bool      `json:"IPV6"`
	WebsiteSSLID    *uint      `json:"websiteSSLId"`
	Protocol        string     `json:"protocol"`
	HttpConfig      string     `json:"httpConfig"`
	Proxy           string     `json:"proxy"`
	ProxyType       string     `json:"proxyType"`
	ErrorLog        *bool      `json:"errorLog"`
	AccessLog       *bool      `json:"accessLog"`
	DefaultServer   *bool      `json:"defaultServer"`
	Rewrite         string     `json:"rewrite"`
	RuntimeID       *string    `json:"runtimeID"`
	AppInstallID    *uint      `json:"appInstallId"`
	FtpID           *uint      `json:"ftpId"`
	ParentWebsiteID *uint      `json:"parentWebsiteID"`
	User            string     `json:"user"`
	Group           string     `json:"group"`
	DbType          string     `json:"dbType"`
	DbID            *uint      `json:"dbID"`
	StreamPorts     string     `json:"streamPorts"`
	UDP             *bool      `json:"udp"`
}

// WebsiteDeleteRequest 删除网站。
type WebsiteDeleteRequest struct {
	ID uint `json:"id"`
}

// WebsiteDomain 表示网站绑定的域名记录。
type WebsiteDomain struct {
	ID        string `json:"id"`
	WebsiteID uint   `json:"websiteID"`
	Domain    string `json:"domain"`
	Port      int    `json:"port,omitempty"`
	SSL       bool   `json:"ssl"`
	Remark    string `json:"remark,omitempty"`
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
	Version            string            `json:"version"`
	Enabled            bool              `json:"enabled"`
	DefaultHTTPS       bool              `json:"defaultHttps"`
	SSLRejectHandshake bool              `json:"sslRejectHandshake"`
	ConfigContent      string            `json:"configContent,omitempty"`
	Modules            []OpenRestyModule `json:"modules"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

// OpenRestyModule 描述一个可加载模块。
type OpenRestyModule struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	BuildMode string `json:"buildMode,omitempty"`
	LoadOrder int    `json:"loadOrder,omitempty"`
}
