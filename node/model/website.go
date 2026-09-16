// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package model

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Website 是节点本地网站的最小元数据；配置内容由 OpenRestyConfig 单独保存。
type Website struct {
	ID            uint   `json:"id"`
	PrimaryDomain string `json:"primaryDomain"`
	Alias         string `json:"alias"`
	Type          string `json:"type"`
	Remark        string `json:"remark,omitempty"`
	SiteDir       string `json:"siteDir,omitempty"`
	// SitePath 和 Root 保留旧 Web 客户端的目录字段，始终与 SiteDir 指向同一站点管理目录。
	SitePath        string `json:"sitePath,omitempty"`
	Root            string `json:"root,omitempty"`
	Status          string `json:"status"`
	HttpConfig      string `json:"httpConfig,omitempty"`
	Proxy           string `json:"proxy,omitempty"`
	ProxyType       string `json:"proxyType,omitempty"`
	ErrorLog        bool   `json:"errorLog"`
	AccessLog       bool   `json:"accessLog"`
	DefaultServer   bool   `json:"defaultServer"`
	IPV6            bool   `json:"IPV6"`
	Rewrite         string `json:"rewrite,omitempty"`
	WebsiteSSLID    uint   `json:"websiteSSLId,omitempty"`
	RuntimeID       string `json:"runtimeID,omitempty"`
	AppInstallID    uint   `json:"appInstallId,omitempty"`
	AppInstallRef   string `json:"appInstallRef,omitempty"`
	FtpID           uint   `json:"ftpId,omitempty"`
	ParentWebsiteID uint   `json:"parentWebsiteID,omitempty"`
	User            string `json:"user,omitempty"`
	Group           string `json:"group,omitempty"`
	DbType          string `json:"dbType,omitempty"`
	DbID            uint   `json:"dbID,omitempty"`
	StreamPorts     string `json:"streamPorts,omitempty"`
	UDP             bool   `json:"udp"`
	// Algorithm and Servers are the real TCP/UDP upstream settings returned by
	// the original website form; keeping them on the website response avoids
	// forcing the frontend to make a second, incompatible request.
	Algorithm      string                `json:"algorithm,omitempty"`
	Servers        []WebsiteStreamServer `json:"servers,omitempty"`
	Favorite       bool                  `json:"favorite"`
	WebsiteGroupID uint                  `json:"webSiteGroupId"`
	Protocol       string                `json:"protocol,omitempty"`
	ExpireDate     time.Time             `json:"expireDate,omitempty"`
	SSLExpireDate  *time.Time            `json:"sslExpireDate,omitempty"`
	CreatedAt      time.Time             `json:"createdAt"`
	UpdatedAt      time.Time             `json:"updatedAt"`
	Domains        []WebsiteDomain       `json:"domains,omitempty"`
}

// WebsiteStreamServer describes one validated stream upstream target.
type WebsiteStreamServer struct {
	Server          string `json:"server"`
	Weight          int    `json:"weight,omitempty"`
	FailTimeout     int    `json:"failTimeout,omitempty"`
	FailTimeoutUnit string `json:"failTimeoutUnit,omitempty"`
	MaxFails        int    `json:"maxFails,omitempty"`
	MaxConns        int    `json:"maxConns,omitempty"`
	Flag            string `json:"flag,omitempty"`
}

// WebsiteCreateRequest 创建网站时使用的兼容字段集合。
type WebsiteCreateRequest struct {
	Name           string `json:"name"`
	AppType        string `json:"appType"`
	PrimaryDomain  string `json:"primaryDomain"`
	Alias          string `json:"alias"`
	Type           string `json:"type"`
	Remark         string `json:"remark"`
	SiteDir        string `json:"siteDir"`
	WebsiteGroupID uint   `json:"webSiteGroupId"`
	Protocol       string `json:"protocol"`
	HttpConfig     string `json:"httpConfig"`
	Proxy          string `json:"proxy"`
	ProxyType      string `json:"proxyType"`
	WebsiteSSLID   uint   `json:"websiteSSLId"`
	SSLID          uint   `json:"SSLID"`
	RuntimeID      string `json:"runtimeID"`
	AppInstallID   uint   `json:"appInstallId"`
	// AppInstallIDCompat/AppInstall 支持 1Panel 使用的大写 ID 和一键部署对象。
	AppInstallIDCompat uint             `json:"appInstallID"`
	AppInstallRef      string           `json:"appInstallRef,omitempty"`
	AppInstall         map[string]any   `json:"appInstall"`
	AppInstallLegacy   map[string]any   `json:"appinstall"`
	AppID              uint             `json:"appID"`
	FtpID              uint             `json:"ftpId"`
	ParentWebsiteID    uint             `json:"parentWebsiteID"`
	ErrorLog           *bool            `json:"errorLog"`
	AccessLog          *bool            `json:"accessLog"`
	DefaultServer      bool             `json:"defaultServer"`
	IPV6               bool             `json:"IPV6"`
	Rewrite            string           `json:"rewrite"`
	User               string           `json:"user"`
	Group              string           `json:"group"`
	DbType             string           `json:"dbType"`
	DbID               uint             `json:"dbID"`
	StreamPorts        string           `json:"streamPorts"`
	UDP                bool             `json:"udp"`
	Algorithm          string           `json:"algorithm"`
	Servers            []map[string]any `json:"servers"`
	Domains            []WebsiteDomain  `json:"domains"`
}

// UnmarshalJSON 兼容 1Panel 将 appInstallId 作为数字或字符串返回的两种协议。
// 业务层保留数字字段给旧客户端，同时用 AppInstallRef 保存非数字安装标识（例如 showdoc）。
func (r *WebsiteCreateRequest) UnmarshalJSON(data []byte) error {
	type alias WebsiteCreateRequest
	aux := struct {
		*alias
		AppInstallIDRaw       json.RawMessage `json:"appInstallId"`
		AppInstallIDCompatRaw json.RawMessage `json:"appInstallID"`
		AppInstallRaw         map[string]any  `json:"appInstall"`
		AppInstallLegacyRaw   map[string]any  `json:"appinstall"`
		AppIDRaw              json.RawMessage `json:"appID"`
	}{alias: (*alias)(r)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	r.AppInstall = aux.AppInstallRaw
	r.AppInstallLegacy = aux.AppInstallLegacyRaw
	parseID := func(raw json.RawMessage) (uint, string, error) {
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
			return 0, "", nil
		}
		var number uint64
		if err := json.Unmarshal(raw, &number); err == nil {
			if number > uint64(^uint(0)) {
				return 0, "", strconv.ErrRange
			}
			return uint(number), "", nil
		}
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return 0, "", err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return 0, "", nil
		}
		if parsed, err := strconv.ParseUint(text, 10, 64); err == nil {
			if parsed > uint64(^uint(0)) {
				return 0, "", strconv.ErrRange
			}
			return uint(parsed), "", nil
		}
		return 0, text, nil
	}
	for _, raw := range []json.RawMessage{aux.AppInstallIDRaw, aux.AppInstallIDCompatRaw} {
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		id, ref, err := parseID(raw)
		if err != nil {
			return err
		}
		if id != 0 {
			r.AppInstallID = id
		}
		if ref != "" {
			r.AppInstallRef = ref
		}
	}
	if id, ref, err := parseID(aux.AppIDRaw); err != nil {
		return err
	} else if r.AppInstallID == 0 && id != 0 {
		r.AppInstallID = id
	} else if r.AppInstallRef == "" && ref != "" {
		r.AppInstallRef = ref
	}
	for _, nested := range []map[string]any{r.AppInstall, r.AppInstallLegacy} {
		if r.AppInstallRef != "" || r.AppInstallID != 0 || nested == nil {
			continue
		}
		for _, key := range []string{"id", "appInstallId", "appInstallID", "appId", "appID", "key", "appkey"} {
			if raw, ok := nested[key]; ok {
				data, _ := json.Marshal(raw)
				id, ref, err := parseID(data)
				if err != nil {
					return err
				}
				if id != 0 {
					r.AppInstallID = id
				}
				if ref != "" {
					r.AppInstallRef = ref
				}
				break
			}
		}
	}
	return nil
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
	AppInstallRef   *string    `json:"appInstallRef"`
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
	ID           uint `json:"id"`
	DeleteApp    bool `json:"deleteApp"`
	DeleteBackup bool `json:"deleteBackup"`
	ForceDelete  bool `json:"forceDelete"`
	DeleteDB     bool `json:"deleteDB"`
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
	WebsiteID        uint                         `json:"websiteID"`
	Alias            string                       `json:"alias"`
	Enabled          bool                         `json:"enabled"`
	Mode             string                       `json:"mode"`
	DetectionLevel   int                          `json:"detectionLevel"`
	FrequencyEnabled bool                         `json:"frequencyEnabled"`
	RateLimits       map[string]WAFFrequencyLimit `json:"rateLimits,omitempty"`
	Rules            []WAFRule                    `json:"rules"`
}

// WAFGlobalConfig 是节点级 WAF 配置。
type WAFGlobalConfig struct {
	Enabled          bool                         `json:"enabled"`
	StandardRules    bool                         `json:"standardRules"`
	Mode             string                       `json:"mode"`
	ParanoiaLevel    int                          `json:"paranoiaLevel"`
	InboundThreshold int                          `json:"inboundThreshold"`
	RequestBodyLimit int64                        `json:"requestBodyLimit"`
	StrictMode       bool                         `json:"strictMode,omitempty"`
	Redis            *WAFRedisConfig              `json:"redis,omitempty"`
	Frequency        map[string]WAFFrequencyLimit `json:"frequency,omitempty"`
	FrequencyLimit   map[string]WAFFrequencyLimit `json:"frequencyLimit,omitempty"`
}

type WAFRedisConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password,omitempty"`
	DB       int    `json:"db,omitempty"`
}

type WAFFrequencyLimit struct {
	Enabled   bool   `json:"enabled"`
	Mode      string `json:"mode"`
	Period    int    `json:"period"`
	Count     int    `json:"count"`
	BlockTime int    `json:"blockTime"`
}

// WAFAccessLists 保存节点级黑白名单。
type WAFAccessLists struct {
	Whitelist    []string               `json:"whitelist"`
	Blacklist    []string               `json:"blacklist"`
	Enabled      map[string]bool        `json:"enabled,omitempty"`
	URLWhitelist []string               `json:"urlWhitelist,omitempty"`
	URLBlacklist []string               `json:"urlBlacklist,omitempty"`
	UAWhitelist  []string               `json:"uaWhitelist,omitempty"`
	UABlacklist  []string               `json:"uaBlacklist,omitempty"`
	IPGroups     []WAFIPGroup           `json:"ipGroups,omitempty"`
	ListMeta     map[string]WAFListMeta `json:"listMeta,omitempty"`
}

// WAFListMeta stores the row-level state and operator note shown by the
// blacklist/whitelist table. The legacy string arrays remain authoritative for
// matching and keep older clients compatible.
type WAFListMeta struct {
	Enabled bool   `json:"enabled"`
	Remark  string `json:"remark,omitempty"`
}

// WAFIPGroup is a named collection of IP/CIDR entries used by the access-list UI.
type WAFIPGroup struct {
	Name    string   `json:"name"`
	Entries []string `json:"entries"`
	Enabled bool     `json:"enabled"`
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
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Custom      bool   `json:"custom,omitempty"`
	Script      string `json:"script,omitempty"`
	Packages    string `json:"packages,omitempty"`
	Params      string `json:"params,omitempty"`
	BuildMode   string `json:"buildMode,omitempty"`
	Provider    string `json:"provider,omitempty"`
	LoadOrder   int    `json:"loadOrder,omitempty"`
	BuildStatus string `json:"buildStatus,omitempty"`
	LoadStatus  string `json:"loadStatus,omitempty"`
	LastError   string `json:"lastError,omitempty"`
}
