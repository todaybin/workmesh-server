package service

// DNS provider construction is deliberately kept in the node service.  The
// credentials are read from SQLite only for the duration of an issuance and
// are never copied into API responses or logs.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-acme/lego/v5/challenge"
	"github.com/go-acme/lego/v5/providers/dns/alidns"
	"github.com/go-acme/lego/v5/providers/dns/cloudflare"
	"github.com/go-acme/lego/v5/providers/dns/huaweicloud"
	"github.com/go-acme/lego/v5/providers/dns/route53"
	"github.com/go-acme/lego/v5/providers/dns/tencentcloud"
)

type dnsCredentials struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	Email     string `json:"email"`
	APIKey    string `json:"apiKey"`
	APIUser   string `json:"apiUser"`
	APISecret string `json:"apiSecret"`
	SecretID  string `json:"secretID"`
	Region    string `json:"region"`
	Endpoint  string `json:"endpoint"`
}

func newDNSProvider(provider string, raw []byte, client *http.Client) (challenge.Provider, error) {
	var c dnsCredentials
	if len(raw) > 0 && json.Unmarshal(raw, &c) != nil {
		return nil, fmt.Errorf("DNS 账户凭据格式无效")
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	const timeout = 30 * time.Minute
	const poll = 10 * time.Second
	switch p {
	case "cloudflare", "cloudflareapi", "cloudflare_token":
		cfg := cloudflare.NewDefaultConfig()
		cfg.AuthEmail, cfg.AuthToken = c.Email, firstNonEmpty(c.APIKey, c.Token)
		cfg.PropagationTimeout, cfg.PollingInterval, cfg.HTTPClient = timeout, poll, client
		return cloudflare.NewDNSProviderConfig(cfg)
	case "aliyun", "alidns", "aliyun_dns", "aliyun.com":
		cfg := alidns.NewDefaultConfig()
		cfg.APIKey, cfg.SecretKey = firstNonEmpty(c.AccessKey, c.APIUser), c.SecretKey
		cfg.PropagationTimeout, cfg.PollingInterval = timeout, poll
		return alidns.NewDNSProviderConfig(cfg)
	case "tencentcloud", "dnspod", "tencent":
		cfg := tencentcloud.NewDefaultConfig()
		cfg.SecretID, cfg.SecretKey, cfg.Region = c.SecretID, c.SecretKey, c.Region
		cfg.PropagationTimeout, cfg.PollingInterval = timeout, poll
		return tencentcloud.NewDNSProviderConfig(cfg)
	case "awsroute53", "route53", "aws":
		cfg := route53.NewDefaultConfig()
		cfg.AccessKeyID, cfg.SecretAccessKey, cfg.Region, cfg.HostedZoneID = c.AccessKey, c.SecretKey, c.Region, c.Endpoint
		cfg.PropagationTimeout, cfg.PollingInterval = timeout, poll
		return route53.NewDNSProviderConfig(cfg)
	case "huaweicloud", "huaweidns":
		cfg := huaweicloud.NewDefaultConfig()
		cfg.AccessKeyID, cfg.SecretAccessKey, cfg.Region = c.AccessKey, c.SecretKey, c.Region
		cfg.PropagationTimeout, cfg.PollingInterval = timeout, poll
		return huaweicloud.NewDNSProviderConfig(cfg)
	default:
		return nil, fmt.Errorf("不支持 DNS provider %q", provider)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
