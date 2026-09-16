// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package model

import "time"

// WebsiteSSL 是网站证书元数据；PrivateKey 仅用于本机执行，序列化时必须脱敏。
type WebsiteSSL struct {
	ID            uint      `json:"id"`
	PrimaryDomain string    `json:"primaryDomain"`
	Domains       string    `json:"domains"`
	Certificate   string    `json:"certificate,omitempty"`
	PEM           string    `json:"pem,omitempty"`
	CertURL       string    `json:"certUrl,omitempty"`
	Type          string    `json:"type,omitempty"`
	Organization  string    `json:"organization,omitempty"`
	CAID          uint      `json:"caId,omitempty"`
	PrivateKey    string    `json:"-"`
	Provider      string    `json:"provider"`
	AcmeAccountID uint      `json:"acmeAccountId"`
	DnsAccountID  uint      `json:"dnsAccountId"`
	AutoRenew     bool      `json:"autoRenew"`
	ExpireDate    time.Time `json:"expireDate,omitempty"`
	StartDate     time.Time `json:"startDate,omitempty"`
	Status        string    `json:"status"`
	Message       string    `json:"message,omitempty"`
	KeyType       string    `json:"keyType,omitempty"`
	PushDir       bool      `json:"pushDir"`
	Dir           string    `json:"dir,omitempty"`
	Description   string    `json:"description,omitempty"`
	PushNode      bool      `json:"pushNode"`
	// Nodes 始终返回字符串，避免前端编辑证书时因字段缺失触发类型错误。
	Nodes          string                 `json:"nodes"`
	SkipDNS        bool                   `json:"skipDns"`
	Nameserver1    string                 `json:"nameserver1,omitempty"`
	Nameserver2    string                 `json:"nameserver2,omitempty"`
	DisableCNAME   bool                   `json:"disableCname"`
	ExecShell      bool                   `json:"execShell"`
	Shell          string                 `json:"shell,omitempty"`
	MasterSSLID    uint                   `json:"masterSslId,omitempty"`
	PushNodeFlag   bool                   `json:"pushNodeFlag"`
	PrivateKeyPath string                 `json:"privateKeyPath,omitempty"`
	CertPath       string                 `json:"certPath,omitempty"`
	IsIP           bool                   `json:"isIp"`
	AcmeAccount    *WebsiteSSLACMEAccount `json:"acmeAccount,omitempty"`
	DnsAccount     *WebsiteSSLDNSAccount  `json:"dnsAccount,omitempty"`
	Websites       []Website              `json:"websites"`
	LogPath        string                 `json:"logPath,omitempty"`
	CreatedAt      time.Time              `json:"createdAt"`
	UpdatedAt      time.Time              `json:"updatedAt"`
}

// WebsiteSSLACMEAccount 是证书响应中可公开的 ACME 账户信息。
type WebsiteSSLACMEAccount struct {
	ID       uint   `json:"id"`
	Email    string `json:"email"`
	URL      string `json:"url"`
	Type     string `json:"type"`
	KeyType  string `json:"keyType"`
	UseProxy bool   `json:"useProxy"`
}

// WebsiteSSLDNSAccount 是证书响应中不含凭据的 DNS 账户信息。
type WebsiteSSLDNSAccount struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// WebsiteSSLCreateRequest 创建证书元数据。
type WebsiteSSLCreateRequest struct {
	PrimaryDomain string `json:"primaryDomain"`
	OtherDomains  string `json:"otherDomains"`
	Provider      string `json:"provider"`
	AcmeAccountID uint   `json:"acmeAccountId"`
	DnsAccountID  uint   `json:"dnsAccountId"`
	AutoRenew     bool   `json:"autoRenew"`
	KeyType       string `json:"keyType"`
	Description   string `json:"description"`
	PushDir       bool   `json:"pushDir"`
	Dir           string `json:"dir"`
	SkipDNS       bool   `json:"skipDNS"`
	Nameserver1   string `json:"nameserver1"`
	Nameserver2   string `json:"nameserver2"`
	DisableCNAME  bool   `json:"disableCNAME"`
	ExecShell     bool   `json:"execShell"`
	Shell         string `json:"shell"`
	PushNode      bool   `json:"pushNode"`
	Nodes         string `json:"nodes"`
	IsIP          bool   `json:"isIp"`
	Apply         bool   `json:"apply"`
	ID            uint   `json:"id"`
	Certificate   string `json:"certificate"`
	PrivateKey    string `json:"privateKey"`
}

// WebsiteSSLUploadRequest 上传或导入证书内容。
type WebsiteSSLUploadRequest struct {
	ID          uint   `json:"sslID"`
	Type        string `json:"type"`
	PrivateKey  string `json:"privateKey"`
	Certificate string `json:"certificate"`
	Description string `json:"description"`
}

// WebsiteSSLUpdateRequest 更新证书设置。
type WebsiteSSLUpdateRequest struct {
	ID            uint   `json:"id"`
	PrimaryDomain string `json:"primaryDomain"`
	OtherDomains  string `json:"otherDomains"`
	Provider      string `json:"provider"`
	AutoRenew     bool   `json:"autoRenew"`
	Description   string `json:"description"`
	AcmeAccountID uint   `json:"acmeAccountId"`
	DnsAccountID  uint   `json:"dnsAccountId"`
	KeyType       string `json:"keyType"`
	PushDir       bool   `json:"pushDir"`
	Dir           string `json:"dir"`
	SkipDNS       bool   `json:"skipDNS"`
	Nameserver1   string `json:"nameserver1"`
	Nameserver2   string `json:"nameserver2"`
	DisableCNAME  bool   `json:"disableCNAME"`
	ExecShell     bool   `json:"execShell"`
	Shell         string `json:"shell"`
	PushNode      bool   `json:"pushNode"`
	Nodes         string `json:"nodes"`
	IsIP          bool   `json:"isIp"`
	Apply         bool   `json:"apply"`
}
