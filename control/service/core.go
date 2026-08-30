// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

// ErrUnauthenticated 表示请求未提供有效的本机会话。
var ErrUnauthenticated = errors.New("未登录或会话已失效")

// User 描述本机登录用户，密码哈希不对外暴露。
type User struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	Password string    `json:"-"`
	Groups   []string  `json:"groups,omitempty"`
	MFA      bool      `json:"mfa"`
	API      APIConfig `json:"-"`
}

// APIConfig 保存用户 API 调用配置；密钥只在生成或当前用户查询时按契约返回。
type APIConfig struct {
	Enabled       bool
	Key           string
	IPWhiteList   string
	TrustedProxy  string
	ValidityHours int
}

// Session 保存一次本机登录会话及过期时间。
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// CoreService 提供轻量认证、会话、分组和设置存储；后续可替换为统一 Store 实现。
type CoreService struct {
	mu       sync.RWMutex
	users    map[string]User
	sessions map[string]Session
	groups   map[string]map[string]any
	settings map[string]string
}

// NewCoreService 创建默认管理员和基础设置。
func NewCoreService() *CoreService {
	return &CoreService{users: map[string]User{"admin": {ID: "admin", Name: "admin", Role: "ADMIN", Password: hashPassword("admin"), Groups: []string{"administrators"}}}, sessions: make(map[string]Session), groups: make(map[string]map[string]any), settings: map[string]string{"language": "zh", "theme": "system", "securityEntrance": ""}}
}

// Login 验证用户名和密码并创建 24 小时会话。
func (s *CoreService) Login(name, password string) (User, Session, error) {
	s.mu.RLock()
	user, ok := s.users[name]
	s.mu.RUnlock()
	if !ok || !verifyPassword(user.Password, password) {
		return User{}, Session{}, errors.New("用户名或密码错误")
	}
	id := randomToken()
	session := Session{ID: id, UserID: user.ID, ExpiresAt: time.Now().Add(24 * time.Hour)}
	s.mu.Lock()
	s.sessions[id] = session
	s.mu.Unlock()
	return publicUser(user), session, nil
}

// Logout 删除会话。
func (s *CoreService) Logout(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

// Current 返回有效会话对应用户。
func (s *CoreService) Current(sessionID string) (User, error) {
	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	if !ok {
		// API Key 是长期凭证，允许无 Cookie 的本机调用读取当前用户。
		for _, candidate := range s.users {
			if candidate.API.Enabled && candidate.API.Key != "" && subtle.ConstantTimeCompare([]byte(candidate.API.Key), []byte(sessionID)) == 1 {
				user := publicUser(candidate)
				s.mu.Unlock()
				return user, nil
			}
		}
		s.mu.Unlock()
		return User{}, errors.New("会话无效或已过期")
	}
	if time.Now().After(session.ExpiresAt) {
		delete(s.sessions, sessionID)
		s.mu.Unlock()
		return User{}, errors.New("会话无效或已过期")
	}
	user, ok := s.users[session.UserID]
	s.mu.Unlock()
	if !ok {
		return User{}, errors.New("会话用户不存在")
	}
	return publicUser(user), nil
}

// UpdateCurrentUser 修改当前用户的名称或密码；空字段表示保持原值。
func (s *CoreService) UpdateCurrentUser(sessionID, name, oldPassword, newPassword string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok || time.Now().After(session.ExpiresAt) {
		return User{}, errors.New("会话无效或已过期")
	}
	user, ok := s.users[session.UserID]
	if !ok {
		return User{}, errors.New("用户不存在")
	}
	if newPassword != "" && !verifyPassword(user.Password, oldPassword) {
		return User{}, errors.New("原密码错误")
	}
	if strings.TrimSpace(name) != "" && name != user.Name {
		oldName := user.Name
		if _, exists := s.users[name]; exists {
			return User{}, errors.New("用户名已存在")
		}
		delete(s.users, oldName)
		user.Name = strings.TrimSpace(name)
		for id, active := range s.sessions {
			if active.UserID == session.UserID {
				active.UserID = user.Name
				s.sessions[id] = active
			}
		}
	}
	if newPassword != "" {
		user.Password = hashPassword(newPassword)
	}
	s.users[user.Name] = user
	return publicUser(user), nil
}

// GenerateAPIKey 创建并返回当前用户的随机 API Key。
func (s *CoreService) GenerateAPIKey(sessionID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok || time.Now().After(session.ExpiresAt) {
		return "", errors.New("会话无效或已过期")
	}
	user, ok := s.users[session.UserID]
	if !ok {
		return "", errors.New("用户不存在")
	}
	user.API.Key = randomToken()
	user.API.Enabled = true
	s.users[user.Name] = user
	return user.API.Key, nil
}

// UpdateAPIConfig 更新当前用户 API 开关及白名单配置。
func (s *CoreService) UpdateAPIConfig(sessionID string, config APIConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok || time.Now().After(session.ExpiresAt) {
		return errors.New("会话无效或已过期")
	}
	user, ok := s.users[session.UserID]
	if !ok {
		return errors.New("用户不存在")
	}
	if config.Key != "" {
		user.API.Key = config.Key
	}
	user.API.Enabled = config.Enabled
	user.API.IPWhiteList = config.IPWhiteList
	user.API.TrustedProxy = config.TrustedProxy
	if config.ValidityHours > 0 {
		user.API.ValidityHours = config.ValidityHours
	}
	s.users[user.Name] = user
	return nil
}

// APIConfig 返回当前用户 API 配置的脱敏快照。
func (s *CoreService) APIConfig(sessionID string) (APIConfig, error) {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	var user User
	if ok {
		user = s.users[session.UserID]
	} else {
		for _, candidate := range s.users {
			if candidate.API.Enabled && candidate.API.Key != "" && subtle.ConstantTimeCompare([]byte(candidate.API.Key), []byte(sessionID)) == 1 {
				user = candidate
				ok = true
				break
			}
		}
	}
	s.mu.RUnlock()
	if !ok || session.ID != "" && time.Now().After(session.ExpiresAt) {
		return APIConfig{}, errors.New("会话无效或已过期")
	}
	return user.API, nil
}

// Groups 返回分组列表。
func (s *CoreService) Groups() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]map[string]any, 0, len(s.groups))
	for _, group := range s.groups {
		result = append(result, group)
	}
	return result
}

// UpsertGroup 创建或更新分组。
func (s *CoreService) UpsertGroup(id, name, kind string) map[string]any {
	if id == "" {
		id = randomToken()[:12]
	}
	item := map[string]any{"id": id, "name": name, "type": kind}
	s.mu.Lock()
	s.groups[id] = item
	s.mu.Unlock()
	return item
}

// DeleteGroup 删除分组。
func (s *CoreService) DeleteGroup(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.groups[id]; !ok {
		return errors.New("分组不存在")
	}
	delete(s.groups, id)
	return nil
}

// Settings 返回设置快照。
func (s *CoreService) Settings() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]string, len(s.settings))
	for key, value := range s.settings {
		result[key] = value
	}
	return result
}

// UpdateSettings 合并设置字段。
func (s *CoreService) UpdateSettings(values map[string]string) {
	s.mu.Lock()
	for key, value := range values {
		if strings.TrimSpace(key) != "" {
			s.settings[key] = value
		}
	}
	s.mu.Unlock()
}

func publicUser(user User) User { user.Password = ""; return user }

func hashPassword(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func verifyPassword(encoded, value string) bool {
	candidate := hashPassword(value)
	return subtle.ConstantTimeCompare([]byte(encoded), []byte(candidate)) == 1
}
func randomToken() string {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(raw[:])
}
