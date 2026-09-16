// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// websiteExtensionContractPaths 记录由统一分发器承接的旧公开契约，供静态实现扫描和文档生成使用。
var websiteExtensionContractPaths = []string{
	"GET /api/v2/websites/databases", "GET /api/v2/websites/default/html/:type",
	"POST /api/v2/websites/auths", "POST /api/v2/websites/auths/path", "POST /api/v2/websites/auths/path/update", "POST /api/v2/websites/auths/update",
	"POST /api/v2/websites/batch/group", "POST /api/v2/websites/batch/operate", "POST /api/v2/websites/batch/ssl", "POST /api/v2/websites/crosssite", "POST /api/v2/websites/databases", "POST /api/v2/websites/exec/composer", "POST /api/v2/websites/group/change", "POST /api/v2/websites/lbs/del",
	"POST /api/v2/websites/log/operate", "POST /api/v2/websites/log/search", "POST /api/v2/websites/monitor/logs/clear", "POST /api/v2/websites/monitor/logs/detail", "POST /api/v2/websites/monitor/logs/search", "POST /api/v2/websites/monitor/logs/stat", "POST /api/v2/websites/php/version",
	"POST /api/v2/websites/proxies", "POST /api/v2/websites/proxies/delete", "POST /api/v2/websites/proxies/file", "POST /api/v2/websites/proxies/status", "POST /api/v2/websites/proxies/update",
	"POST /api/v2/websites/templates/del", "POST /api/v2/websites/templates/get", "POST /api/v2/websites/templates/outputs", "POST /api/v2/websites/templates/outputs/del", "POST /api/v2/websites/templates/outputs/get", "POST /api/v2/websites/templates/outputs/search", "POST /api/v2/websites/templates/preview", "POST /api/v2/websites/templates/search", "POST /api/v2/websites/templates/update", "POST /api/v2/websites/templates/upload",
	"POST /api/v2/websites/waf/attack/stat", "POST /api/v2/websites/waf/block/search", "POST /api/v2/websites/waf/log/search", "POST /api/v2/websites/waf/relation/stat",
}

// websiteExtensionStore 保存旧网站扩展接口需要的本地元数据。
// 外部证书签发、DNS 验证和数据库服务仍由对应适配器负责，本存储只记录可恢复状态。
type websiteExtensionStore struct {
	mu         sync.Mutex
	path       string
	ACME       []map[string]any          `json:"acme"`
	Templates  []map[string]any          `json:"templates"`
	Outputs    []map[string]any          `json:"outputs"`
	Proxies    map[string]map[string]any `json:"proxies"`
	Auths      map[string]map[string]any `json:"auths"`
	Databases  []map[string]any          `json:"databases"`
	Logs       []map[string]any          `json:"logs"`
	NextID     uint64                    `json:"nextId"`
	db         *sql.DB
	repository storage.Transactional
	owner      *storage.Store
	loadErr    error
}

// newWebsiteExtensionStore 打开网站扩展共用的 SQLite 状态并补齐内存集合。
func newWebsiteExtensionStore() *websiteExtensionStore {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	s := &websiteExtensionStore{Proxies: map[string]map[string]any{}, Auths: map[string]map[string]any{}}
	if db := sharedDB(); db != nil {
		s.db = db
		var err error
		s.repository, err = storage.NewSQLiteRepository(db)
		if err != nil {
			s.loadErr = err
		}
	} else if opened, err := storage.Open(filepath.Join(root, "workmesh.db")); err == nil {
		s.owner, s.db = opened, opened.DB()
		s.repository, s.loadErr = opened.Repository()
	} else {
		s.loadErr = err
	}
	if s.repository != nil && s.loadErr == nil {
		if _, err := s.repository.Exec(`CREATE TABLE IF NOT EXISTS website_extension_state (id INTEGER PRIMARY KEY CHECK(id=1), payload BLOB NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
			s.loadErr = err
		}
		var data []byte
		if s.loadErr == nil {
			if err := s.repository.QueryRow("SELECT payload FROM website_extension_state WHERE id=1").Scan(&data); err == nil {
				_ = json.Unmarshal(data, s)
			}
		}
	}
	s.ensureCollections()
	return s
}

// ensureCollections 补齐旧 SQLite 快照中缺失的集合，避免路由处理时写入 nil map。
func (s *websiteExtensionStore) ensureCollections() {
	if s.Proxies == nil {
		s.Proxies = map[string]map[string]any{}
	}
	if s.Auths == nil {
		s.Auths = map[string]map[string]any{}
	}
	if s.ACME == nil {
		s.ACME = []map[string]any{}
	}
	if s.Templates == nil {
		s.Templates = []map[string]any{}
	}
	if s.Outputs == nil {
		s.Outputs = []map[string]any{}
	}
	if s.Databases == nil {
		s.Databases = []map[string]any{}
	}
	if s.Logs == nil {
		s.Logs = []map[string]any{}
	}
}

// persistLocked 在持有 store 锁时将扩展快照原子更新到 SQLite 单行状态表。
func (s *websiteExtensionStore) persistLocked() error {
	if s.loadErr != nil {
		return s.loadErr
	}
	if s.repository == nil {
		return errors.New("网站扩展公共数据库未初始化")
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	_, err = s.repository.Exec(`INSERT INTO website_extension_state(id,payload,updated_at) VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`, data, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func cloneWebsiteExtensionState(source *websiteExtensionStore) *websiteExtensionStore {
	payload, err := json.Marshal(source)
	if err != nil {
		clone := &websiteExtensionStore{}
		clone.ensureCollections()
		return clone
	}
	clone := &websiteExtensionStore{}
	if err := json.Unmarshal(payload, clone); err != nil {
		clone.ensureCollections()
		return clone
	}
	clone.ensureCollections()
	return clone
}

// id 生成仅用于扩展兼容记录的单调字符串 ID。
func (s *websiteExtensionStore) id() string {
	s.NextID++
	return strconv.FormatUint(s.NextID, 10)
}

// extensionJSON 使用网站扩展接口约定的成功 envelope 返回数据。
func extensionJSON(w http.ResponseWriter, data any) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}

// extensionError 使用网站扩展接口约定的错误 envelope 返回业务错误。
func extensionError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}
