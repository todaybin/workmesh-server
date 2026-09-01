// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
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

func registerHostRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/hosts/", hostRequest)
	mux.HandleFunc("GET /api/v2/hosts", hostRequest)
	mux.HandleFunc("POST /api/v2/hosts", hostRequest)
}

func isHostRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/hosts" || strings.HasPrefix(path, "/api/v2/hosts/")
}

func hostRequest(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/hosts"), "/")
	if r.Method == http.MethodPost && (path == "test/byinfo" || path == "test/byid") {
		host, err := hostForConnectionTest(r, path)
		if err != nil {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "INVALID_HOST_TEST"}, "message": err.Error()})
			return
		}
		connected, latency, probeErr := probeHost(host.Address, host.Port)
		result := map[string]any{"connected": connected, "latency": latency, "address": host.Address, "addr": host.Address, "port": host.Port}
		if probeErr != nil {
			result["error"] = probeErr.Error()
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": result})
		return
	}
	if r.Method == http.MethodPost && (path == "search" || path == "tree") {
		if path == "tree" {
			items, err := listHosts("", 0, 0, 0)
			if err != nil {
				wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
				return
			}
			groups := map[uint]map[string]any{}
			for _, item := range items {
				g := groups[item.GroupID]
				if g == nil {
					g = map[string]any{"id": item.GroupID, "label": item.GroupBelong, "children": []hostRecord{}}
					groups[item.GroupID] = g
				}
				g["children"] = append(g["children"].([]hostRecord), item)
			}
			out := make([]map[string]any, 0, len(groups))
			for _, g := range groups {
				out = append(out, g)
			}
			wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": out})
			return
		}
		var q struct {
			Info     string `json:"info"`
			Page     int    `json:"page"`
			PageSize int    `json:"pageSize"`
			GroupID  uint   `json:"groupID"`
		}
		if err := decodeJSON(r, &q); err != nil && err != io.EOF {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		if q.Page < 1 {
			q.Page = 1
		}
		if q.PageSize < 1 || q.PageSize > 200 {
			q.PageSize = 20
		}
		items, total, err := searchHosts(q.Info, q.GroupID, q.Page, q.PageSize)
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": total, "page": q.Page, "pageSize": q.PageSize}})
		return
	}
	if r.Method == http.MethodPost {
		handleHostMutation(w, r, path)
		return
	}
	hostname, _ := os.Hostname()
	base := map[string]any{"hostname": hostname, "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU(), "goroutines": runtime.NumGoroutine(), "timestamp": time.Now().UTC()}
	switch path {
	case "":
		items, err := listHosts("", 0, 0, 0)
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": len(items), "page": 1, "pageSize": len(items)}})
	case "system/info":
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": base})
	case "info":
		id := r.URL.Query().Get("id")
		item, ok, err := findHost(id)
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		if !ok {
			wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "message": "host not found"})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
	case "diagnostics/goroutines":
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"count": runtime.NumGoroutine(), "profiles": len(pprof.Profiles())}})
	case "diagnostics/summary":
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		base["heapAlloc"] = mem.HeapAlloc
		base["heapInuse"] = mem.HeapInuse
		base["numGC"] = mem.NumGC
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": base})
	case "tree":
		items, err := listHosts("", 0, 0, 0)
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		groups := map[uint]map[string]any{}
		for _, item := range items {
			g := groups[item.GroupID]
			if g == nil {
				g = map[string]any{"id": item.GroupID, "label": item.GroupBelong, "children": []hostRecord{}}
				groups[item.GroupID] = g
			}
			g["children"] = append(g["children"].([]hostRecord), item)
		}
		out := make([]map[string]any, 0, len(groups))
		for _, g := range groups {
			out = append(out, g)
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": out})
	case "search":
		var q struct {
			Info     string `json:"info"`
			Page     int    `json:"page"`
			PageSize int    `json:"pageSize"`
			GroupID  uint   `json:"groupID"`
		}
		if err := decodeJSON(r, &q); err != nil && err != io.EOF {
			wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		if q.Page < 1 {
			q.Page = 1
		}
		if q.PageSize < 1 || q.PageSize > 200 {
			q.PageSize = 20
		}
		items, total, err := searchHosts(q.Info, q.GroupID, q.Page, q.PageSize)
		if err != nil {
			wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"items": items, "total": total, "page": q.Page, "pageSize": q.PageSize}})
	default:
		wmhttp.JSON(w, 404, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "HOST_ROUTE_NOT_FOUND"}, "message": "host route not found"})
	}
}

func hostDB() (*sql.DB, error) {
	if db := sharedDB(); db != nil {
		return db, nil
	}
	return nil, fmt.Errorf("公共数据库未初始化")
}

// hostPayload 将主机凭据与普通元数据分离，凭据只以 AES-GCM 密文写入公共数据库。
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

// hostWithCredentials 仅供建立 SSH 会话使用，列表和详情接口不会返回或加载明文凭据。
func hostWithCredentials(id string) (hostRecord, error) {
	db, err := hostDB()
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
	err = db.QueryRow(`SELECT name,address,port,user_name,group_id,payload,created_at,updated_at FROM node_hosts WHERE id=?`, id).Scan(&name, &address, &port, &user, &groupID, &payload, &created, &updated)
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

func listHosts(info string, groupID uint, page, pageSize int) ([]hostRecord, error) {
	db, err := hostDB()
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
	rows, e := db.Query(q, args...)
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
func searchHosts(info string, groupID uint, page, pageSize int) ([]hostRecord, int, error) {
	db, err := hostDB()
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
	if err = db.QueryRow("SELECT COUNT(*) FROM node_hosts "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	items, err := listHosts(info, groupID, page, pageSize)
	return items, total, err
}
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
func handleHostMutation(w http.ResponseWriter, r *http.Request, path string) {
	var in map[string]any
	if err := decodeJSON(r, &in); err != nil {
		wmhttp.JSON(w, 400, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	item := hostRecord{ID: stringValue(in, "id"), Name: stringValue(in, "name"), Address: stringValue(in, "address", "addr"), Port: hostIntValue(in["port"]), User: stringValue(in, "user"), GroupID: uint(hostIntValue(in["groupID"])), GroupBelong: stringValue(in, "groupBelong"), AuthMode: stringValue(in, "authMode"), Description: stringValue(in, "description"), Password: stringValue(in, "password"), PrivateKey: stringValue(in, "privateKey"), PassPhrase: stringValue(in, "passPhrase"), RememberPassword: hostBoolValue(in["rememberPassword"])}
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
	db, err := hostDB()
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	payload, err := hostPayload(item)
	if err != nil {
		wmhttp.JSON(w, http.StatusUnauthorized, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "HOST_CREDENTIAL_KEY_UNAVAILABLE"}, "message": err.Error()})
		return
	}
	_, err = db.Exec(`INSERT INTO node_hosts(id,name,address,port,user_name,group_id,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,address=excluded.address,port=excluded.port,user_name=excluded.user_name,group_id=excluded.group_id,payload=excluded.payload,updated_at=excluded.updated_at`, item.ID, item.Name, item.Address, item.Port, item.User, item.GroupID, payload, item.Created, item.Updated)
	if err != nil {
		wmhttp.JSON(w, 500, map[string]any{"code": "ERR", "message": "persist host: " + err.Error()})
		return
	}
	item.Password = ""
	item.PrivateKey = ""
	item.PassPhrase = ""
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
}
func deleteHost(id string) error {
	db, err := hostDB()
	if err != nil {
		return err
	}
	res, err := db.Exec("DELETE FROM node_hosts WHERE id=?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("host not found")
	}
	return nil
}
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
func stringValue(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
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
func hostBoolValue(v any) bool { b, _ := v.(bool); return b }
func loadHosts() []hostRecord {
	hostStoreMu.Lock()
	defer hostStoreMu.Unlock()
	items, err := listHosts("", 0, 0, 0)
	if err != nil {
		return []hostRecord{}
	}
	return items
}
