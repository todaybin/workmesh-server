// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// hostRepository 返回统一 SQLite repository，供主机运行期业务读写使用。
func hostRepository() (storage.Transactional, error) {
	return SharedRepository()
}

// hostPayload 将主机凭据与普通元数据分离，凭据只以 AES-GCM 密文写入数据库。
func hostPayload(item hostRecord) ([]byte, error) {
	credentials := hostCredentials{Password: item.Password, PrivateKey: item.PrivateKey, PassPhrase: item.PassPhrase}
	item.Password, item.PrivateKey, item.PassPhrase = "", "", ""
	payload := storedHostPayload{Host: item}
	if credentials.Password != "" || credentials.PrivateKey != "" || credentials.PassPhrase != "" {
		encoded, err := encryptHostCredentials(credentials)
		if err != nil {
			return nil, err
		}
		payload.Credentials = encoded
	}
	return json.Marshal(payload)
}

// hostFromRow 将 SQLite 行及其 JSON 元数据还原为不含明文凭据的主机对象。
func hostFromRow(id, name, address, user string, port int, groupID uint, payload []byte, created, updated string) hostRecord {
	var stored storedHostPayload
	var item hostRecord
	if json.Unmarshal(payload, &stored) == nil && stored.Host.ID != "" {
		item = stored.Host
	} else {
		// 兼容此前未加密的普通主机元数据；旧明文凭据不会再被读取或使用。
		_ = json.Unmarshal(payload, &item)
	}
	item.ID = id
	item.Name = name
	item.Address = address
	item.Port = port
	item.User = user
	item.GroupID = groupID
	item.Created = created
	item.Updated = updated
	return item
}

// hostCredentialKey 从环境配置派生固定长度的 AES 密钥。
func hostCredentialKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("WORKMESH_HOST_CREDENTIAL_KEY"))
	if raw == "" {
		return nil, fmt.Errorf("未配置主机凭据密钥 WORKMESH_HOST_CREDENTIAL_KEY")
	}
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) >= 16 {
		raw = string(decoded)
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], nil
}

// encryptHostCredentials 使用 AES-GCM 加密主机认证凭据并编码为文本。
func encryptHostCredentials(credentials hostCredentials) (string, error) {
	key, err := hostCredentialKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := json.Marshal(credentials)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(append(nonce, gcm.Seal(nil, nonce, plain, nil)...)), nil
}

// decryptHostCredentials 解码并验证 AES-GCM 主机认证凭据。
func decryptHostCredentials(encoded string) (hostCredentials, error) {
	key, err := hostCredentialKey()
	if err != nil {
		return hostCredentials{}, err
	}
	data, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return hostCredentials{}, fmt.Errorf("主机凭据密文无效: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return hostCredentials{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(data) < gcm.NonceSize() {
		return hostCredentials{}, fmt.Errorf("主机凭据密文无效")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return hostCredentials{}, fmt.Errorf("主机凭据无法解密: %w", err)
	}
	var credentials hostCredentials
	if err := json.Unmarshal(plain, &credentials); err != nil {
		return hostCredentials{}, fmt.Errorf("主机凭据格式无效: %w", err)
	}
	return credentials, nil
}
