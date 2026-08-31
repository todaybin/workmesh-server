// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// BackupProviderMaxUpload 是单个云端备份请求允许的最大文件大小。
	BackupProviderMaxUpload       int64 = 128 << 20
	backupProviderBodyLimit       int64 = 2 << 20
	backupProviderTimeout               = 15 * time.Second
	backupProviderTransferTimeout       = 30 * time.Minute
)

// BackupBucket 表示云端存储桶的最小公共字段。
type BackupBucket struct {
	Name   string `json:"name"`
	Region string `json:"region,omitempty"`
}

// BackupTokenResult 是 OAuth 刷新响应，令牌只在服务端 Provider 内部流转。
type BackupTokenResult struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int64
}

// BackupProvider 抽象云端备份的读写操作，便于接入不同厂商或本地测试 Provider。
type BackupProvider interface {
	ListBuckets(context.Context) ([]BackupBucket, error)
	Upload(context.Context, string, string) error
	Delete(context.Context, string) error
	Check(context.Context) error
	RefreshToken(context.Context, string) (BackupTokenResult, error)
}

// HTTPBackupProvider 使用账号显式配置的 HTTP(S) 端点访问云端备份服务。
// 端点不得包含查询参数，令牌通过 Authorization 头发送，避免凭据泄漏到日志或代理。
type HTTPBackupProvider struct {
	accountType string
	accessToken string
	accessKey   string
	credential  string
	client      *http.Client
	vars        map[string]any
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelReadCloser) Close() error {
	r.cancel()
	return r.ReadCloser.Close()
}

// NewHTTPBackupProvider 创建云备份 Provider。未配置任何可用端点时返回错误，禁止伪造成功。
func NewHTTPBackupProvider(accountType string, vars map[string]any, accessKey, credential string) (*HTTPBackupProvider, error) {
	if vars == nil {
		vars = map[string]any{}
	}
	copyVars := make(map[string]any, len(vars))
	for key, value := range vars {
		copyVars[key] = value
	}
	p := &HTTPBackupProvider{
		accountType: strings.ToLower(strings.TrimSpace(accountType)),
		accessToken: stringValue(copyVars, "access_token", "token"),
		accessKey:   strings.TrimSpace(accessKey),
		credential:  strings.TrimSpace(credential),
		client:      &http.Client{Timeout: backupProviderTransferTimeout},
		vars:        copyVars,
	}
	if !p.hasEndpoint("buckets_url", "bucket_url", "endpoint", "upload_url", "delete_url", "check_url", "refresh_url", "token_url") {
		return nil, errors.New("云备份账号未配置可用 HTTP(S) 端点")
	}
	return p, nil
}

func stringValue(vars map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := vars[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (p *HTTPBackupProvider) hasEndpoint(keys ...string) bool {
	for _, key := range keys {
		if stringValue(p.vars, key) != "" {
			return true
		}
	}
	return false
}

func validateProviderURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("云备份端点必须是无查询参数的 HTTP(S) 地址")
	}
	return u, nil
}

func (p *HTTPBackupProvider) endpoint(keys ...string) (*url.URL, error) {
	for _, key := range keys {
		if raw := stringValue(p.vars, key); raw != "" {
			return validateProviderURL(raw)
		}
	}
	return nil, fmt.Errorf("云备份账号缺少端点配置: %s", strings.Join(keys, ","))
}

func (p *HTTPBackupProvider) authorize(req *http.Request) {
	if p.accessToken != "" {
		typ := stringValue(p.vars, "token_type")
		if typ == "" {
			typ = "Bearer"
		}
		req.Header.Set("Authorization", typ+" "+p.accessToken)
	}
	if p.accessKey != "" {
		req.Header.Set("X-Backup-Access-Key", p.accessKey)
	}
}

func (p *HTTPBackupProvider) do(ctx context.Context, req *http.Request, timeout time.Duration) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 || timeout > backupProviderTransferTimeout {
		timeout = backupProviderTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	req = req.WithContext(ctx)
	p.authorize(req)
	resp, err := p.client.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func (p *HTTPBackupProvider) requestWithRetry(ctx context.Context, build func() (*http.Request, error), timeout time.Duration) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := build()
		if err != nil {
			return nil, err
		}
		resp, err := p.do(ctx, req, timeout)
		if err == nil && !retryableStatus(resp.StatusCode) {
			return resp, nil
		}
		if resp != nil {
			_, _ = io.CopyN(io.Discard, resp.Body, backupProviderBodyLimit)
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("云备份端点返回 HTTP %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * 100 * time.Millisecond):
			}
		}
	}
	return nil, lastErr
}

// ListBuckets 获取云端 Bucket 列表并限制响应大小及条目数量。
func (p *HTTPBackupProvider) ListBuckets(ctx context.Context) ([]BackupBucket, error) {
	u, err := p.endpoint("buckets_url", "bucket_url", "endpoint")
	if err != nil {
		return nil, err
	}
	resp, err := p.requestWithRetry(ctx, func() (*http.Request, error) { return http.NewRequest(http.MethodGet, u.String(), nil) }, backupProviderTimeout)
	if err != nil {
		return nil, fmt.Errorf("查询云备份 Bucket 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("查询云备份 Bucket 失败: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, backupProviderBodyLimit+1))
	if err != nil {
		return nil, fmt.Errorf("读取 Bucket 响应失败: %w", err)
	}
	if int64(len(body)) > backupProviderBodyLimit {
		return nil, errors.New("Bucket 响应超过大小限制")
	}
	return decodeBuckets(body)
}

func decodeBuckets(body []byte) ([]BackupBucket, error) {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("Bucket 响应 JSON 无效")
	}
	var raw []any
	switch value := payload.(type) {
	case []any:
		raw = value
	case map[string]any:
		for _, key := range []string{"buckets", "items", "data"} {
			if list, ok := value[key].([]any); ok {
				raw = list
				break
			}
		}
	}
	if raw == nil {
		return nil, errors.New("Bucket 响应未返回列表")
	}
	if len(raw) > 500 {
		raw = raw[:500]
	}
	result := make([]BackupBucket, 0, len(raw))
	for _, entry := range raw {
		if text, ok := entry.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, BackupBucket{Name: strings.TrimSpace(text)})
			continue
		}
		obj, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		name := stringValue(obj, "name", "bucket", "id")
		if name == "" {
			continue
		}
		result = append(result, BackupBucket{Name: name, Region: stringValue(obj, "region", "location")})
	}
	return result, nil
}

// Upload 将本地备份以 multipart 流式上传到显式端点，避免将大文件一次读入内存。
func (p *HTTPBackupProvider) Upload(ctx context.Context, source, target string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("读取备份源失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("云备份上传源必须是普通文件")
	}
	if info.Size() > BackupProviderMaxUpload {
		return errors.New("云备份文件超过大小限制")
	}
	u, err := p.endpoint("upload_url", "upload_endpoint")
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		err = p.uploadOnce(ctx, u, source, target)
		if err == nil {
			return nil
		}
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt < 2 {
			wait := time.NewTimer(time.Duration(1<<attempt) * 100 * time.Millisecond)
			select {
			case <-wait.C:
			case <-ctx.Done():
				if !wait.Stop() {
					<-wait.C
				}
				return ctx.Err()
			}
		}
	}
	return fmt.Errorf("上传云备份失败: %w", err)
}

func (p *HTTPBackupProvider) uploadOnce(ctx context.Context, endpoint *url.URL, source, target string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	reader, writer := io.Pipe()
	mp := multipart.NewWriter(writer)
	writeErr := make(chan error, 1)
	go func() {
		defer writer.Close()
		part, partErr := mp.CreateFormFile("file", filepath.Base(source))
		if partErr == nil {
			_, partErr = io.Copy(part, io.LimitReader(file, BackupProviderMaxUpload+1))
		}
		if partErr == nil {
			partErr = mp.WriteField("path", target)
		}
		if closeErr := mp.Close(); partErr == nil {
			partErr = closeErr
		}
		writeErr <- partErr
	}()
	req, err := http.NewRequest(http.MethodPost, endpoint.String(), reader)
	if err != nil {
		_ = reader.Close()
		return err
	}
	req.Header.Set("Content-Type", mp.FormDataContentType())
	resp, err := p.do(ctx, req, backupProviderTransferTimeout)
	if err != nil {
		_ = reader.Close()
		return err
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, backupProviderBodyLimit)
	if err := <-writeErr; err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("云备份上传端点返回 HTTP %d", resp.StatusCode)
	}
	return nil
}

// Delete 删除云端指定对象，端点接收 JSON {"path":"..."}。
func (p *HTTPBackupProvider) Delete(ctx context.Context, target string) error {
	u, err := p.endpoint("delete_url", "delete_endpoint")
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"path": target})
	resp, err := p.requestWithRetry(ctx, func() (*http.Request, error) {
		req, reqErr := http.NewRequest(http.MethodPost, u.String(), strings.NewReader(string(body)))
		if reqErr == nil {
			req.Header.Set("Content-Type", "application/json")
		}
		return req, reqErr
	}, backupProviderTimeout)
	if err != nil {
		return fmt.Errorf("删除云备份失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("删除云备份失败: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Check 执行显式连通性检查，不会把缺少凭据的账号标记为成功。
func (p *HTTPBackupProvider) Check(ctx context.Context) error {
	u, err := p.endpoint("check_url", "buckets_url", "bucket_url", "endpoint")
	if err != nil {
		return err
	}
	resp, err := p.requestWithRetry(ctx, func() (*http.Request, error) { return http.NewRequest(http.MethodGet, u.String(), nil) }, backupProviderTimeout)
	if err != nil {
		return fmt.Errorf("检查云备份连接失败: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, backupProviderBodyLimit)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("云备份连接检查返回 HTTP %d", resp.StatusCode)
	}
	return nil
}

// RefreshToken 使用 OAuth refresh_token 授权方式刷新令牌，成功后由调用方持久化结果。
func (p *HTTPBackupProvider) RefreshToken(ctx context.Context, refreshToken string) (BackupTokenResult, error) {
	u, err := p.endpoint("refresh_url", "token_url")
	if err != nil {
		return BackupTokenResult{}, err
	}
	if strings.TrimSpace(refreshToken) == "" {
		return BackupTokenResult{}, errors.New("OAuth refresh_token 不能为空")
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}}
	if clientID := stringValue(p.vars, "client_id", "clientId"); clientID != "" {
		form.Set("client_id", clientID)
	}
	if clientSecret := stringValue(p.vars, "client_secret", "clientSecret"); clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	resp, err := p.requestWithRetry(ctx, func() (*http.Request, error) {
		req, reqErr := http.NewRequest(http.MethodPost, u.String(), strings.NewReader(form.Encode()))
		if reqErr == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return req, reqErr
	}, backupProviderTimeout)
	if err != nil {
		return BackupTokenResult{}, fmt.Errorf("刷新云备份令牌失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, backupProviderBodyLimit+1))
	if err != nil {
		return BackupTokenResult{}, fmt.Errorf("读取令牌响应失败: %w", err)
	}
	if int64(len(body)) > backupProviderBodyLimit {
		return BackupTokenResult{}, errors.New("令牌响应超过大小限制")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BackupTokenResult{}, fmt.Errorf("刷新端点返回 HTTP %d", resp.StatusCode)
	}
	var token BackupTokenResult
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || strings.TrimSpace(payload.AccessToken) == "" {
		return BackupTokenResult{}, errors.New("令牌响应缺少 access_token")
	}
	token.AccessToken, token.RefreshToken, token.TokenType, token.ExpiresIn = strings.TrimSpace(payload.AccessToken), strings.TrimSpace(payload.RefreshToken), strings.TrimSpace(payload.TokenType), payload.ExpiresIn
	return token, nil
}
