// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/runtime/gateway"
)

// persistedGatewayState 是兼容文件中的绑定快照；正式运行态优先使用 SQLite。
type persistedGatewayState struct {
	Status             gateway.Status `json:"status"`
	BindingID          string         `json:"bindingId,omitempty"`
	Scopes             []string       `json:"scopes,omitempty"`
	ExpiresAt          string         `json:"expiresAt,omitempty"`
	Refreshable        bool           `json:"refreshable"`
	AccessToken        string         `json:"accessToken,omitempty"`
	GatewayURL         string         `json:"gatewayUrl,omitempty"`
	GatewayID          string         `json:"gatewayId,omitempty"`
	Account            string         `json:"account,omitempty"`
	MachineCode        string         `json:"machineCode,omitempty"`
	BoundMachineCode   string         `json:"boundMachineCode,omitempty"`
	FingerprintVersion int            `json:"fingerprintVersion,omitempty"`
	IdentityStatus     string         `json:"identityStatus,omitempty"`
	PreviousBindingID  string         `json:"previousBindingId,omitempty"`
	UpdatedAt          string         `json:"updatedAt"`
}

// persistedGatewayAuthorization 是仅用于本地存储的授权快照。
// Authorization.AccessToken 通过 json:"-" 隐藏在 API 响应中，但重启后恢复 Gateway 心跳仍需要它。
type persistedGatewayAuthorization struct {
	BindingID   string   `json:"bindingId"`
	Scopes      []string `json:"scopes"`
	ExpiresAt   string   `json:"expiresAt"`
	Refreshable bool     `json:"refreshable"`
	AccessToken string   `json:"accessToken,omitempty"`
}

// validateGatewayBaseURL 校验外部 Gateway 地址，拒绝凭据、查询和片段以避免 SSRF 与凭据泄露。
func validateGatewayBaseURL(value string) error {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(value), "/"))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && strings.TrimSpace(os.Getenv("WORKMESH_GATEWAY_ALLOW_HTTP")) == "1")) {
		return errors.New("Gateway 地址必须是无凭据 HTTPS URL；本地 HTTP 需显式启用 WORKMESH_GATEWAY_ALLOW_HTTP=1")
	}
	return nil
}

// load 从数据目录恢复绑定。损坏快照只会进入 pending，不会伪造已注册状态。
func (s *GatewayStateStore) load() {
	if s.repository != nil {
		var statusRaw, authRaw []byte
		var gatewayURL, account, savedMachineCode, boundMachineCode, identityStatus, previousBindingID string
		var fingerprintVersion int
		if err := s.repository.QueryRow(`SELECT status,auth,gateway_url,account,machine_code,bound_machine_code,fingerprint_version,identity_status,previous_binding_id FROM gateway_binding WHERE id=1`).Scan(&statusRaw, &authRaw, &gatewayURL, &account, &savedMachineCode, &boundMachineCode, &fingerprintVersion, &identityStatus, &previousBindingID); err == nil {
			var status gateway.Status
			var savedAuth persistedGatewayAuthorization
			if json.Unmarshal(statusRaw, &status) == nil && json.Unmarshal(authRaw, &savedAuth) == nil {
				auth := gateway.Authorization{
					BindingID: savedAuth.BindingID, Scopes: savedAuth.Scopes, ExpiresAt: savedAuth.ExpiresAt,
					Refreshable: savedAuth.Refreshable, AccessToken: savedAuth.AccessToken,
				}
				s.status, s.auth, s.gatewayURL, s.account = status, auth, gatewayURL, account
				s.boundMachineCode, s.previousBindingID = boundMachineCode, previousBindingID
				_ = savedMachineCode
				_ = fingerprintVersion
				if identityStatus != "" {
					s.identityStatus = identityStatus
				}
				if s.status.Registration == gateway.RegistrationRegistered && s.auth.BindingID == "" {
					s.status.Registration, s.status.Connected = gateway.RegistrationPending, false
				}
			}
		}
		return
	}
	raw, err := os.ReadFile(s.statePath)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		s.status.Registration = gateway.RegistrationPending
		s.status.Reason = fmt.Sprintf("读取 Gateway 绑定状态失败: %v", err)
		return
	}
	var saved persistedGatewayState
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Status.NodeID == "" {
		s.status.Registration = gateway.RegistrationPending
		s.status.Reason = "Gateway 绑定状态文件损坏"
		return
	}
	s.status = saved.Status
	s.gatewayURL = strings.TrimRight(strings.TrimSpace(saved.GatewayURL), "/")
	if s.status.GatewayID == "" {
		s.status.GatewayID = strings.TrimSpace(saved.GatewayID)
	}
	s.auth = gateway.Authorization{BindingID: saved.BindingID, Scopes: saved.Scopes, ExpiresAt: saved.ExpiresAt, Refreshable: saved.Refreshable, AccessToken: saved.AccessToken}
	s.account = saved.Account
	s.boundMachineCode, s.previousBindingID = saved.BoundMachineCode, saved.PreviousBindingID
	if saved.IdentityStatus != "" {
		s.identityStatus = saved.IdentityStatus
	}
	if s.status.Registration == gateway.RegistrationRegistered && s.auth.BindingID == "" {
		s.status.Registration, s.status.Connected = gateway.RegistrationPending, false
	}
}

// persist 将当前绑定原子写入数据目录，文件权限限制为仅所有者可读写。
func (s *GatewayStateStore) persist() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.persistSnapshot(s.status, s.auth)
}

// persistLocked 在调用方持有写锁时保存 Gateway 绑定快照。
func (s *GatewayStateStore) persistLocked() error { return s.persistSnapshot(s.status, s.auth) }

// persistSnapshot 将状态和授权写入 SQLite；无数据库时使用受限权限的兼容文件。
func (s *GatewayStateStore) persistSnapshot(status gateway.Status, auth gateway.Authorization) error {
	if s.repository != nil {
		statusRaw, err := json.Marshal(status)
		if err != nil {
			return err
		}
		authRaw, err := json.Marshal(persistedGatewayAuthorization{
			BindingID: auth.BindingID, Scopes: auth.Scopes, ExpiresAt: auth.ExpiresAt,
			Refreshable: auth.Refreshable, AccessToken: auth.AccessToken,
		})
		if err != nil {
			return err
		}
		gatewayURL := s.gatewayURL
		if configuredURL := strings.TrimSpace(os.Getenv("WORKMESH_GATEWAY_URL")); configuredURL != "" {
			gatewayURL = strings.TrimRight(configuredURL, "/")
		}
		_, err = s.repository.Exec(`INSERT INTO gateway_binding(id,status,auth,gateway_url,account,machine_code,bound_machine_code,fingerprint_version,identity_status,previous_binding_id,updated_at) VALUES(1,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,auth=excluded.auth,gateway_url=excluded.gateway_url,account=excluded.account,machine_code=excluded.machine_code,bound_machine_code=excluded.bound_machine_code,fingerprint_version=excluded.fingerprint_version,identity_status=excluded.identity_status,previous_binding_id=excluded.previous_binding_id,updated_at=excluded.updated_at`, statusRaw, authRaw, gatewayURL, s.account, s.machineCode, s.boundMachineCode, s.fingerprintVersion, s.identityStatus, s.previousBindingID, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o700); err != nil {
		return err
	}
	gatewayURL, account := s.gatewayURL, s.account
	if configuredURL := strings.TrimSpace(os.Getenv("WORKMESH_GATEWAY_URL")); configuredURL != "" {
		gatewayURL = strings.TrimRight(configuredURL, "/")
	}
	saved := persistedGatewayState{Status: status, BindingID: auth.BindingID, Scopes: auth.Scopes, ExpiresAt: auth.ExpiresAt, Refreshable: auth.Refreshable, AccessToken: auth.AccessToken, GatewayURL: gatewayURL, GatewayID: status.GatewayID, Account: account, MachineCode: s.machineCode, BoundMachineCode: s.boundMachineCode, FingerprintVersion: s.fingerprintVersion, IdentityStatus: s.identityStatus, PreviousBindingID: s.previousBindingID, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	if saved.ExpiresAt != "" {
		saved.Status.AuthorizationExpireAt = saved.ExpiresAt
	}
	raw, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.statePath), ".gateway-binding-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.statePath)
}

// removePersisted 删除 SQLite 或兼容文件中的绑定快照，供解绑操作调用。
func (s *GatewayStateStore) removePersisted() error {
	if s.repository != nil {
		_, err := s.repository.Exec(`DELETE FROM gateway_binding WHERE id=1`)
		return err
	}
	if err := os.Remove(s.statePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
