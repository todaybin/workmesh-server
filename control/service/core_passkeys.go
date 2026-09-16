// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"crypto/subtle"
	"errors"
	"strings"
	"time"
)

// ListPasskeys 返回当前服务已登记的 Passkey 元数据。
func (s *CoreService) ListPasskeys() []Passkey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Passkey, 0, len(s.passkeys))
	for _, item := range s.passkeys {
		result = append(result, item)
	}
	return result
}

// BeginPasskeyRegistration 创建短时注册挑战；挑战本身不包含凭据秘密。
func (s *CoreService) BeginPasskeyRegistration(sessionID string) (string, error) {
	if _, err := s.Current(sessionID); err != nil {
		return "", err
	}
	id := randomToken()
	s.mu.Lock()
	s.passkeySessions[id] = time.Now().Add(5 * time.Minute)
	s.mu.Unlock()
	return id, nil
}

// FinishPasskeyRegistration 持久化浏览器提交的凭据标识，并拒绝重复注册。
func (s *CoreService) FinishPasskeyRegistration(authSessionID, challengeID, credentialID, name string) (Passkey, error) {
	if _, err := s.Current(authSessionID); err != nil {
		return Passkey{}, err
	}
	if strings.TrimSpace(credentialID) == "" {
		return Passkey{}, errors.New("credentialId 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	challenge, ok := s.passkeySessions[challengeID]
	if !ok || time.Now().After(challenge) {
		return Passkey{}, errors.New("Passkey 注册会话无效或已过期")
	}
	for _, item := range s.passkeys {
		if subtle.ConstantTimeCompare([]byte(item.CredentialID), []byte(credentialID)) == 1 {
			return Passkey{}, errors.New("Passkey 凭据已存在")
		}
	}
	if strings.TrimSpace(name) == "" {
		name = "Passkey"
	}
	item := Passkey{ID: randomToken()[:16], Name: strings.TrimSpace(name), CredentialID: strings.TrimSpace(credentialID), CreatedAt: time.Now().UTC()}
	s.passkeys[item.ID] = item
	if err := s.savePasskeysLocked(); err != nil {
		delete(s.passkeys, item.ID)
		return Passkey{}, err
	}
	delete(s.passkeySessions, challengeID)
	return item, nil
}

// DeletePasskey 删除指定凭据。
func (s *CoreService) DeletePasskey(sessionID, id string) error {
	if _, err := s.Current(sessionID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.passkeys[id]; !ok {
		return errors.New("Passkey 不存在")
	}
	item := s.passkeys[id]
	delete(s.passkeys, id)
	if err := s.savePasskeysLocked(); err != nil {
		s.passkeys[id] = item
		return err
	}
	return nil
}
