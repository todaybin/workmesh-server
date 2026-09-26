// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type hostRecord struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Address          string `json:"addr"`
	Port             int    `json:"port"`
	User             string `json:"user,omitempty"`
	GroupID          uint   `json:"groupID,omitempty"`
	GroupBelong      string `json:"groupBelong,omitempty"`
	AuthMode         string `json:"authMode,omitempty"`
	Description      string `json:"description,omitempty"`
	Password         string `json:"-"`
	PrivateKey       string `json:"-"`
	PassPhrase       string `json:"-"`
	RememberPassword bool   `json:"rememberPassword,omitempty"`
	Created          string `json:"createdAt"`
	Updated          string `json:"updatedAt"`
}

type hostCredentials struct {
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"`
	PassPhrase string `json:"passPhrase,omitempty"`
}

type storedHostPayload struct {
	Host        hostRecord `json:"host"`
	Credentials string     `json:"credentials,omitempty"`
}

var hostStoreMu sync.Mutex

// registerHostRoutes 注册主机 v2 列表和子路由。
func registerHostRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/hosts/", hostRequest)
	mux.HandleFunc("GET /api/v2/hosts", hostRequest)
	mux.HandleFunc("POST /api/v2/hosts", hostRequest)
	mux.HandleFunc("POST /api/v2/hosts/diagnostics/profiles", handleRuntimeProfile)
}

// isHostRoute 判断路由模式是否属于主机接口。
func isHostRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/hosts" || strings.HasPrefix(path, "/api/v2/hosts/")
}

// hostRequest 分发主机 HTTP 请求，保持原有 v2 路由和响应封装。
func hostRequest(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/hosts"), "/")
	// SSH 日志使用真实文件处理器，保持与主机其余通用资源相同的 v2 路由入口。
	switch path {
	case "ssh/log":
		if r.Method == http.MethodPost {
			loadSSHLogs(w, r)
			return
		}
	case "ssh/log/clean":
		if r.Method == http.MethodPost {
			cleanSSHLogs(w, r)
			return
		}
	case "ssh/log/export":
		if r.Method == http.MethodPost {
			exportSSHLogs(w, r)
			return
		}
	}
	if hostRequestTest(w, r, path) || hostRequestCollection(w, r, path) {
		return
	}
	if r.Method == http.MethodPost {
		handleHostMutation(w, r, path)
		return
	}
	hostRequestRead(w, r, path)
}

// hostWithCredentials 仅供建立 SSH 会话使用，列表和详情接口不会返回或加载明文凭据。
func hostWithCredentials(id string) (hostRecord, error) {
	repository, err := hostRepository()
	if err != nil {
		return hostRecord{}, err
	}
	var name, address, user, created, updated string
	var port int
	var groupID uint
	var payload []byte
	id = strings.TrimSpace(id)
	if id == "" {
		return hostRecord{}, fmt.Errorf("主机 ID 不能为空")
	}
	err = repository.QueryRow(`SELECT name,address,port,user_name,group_id,payload,created_at,updated_at FROM node_hosts WHERE id=?`, id).Scan(&name, &address, &port, &user, &groupID, &payload, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return hostRecord{}, fmt.Errorf("主机不存在: %s", id)
	}
	if err != nil {
		return hostRecord{}, err
	}
	item := hostFromRow(id, name, address, user, port, groupID, payload, created, updated)
	var stored storedHostPayload
	if err := json.Unmarshal(payload, &stored); err != nil || stored.Credentials == "" {
		return hostRecord{}, fmt.Errorf("主机 %s 未保存可用认证凭据", id)
	}
	credentials, err := decryptHostCredentials(stored.Credentials)
	if err != nil {
		return hostRecord{}, err
	}
	item.Password, item.PrivateKey, item.PassPhrase = credentials.Password, credentials.PrivateKey, credentials.PassPhrase
	return item, nil
}

// listHosts 从 SQLite 查询主机列表，支持名称、地址、分组和分页筛选。
func listHosts(info string, groupID uint, page, pageSize int) ([]hostRecord, error) {
	repository, err := hostRepository()
	if err != nil {
		return nil, err
	}
	args := []any{}
	where := "WHERE 1=1"
	if strings.TrimSpace(info) != "" {
		where += " AND (lower(name) LIKE ? OR lower(address) LIKE ?)"
		p := "%" + strings.ToLower(strings.TrimSpace(info)) + "%"
		args = append(args, p, p)
	}
	if groupID > 0 {
		where += " AND group_id=?"
		args = append(args, groupID)
	}
	q := `SELECT id,name,address,port,user_name,group_id,payload,created_at,updated_at FROM node_hosts ` + where + ` ORDER BY name,id`
	if page > 0 {
		q += " LIMIT ? OFFSET ?"
		args = append(args, pageSize, (page-1)*pageSize)
	}
	rows, e := repository.Query(q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []hostRecord{}
	for rows.Next() {
		var id, name, address, user, created, updated string
		var port int
		var gid uint
		var payload []byte
		if e := rows.Scan(&id, &name, &address, &port, &user, &gid, &payload, &created, &updated); e != nil {
			return nil, e
		}
		out = append(out, hostFromRow(id, name, address, user, port, gid, payload, created, updated))
	}
	return out, rows.Err()
}

// searchHosts 返回匹配主机及总数，供列表页面的分页查询使用。
func searchHosts(info string, groupID uint, page, pageSize int) ([]hostRecord, int, error) {
	repository, err := hostRepository()
	if err != nil {
		return nil, 0, err
	}
	var total int
	args := []any{}
	where := "WHERE 1=1"
	if strings.TrimSpace(info) != "" {
		where += " AND (lower(name) LIKE ? OR lower(address) LIKE ?)"
		p := "%" + strings.ToLower(strings.TrimSpace(info)) + "%"
		args = append(args, p, p)
	}
	if groupID > 0 {
		where += " AND group_id=?"
		args = append(args, groupID)
	}
	if err = repository.QueryRow("SELECT COUNT(*) FROM node_hosts "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	items, err := listHosts(info, groupID, page, pageSize)
	return items, total, err
}

// findHost 按主机 ID 查找不含明文凭据的主机信息。
func findHost(id string) (hostRecord, bool, error) {
	items, err := listHosts("", 0, 0, 0)
	if err != nil {
		return hostRecord{}, false, err
	}
	for _, item := range items {
		if item.ID == strings.TrimSpace(id) {
			return item, true, nil
		}
	}
	return hostRecord{}, false, nil
}

// hostForConnectionTest 解析按信息或按 ID 发起的主机连通性测试参数。
func hostForConnectionTest(r *http.Request, path string) (hostRecord, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return hostRecord{}, err
	}
	var in map[string]any
	if len(strings.TrimSpace(string(body))) == 0 || json.Unmarshal(body, &in) != nil {
		return hostRecord{}, fmt.Errorf("主机测试参数无效")
	}
	if path == "test/byid" {
		id := stringValue(in, "id")
		if id == "" {
			id = strconv.Itoa(hostIntValue(in["id"]))
		}
		item, ok, e := findHost(id)
		if e != nil {
			return hostRecord{}, e
		}
		if !ok {
			return hostRecord{}, fmt.Errorf("主机不存在: %s", id)
		}
		return item, nil
	}
	address := stringValue(in, "address", "addr")
	port := hostIntValue(in["port"])
	if address == "" || port < 1 || port > 65535 {
		return hostRecord{}, fmt.Errorf("主机地址或端口无效")
	}
	return hostRecord{Address: address, Port: port}, nil
}

// handleHostMutation 处理主机创建、更新、分组更新和删除操作。
func handleHostMutation(w http.ResponseWriter, r *http.Request, path string) {
	var in map[string]any
	if err := decodeJSON(r, &in); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	item := hostRecord{ID: stringValue(in, "id"), Name: stringValue(in, "name"), Address: stringValue(in, "address", "addr"), Port: hostIntValue(in["port"]), User: stringValue(in, "user"), GroupID: uint(hostIntValue(in["groupID"])), GroupBelong: stringValue(in, "groupBelong"), AuthMode: stringValue(in, "authMode"), Description: stringValue(in, "description"), Password: credentialStringValue(in, "password"), PrivateKey: credentialStringValue(in, "privateKey"), PassPhrase: credentialStringValue(in, "passPhrase"), RememberPassword: hostBoolValue(in["rememberPassword"])}
	preservedCipher := ""
	if item.Name == "" {
		item.Name = item.Address
	}
	if path == "" {
		if item.Address == "" || item.Port < 1 || item.Port > 65535 {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": "address and valid port are required"})
			return
		}
		item.ID = fmt.Sprintf("host-%d", time.Now().UnixNano())
		item.Created = time.Now().UTC().Format(time.RFC3339Nano)
		item.Updated = item.Created
	} else if path == "update" || path == "update/group" {
		old, ok, err := findHost(item.ID)
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		if !ok {
			wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "host not found"})
			return
		}
		item.Created = old.Created
		item.Updated = time.Now().UTC().Format(time.RFC3339Nano)
		if path == "update/group" {
			item = old
			item.GroupID = uint(hostIntValue(in["groupID"]))
			item.Updated = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if !hostHasNewCredential(item) {
			cipher, err := storedHostCredentialCipher(item.ID)
			if err != nil {
				wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "message": "读取主机凭据失败: " + err.Error()})
				return
			}
			preservedCipher = cipher
		}
	} else if path == "del" {
		if err := deleteHost(item.ID); err != nil {
			wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"id": item.ID}})
		return
	} else if path == "info" {
		old, ok, err := findHost(item.ID)
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		if !ok {
			wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "host not found"})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": old})
		return
	} else {
		wmhttp.JSON(w, 405, map[string]any{"code": "ERR", "message": "unsupported host operation"})
		return
	}
	repository, err := hostRepository()
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	payload, err := hostPayload(item, preservedCipher)
	if err != nil {
		wmhttp.JSON(w, http.StatusUnauthorized, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "HOST_CREDENTIAL_KEY_UNAVAILABLE"}, "message": err.Error()})
		return
	}
	_, err = repository.Exec(`INSERT INTO node_hosts(id,name,address,port,user_name,group_id,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,address=excluded.address,port=excluded.port,user_name=excluded.user_name,group_id=excluded.group_id,payload=excluded.payload,updated_at=excluded.updated_at`, item.ID, item.Name, item.Address, item.Port, item.User, item.GroupID, payload, item.Created, item.Updated)
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": "persist host: " + err.Error()})
		return
	}
	item.Password = ""
	item.PrivateKey = ""
	item.PassPhrase = ""
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}

// deleteHost 从 SQLite 删除指定主机记录。
func deleteHost(id string) error {
	repository, err := hostRepository()
	if err != nil {
		return err
	}
	res, err := repository.Exec("DELETE FROM node_hosts WHERE id=?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("host not found")
	}
	return nil
}

// probeHost 通过 TCP 连接探测主机地址并返回连通延迟。
func probeHost(address string, port int) (bool, int64, error) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(address, strconv.Itoa(port)))
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return false, latency, fmt.Errorf("连接 %s:%d 失败: %w", address, port, err)
	}
	_ = conn.Close()
	return true, latency, nil
}

// stringValue 从多个候选字段中读取第一个非空字符串。
func stringValue(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// credentialStringValue preserves whitespace in passwords and PEM/OpenSSH
// keys; trimming a private key can make the resulting file unreadable.
// credentialStringValue 保留凭据中的空白字符，尤其是 PEM 私钥换行。
func credentialStringValue(m map[string]any, key string) string {
	value, ok := m[key].(string)
	if !ok || value == "" {
		return ""
	}
	return value
}

// hostHasNewCredential 判断本次更新是否提交了新的认证材料。
func hostHasNewCredential(item hostRecord) bool {
	return item.Password != "" || item.PrivateKey != "" || item.PassPhrase != ""
}

// storedHostCredentialCipher 读取已保存的凭据密文，更新元数据时不解密也不清空。
func storedHostCredentialCipher(id string) (string, error) {
	repository, err := hostRepository()
	if err != nil {
		return "", err
	}
	var payload []byte
	err = repository.QueryRow(`SELECT payload FROM node_hosts WHERE id=?`, strings.TrimSpace(id)).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("主机不存在: %s", id)
	}
	if err != nil {
		return "", err
	}
	var stored storedHostPayload
	if json.Unmarshal(payload, &stored) != nil {
		return "", nil
	}
	return stored.Credentials, nil
}

// hostIntValue 将 JSON 中的数字或数字字符串统一转换为整数。
func hostIntValue(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	}
	return 0
}

// hostBoolValue 将 JSON 字段安全转换为布尔值。
func hostBoolValue(v any) bool { b, _ := v.(bool); return b }

// loadHosts 读取全部主机，供兼容调用方获取当前主机快照。
func loadHosts() []hostRecord {
	hostStoreMu.Lock()
	defer hostStoreMu.Unlock()
	items, err := listHosts("", 0, 0, 0)
	if err != nil {
		return []hostRecord{}
	}
	return items
}
