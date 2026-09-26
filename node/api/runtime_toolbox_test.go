// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
	"github.com/todaybin/workmesh-server/node/model"
)

type recordingRuntimeExecutor struct {
	mu       sync.Mutex
	requests []model.CommandRequest
	inspect  string
}

type failingExtensionExecutor struct {
	recordingRuntimeExecutor
	failFragment string
}

// Execute 执行运行时相关处理并返回可观测错误。
func (r *failingExtensionExecutor) Execute(ctx context.Context, request model.CommandRequest) (model.CommandResult, error) {
	result, err := r.recordingRuntimeExecutor.Execute(ctx, request)
	if strings.Contains(strings.Join(request.Args, " "), r.failFragment) {
		return model.CommandResult{ExitCode: 1, Stderr: "injected restart failure"}, errors.New("injected restart failure")
	}
	return result, err
}

// Execute 执行运行时相关处理并返回可观测错误。
func (r *recordingRuntimeExecutor) Execute(_ context.Context, request model.CommandRequest) (model.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
	if len(request.Args) > 0 && request.Args[0] == "inspect" {
		return model.CommandResult{Stdout: r.inspect}, nil
	}
	return model.CommandResult{}, nil
}

// commandLines 执行运行时相关处理并返回可观测错误。
func (r *recordingRuntimeExecutor) commandLines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	lines := make([]string, 0, len(r.requests))
	for _, request := range r.requests {
		lines = append(lines, request.Program+" "+strings.Join(request.Args, " "))
	}
	return lines
}

// TestOperateRuntimeContainerProvidesComposeDefaults 验证运行时相关功能、失败边界和持久化结果。
func TestOperateRuntimeContainerProvidesComposeDefaults(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(compose, []byte("services:\n  php:\n    image: ${IMAGE_NAME:-php}:latest\n    volumes:\n      - ${PANEL_WEBSITE_DIR}:/www\n    environment:\n      TZ: ${TZ}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := &recordingRuntimeExecutor{}
	item := runtimeRecord{ID: "php74", Type: "php", CodeDir: "/srv/site", ComposePath: compose, Params: map[string]any{"PHP_VERSION": "7.4"}}
	if err := operateRuntimeContainer(executor, item, "stop"); err != nil {
		t.Fatal(err)
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if len(executor.requests) != 2 {
		t.Fatalf("expected compose config and stop, got %d", len(executor.requests))
	}
	for _, request := range executor.requests {
		if request.Env["PANEL_WEBSITE_DIR"] != "/srv/site" || request.Env["TZ"] != "Asia/Shanghai" {
			t.Fatalf("compose defaults missing: %#v", request.Env)
		}
	}
}

// TestRuntimeContainerStatusRequiresRunning 验证运行时相关功能、失败边界和持久化结果。
func TestRuntimeContainerStatusRequiresRunning(t *testing.T) {
	executor := &recordingRuntimeExecutor{inspect: "running\n"}
	item := runtimeRecord{Container: "runtime-test"}
	status, err := runtimeContainerStatus(executor, item)
	if err != nil || status != "running" {
		t.Fatalf("running container status=%q err=%v", status, err)
	}

	executor.inspect = "exited\n"
	status, err = runtimeContainerStatus(executor, item)
	if err != nil || status != "exited" {
		t.Fatalf("exited container status=%q err=%v", status, err)
	}
}

// TestNodeRuntimePackageAndModules 验证运行时相关功能、失败边界和持久化结果。
func TestNodeRuntimePackageAndModules(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { resetSharedStoreForTest(); _ = store.Close() })
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"demo","scripts":{"build":"go build","test":"go test"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	moduleDir := filepath.Join(dir, "node_modules", "demo-module")
	if err := os.MkdirAll(moduleDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "package.json"), []byte(`{"name":"demo-module","version":"1.2.3","license":"MIT","description":"demo"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	createBody := map[string]any{"id": "node-1", "name": "Node", "type": "node", "version": "20", "codeDir": dir}
	createJSON, _ := json.Marshal(createBody)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes", bytes.NewReader(createJSON)))
	if create.Code != http.StatusOK {
		t.Fatalf("create runtime status=%d body=%s", create.Code, create.Body.String())
	}
	packageReq := httptest.NewRecorder()
	mux.ServeHTTP(packageReq, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/node/package", strings.NewReader(`{"codeDir":"`+strings.ReplaceAll(dir, `\`, `\\`)+`"}`)))
	if packageReq.Code != http.StatusOK || !strings.Contains(packageReq.Body.String(), `"build"`) {
		t.Fatalf("package scripts status=%d body=%s", packageReq.Code, packageReq.Body.String())
	}
	modulesReq := httptest.NewRecorder()
	mux.ServeHTTP(modulesReq, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/node/modules", strings.NewReader(`{"id":"node-1"}`)))
	if modulesReq.Code != http.StatusOK || !strings.Contains(modulesReq.Body.String(), "demo-module") {
		t.Fatalf("node modules status=%d body=%s", modulesReq.Code, modulesReq.Body.String())
	}
	badOperation := httptest.NewRecorder()
	mux.ServeHTTP(badOperation, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/node/modules/operate", strings.NewReader(`{"id":"node-1","operate":"install","pkgManager":"powershell","module":"x"}`)))
	if badOperation.Code != http.StatusBadRequest {
		t.Fatalf("invalid package manager status=%d body=%s", badOperation.Code, badOperation.Body.String())
	}
}

// TestRuntimeAndSSHRoutes 验证运行时相关功能、失败边界和持久化结果。
func TestRuntimeAndSSHRoutes(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { resetSharedStoreForTest(); _ = store.Close() })
	mux := http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes", strings.NewReader(`{"id":"php-82","name":"PHP 8.2","type":"php","version":"8.2"}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create runtime status=%d body=%s", create.Code, create.Body.String())
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/search", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "php-82") {
		t.Fatalf("runtime list body=%s", list.Body.String())
	}
	extensions := httptest.NewRecorder()
	mux.ServeHTTP(extensions, httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/php/extensions/search", strings.NewReader(`{"all":true,"page":1,"pageSize":100}`)))
	if extensions.Code != http.StatusOK || !strings.Contains(extensions.Body.String(), `"name":"Default"`) || !strings.Contains(extensions.Body.String(), `"name":"WordPress"`) {
		t.Fatalf("PHP extension search body=%s", extensions.Body.String())
	}
	detailExtensions := httptest.NewRecorder()
	mux.ServeHTTP(detailExtensions, httptest.NewRequest(http.MethodGet, "/api/v2/runtimes/php/php-82/extensions", nil))
	if detailExtensions.Code != http.StatusOK || !strings.Contains(detailExtensions.Body.String(), `"supportExtensions"`) || !strings.Contains(detailExtensions.Body.String(), `"name":"bcmath"`) {
		t.Fatalf("PHP extension detail body=%s", detailExtensions.Body.String())
	}
	ssh := httptest.NewRecorder()
	mux.ServeHTTP(ssh, httptest.NewRequest(http.MethodPost, "/api/v2/settings/ssh", strings.NewReader(`{"host":"127.0.0.1","password":"secret"}`)))
	if ssh.Code != http.StatusOK || strings.Contains(ssh.Body.String(), "secret") {
		t.Fatalf("ssh response leaked or failed: %s", ssh.Body.String())
	}
	for _, path := range []string{"/api/v2/runtimes/php/php-82/config", "/api/v2/runtimes/php/php-82/container", "/api/v2/runtimes/php/php-82/fpm/config", "/api/v2/runtimes/php/php-82/fpm/status"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusNotFound {
			t.Fatalf("runtime detail %s status=%d body=%s", path, res.Code, res.Body.String())
		}
	}
	missing := httptest.NewRecorder()
	mux.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v2/runtimes/php/missing/extensions", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing runtime status=%d body=%s", missing.Code, missing.Body.String())
	}
	supervisor := httptest.NewRecorder()
	mux.ServeHTTP(supervisor, httptest.NewRequest(http.MethodGet, "/api/v2/runtimes/supervisor/process/web", nil))
	if supervisor.Code != http.StatusNotFound {
		t.Fatalf("supervisor status=%d body=%s", supervisor.Code, supervisor.Body.String())
	}
}

func TestNodeModuleQueuePersistenceFailureDoesNotAcceptTask(t *testing.T) {
	dataDir := t.TempDir()
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	repository, err := storage.NewSQLiteRepository(store.DB())
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	runtimeStore := &runtimeStore{
		repository: runtimeRepository{repository: repository},
		state: runtimeState{
			Runtimes: []runtimeRecord{{ID: "node-1", Type: "node", Version: "20", Container: "node-container"}},
			Settings: map[string]any{},
		},
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/runtimes/node/modules/operate", strings.NewReader(`{"id":"node-1","operate":"install","pkgManager":"npm","module":"express"}`))
	nodeModuleOperationHandler(runtimeStore)(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when queue persistence fails, got=%d body=%s", res.Code, res.Body.String())
	}
	if _, exists := runtimeStore.state.Settings["node-task:"]; exists {
		t.Fatal("task should not be accepted after persistence failure")
	}
	for key := range runtimeStore.state.Settings {
		if strings.HasPrefix(key, "node-task:") {
			t.Fatalf("task snapshot should be rolled back, found %q", key)
		}
	}
}

func TestNodeModuleTaskPersistenceFailureStopsExternalCommand(t *testing.T) {
	dataDir := t.TempDir()
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	repository, err := storage.NewSQLiteRepository(store.DB())
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	runtimeStore := &runtimeStore{
		repository: runtimeRepository{repository: repository},
		state: runtimeState{
			Settings: map[string]any{
				"node-task:task-1": map[string]any{
					"id":     "task-1",
					"status": "queued",
				},
			},
		},
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	executor := &recordingRuntimeExecutor{}
	runNodeModuleTask(runtimeStore, "task-1", executor, runtimeRecord{ID: "node-1", Container: "node-container"}, "npm", "install", "express")

	if commands := executor.commandLines(); len(commands) != 0 {
		t.Fatalf("任务状态无法持久化时不应执行 Docker 命令: %v", commands)
	}
	value, ok := runtimeStore.state.Settings["node-task:task-1"].(map[string]any)
	if !ok || value["status"] != "queued" {
		t.Fatalf("任务状态未回滚到 queued: %#v", runtimeStore.state.Settings["node-task:task-1"])
	}
}

// TestRuntimeLifecycleRoutes 验证原前端 ID/operate 契约及 SQLite 生命周期持久化。
func TestRuntimeLifecycleRoutes(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtimeStoreMu.Lock()
		runtimeStoreInstance = nil
		runtimeStoreMu.Unlock()
		resetSharedStoreForTest()
		_ = store.Close()
	})
	executor := &recordingRuntimeExecutor{}
	s := &runtimeStore{
		path:       filepath.Join(dataDir, "workmesh.db"),
		repository: runtimeRepository{db: store.DB()},
		commands:   executor,
		state:      runtimeState{Runtimes: []runtimeRecord{}, Settings: map[string]any{}},
	}
	mux := http.NewServeMux()
	registerRuntimeRoutes(mux, s)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	created := call(http.MethodPost, "/api/v2/runtimes", `{"id":"lifecycle-go","name":"lifecycle-go","type":"go","version":"1.24","container":"lifecycle-go","port":28090}`)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"status":"Running"`) {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	for _, operation := range []string{"down", "up", "restart"} {
		res := call(http.MethodPost, "/api/v2/runtimes/operate", `{"ID":"lifecycle-go","operate":"`+operation+`"}`)
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"message":"success"`) {
			t.Fatalf("%s status=%d body=%s", operation, res.Code, res.Body.String())
		}
	}
	detail := call(http.MethodGet, "/api/v2/runtimes/lifecycle-go", "")
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"port":28090`) {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	reloaded, err := s.repository.load(context.Background())
	if err != nil || len(reloaded.Runtimes) != 1 || reloaded.Runtimes[0].ID != "lifecycle-go" {
		t.Fatalf("runtime not persisted: err=%v state=%#v", err, reloaded)
	}
	deleted := call(http.MethodPost, "/api/v2/runtimes/del", `{"id":"lifecycle-go","forceDelete":true}`)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	if state, loadErr := s.repository.load(context.Background()); loadErr != nil || len(state.Runtimes) != 0 {
		t.Fatalf("runtime delete not persisted: err=%v state=%#v", loadErr, state)
	}
	missing := call(http.MethodGet, "/api/v2/runtimes/lifecycle-go", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("deleted runtime status=%d body=%s", missing.Code, missing.Body.String())
	}
	joined := strings.Join(executor.commandLines(), "\n")
	for _, fragment := range []string{"docker stop lifecycle-go", "docker start lifecycle-go", "docker restart lifecycle-go"} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("missing lifecycle command %q in:\n%s", fragment, joined)
		}
	}
}

// TestRuntimeConcurrentDeleteAndOperate 验证同一运行时的操作和删除不会使用旧切片下标。
// 两个请求共享真实临时 SQLite；Docker 命令使用记录执行器，仅验证锁与状态提交顺序。
func TestRuntimeConcurrentDeleteAndOperate(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtimeStoreMu.Lock()
		runtimeStoreInstance = nil
		runtimeStoreMu.Unlock()
		resetSharedStoreForTest()
		_ = store.Close()
	})
	executor := &recordingRuntimeExecutor{}
	s := &runtimeStore{
		path:       filepath.Join(dataDir, "workmesh.db"),
		repository: runtimeRepository{db: store.DB()},
		commands:   executor,
		state:      runtimeState{Runtimes: []runtimeRecord{{ID: "race-runtime", Name: "race-runtime", Type: "go", Status: "Running", Container: "race-runtime", UpdatedAt: time.Now().UTC()}}, Settings: map[string]any{}},
	}
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerRuntimeRoutes(mux, s)
	call := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	results := make(chan int, 2)
	go func() { results <- call("/api/v2/runtimes/operate", `{"ID":"race-runtime","operate":"restart"}`).Code }()
	go func() { results <- call("/api/v2/runtimes/del", `{"ID":"race-runtime","forceDelete":true}`).Code }()
	for range 2 {
		status := <-results
		if status != http.StatusOK && status != http.StatusConflict && status != http.StatusNotFound {
			t.Fatalf("unexpected concurrent status: %d", status)
		}
	}
	results = make(chan int, 2)
	go func() { results <- call("/api/v2/runtimes/del", `{"ID":"race-runtime","forceDelete":true}`).Code }()
	go func() { results <- call("/api/v2/runtimes/del", `{"ID":"race-runtime","forceDelete":true}`).Code }()
	for range 2 {
		if status := <-results; status != http.StatusOK && status != http.StatusNotFound {
			t.Fatalf("double delete status=%d", status)
		}
	}
}

// TestPHPExtensionTemplatesPersistAndRuntimeParamsNormalize 验证运行时相关功能、失败边界和持久化结果。
func TestPHPExtensionTemplatesPersistAndRuntimeParamsNormalize(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtimeStoreMu.Lock()
		runtimeStoreInstance = nil
		runtimeStoreMu.Unlock()
		resetSharedStoreForTest()
		_ = store.Close()
	})

	item, err := runtimeRecordFromRequest(map[string]any{
		"name": "php74", "type": "php", "source": "https://mirrors.tuna.tsinghua.edu.cn",
		"params": map[string]any{"PHP_EXTENSIONS": []any{"Redis", "curl", "redis"}},
	})
	if err != nil || item.Params["CONTAINER_PACKAGE_URL"] != "https://mirrors.tuna.tsinghua.edu.cn" || item.Params["PHP_EXTENSIONS"] != "redis,curl" {
		t.Fatalf("PHP params not normalized: err=%v params=%#v", err, item.Params)
	}

	mux := http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	call := func(target, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, target, strings.NewReader(body)))
		return res
	}
	initial := call("/api/v2/runtimes/php/extensions/search", `{"all":true,"page":1,"pageSize":1}`)
	if initial.Code != http.StatusOK || !strings.Contains(initial.Body.String(), `"total":5`) || !strings.Contains(initial.Body.String(), `"name":"SeaCMS"`) {
		t.Fatalf("default PHP templates mismatch: %d %s", initial.Code, initial.Body.String())
	}
	created := call("/api/v2/runtimes/php/extensions", `{"name":"Custom","extensions":"redis,curl,redis"}`)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `"id":6`) || !strings.Contains(created.Body.String(), `"extensions":"redis,curl"`) {
		t.Fatalf("create PHP template failed: %d %s", created.Code, created.Body.String())
	}
	duplicate := call("/api/v2/runtimes/php/extensions", `{"name":"custom","extensions":"gd"}`)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate PHP template status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}

	runtimeStoreMu.Lock()
	runtimeStoreInstance = nil
	runtimeStoreMu.Unlock()
	mux = http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	persisted := call("/api/v2/runtimes/php/extensions/search", `{"all":true,"page":1,"pageSize":100}`)
	if persisted.Code != http.StatusOK || !strings.Contains(persisted.Body.String(), `"name":"Custom"`) {
		t.Fatalf("PHP template not restored: %d %s", persisted.Code, persisted.Body.String())
	}
	updated := call("/api/v2/runtimes/php/extensions/update", `{"id":6,"extensions":"gd,imagick"}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update PHP template failed: %d %s", updated.Code, updated.Body.String())
	}
	deleted := call("/api/v2/runtimes/php/extensions/del", `{"id":6}`)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete PHP template failed: %d %s", deleted.Code, deleted.Body.String())
	}
}

// TestToolboxGetDataUsesHostState 验证运行时相关功能、失败边界和持久化结果。
func TestToolboxGetDataUsesHostState(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	s := getRuntimeStore()
	users := toolboxGetData(s, "/api/v2/toolbox/device/users")
	if users["items"] == nil || users["total"] == nil {
		t.Fatalf("users response missing fields: %#v", users)
	}
	zones := toolboxGetData(s, "/api/v2/toolbox/device/zone/options")
	if zones["current"] == "" {
		t.Fatalf("timezone response missing current zone: %#v", zones)
	}
	ftp := toolboxGetData(s, "/api/v2/toolbox/ftp/base")
	if _, ok := ftp["isExist"].(bool); !ok {
		t.Fatalf("ftp response missing isExist: %#v", ftp)
	}
}

// TestToolboxDeviceDNSAndFTPState 验证运行时相关功能、失败边界和持久化结果。
func TestToolboxDeviceDNSAndFTPState(t *testing.T) {
	root := filepath.Join(".tmp", "toolbox-device-test")
	_ = os.RemoveAll(root)
	defer os.RemoveAll(root)
	t.Setenv("WORKMESH_DATA_DIR", root)
	t.Setenv("WORKMESH_PANEL_DIR", "")
	store, err := storage.Open(filepath.Join(root, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { resetSharedStoreForTest(); _ = store.Close() })
	t.Setenv("WORKMESH_FAIL2BAN_CONFIG", filepath.Join(root, "fail2ban.local"))
	mux := http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/device/base", `{}`); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"hostname"`) {
		t.Fatalf("设备基础信息失败: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/device/check/dns", `{"host":"localhost"}`); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"resolved":true`) {
		t.Fatalf("DNS 探测失败: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/device/check/dns", `{"host":"../etc/passwd"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("非法 DNS 主机名应拒绝: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodGet, "/api/v2/toolbox/ftp/base", ``); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"isExist"`) {
		t.Fatalf("FTP 状态失败: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/ftp/search", `{"info":"","page":1,"pageSize":20}`); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"items"`) {
		t.Fatalf("FTP 搜索失败: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/clam/base", `{}`); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"isExist"`) {
		t.Fatalf("ClamAV 状态失败: %d %s", res.Code, res.Body.String())
	}
	uploadDir := filepath.Join(root, "tmp", "upload")
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "junk.tmp"), []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/scan", `{}`); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "junk.tmp") || !strings.Contains(res.Body.String(), `"systemClean"`) {
		t.Fatalf("垃圾扫描失败: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/clean", `[{"treeType":"upload","name":"missing","size":1}]`); res.Code != http.StatusServiceUnavailable {
		t.Fatalf("未授权清理应拒绝: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/fail2ban/update", `{"content":"[sshd]\nenabled=true"}`); res.Code != http.StatusOK {
		t.Fatalf("Fail2ban 配置保存失败: %d %s", res.Code, res.Body.String())
	}
	if res := call(http.MethodPost, "/api/v2/toolbox/fail2ban/search", `{"keyword":"sshd"}`); res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "sshd") {
		t.Fatalf("Fail2ban 配置检索失败: %d %s", res.Code, res.Body.String())
	}
}

func TestTerminalAISettingsDefaultsValidationAndPersistence(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resetSharedStoreForTest()
		_ = store.Close()
	})

	mux := http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	search := func(router *http.ServeMux) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/settings/terminal/ai/search", nil))
		return res
	}
	update := func(router *http.ServeMux, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/settings/terminal/ai/update", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(res, req)
		return res
	}

	defaults := search(mux)
	var defaultEnvelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(defaults.Body.Bytes(), &defaultEnvelope); err != nil {
		t.Fatalf("终端 AI 默认响应不是 JSON: %v", err)
	}
	if defaultEnvelope.Data["aiStatus"] != "Disable" ||
		defaultEnvelope.Data["aiPrefix"] != "@ai" ||
		defaultEnvelope.Data["aiRiskCommands"] != terminalAIDefaultRiskCommands {
		t.Fatalf("终端 AI 默认字段错误: %s", defaults.Body.String())
	}
	defaultRisk, ok := defaultEnvelope.Data["aiRiskCommandsDefault"].(string)
	if !ok || !strings.HasPrefix(defaultRisk, `["rm","mkfs"`) {
		t.Fatalf("终端 AI 默认风险命令错误: %s", defaults.Body.String())
	}

	updated := update(mux, `{"aiStatus":"enable","aiAccountId":"account-1","aiPrefix":"#ai","aiRiskCommands":" [\" rm \",\"rm\",\" reboot \",\"\"] "}`)
	if updated.Code != http.StatusOK ||
		!strings.Contains(updated.Body.String(), `"aiStatus":"Enable"`) ||
		!strings.Contains(updated.Body.String(), `"aiRiskCommands":"[\"rm\",\"reboot\"]"`) {
		t.Fatalf("终端 AI 更新失败: %d %s", updated.Code, updated.Body.String())
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	failed := update(mux, `{"aiStatus":"Disable","aiPrefix":"!ai","aiRiskCommands":"[\"shutdown\"]"}`)
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("数据库关闭后更新应失败: %d %s", failed.Code, failed.Body.String())
	}

	resetSharedStoreForTest()
	restartedStore, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(restartedStore); err != nil {
		_ = restartedStore.Close()
		t.Fatal(err)
	}
	defer func() {
		resetSharedStoreForTest()
		_ = restartedStore.Close()
	}()
	restartedMux := http.NewServeMux()
	RegisterRuntimeToolboxRoutes(restartedMux)
	restarted := search(restartedMux)
	if restarted.Code != http.StatusOK ||
		!strings.Contains(restarted.Body.String(), `"aiStatus":"Enable"`) ||
		!strings.Contains(restarted.Body.String(), `"aiAccountId":"account-1"`) ||
		!strings.Contains(restarted.Body.String(), `"aiPrefix":"#ai"`) ||
		!strings.Contains(restarted.Body.String(), `"aiRiskCommands":"[\"rm\",\"reboot\"]"`) {
		t.Fatalf("终端 AI 重启恢复失败: %d %s", restarted.Code, restarted.Body.String())
	}
}

func TestTerminalAISettingsRejectInvalidParameters(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		resetSharedStoreForTest()
		_ = store.Close()
	})

	mux := http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	for _, body := range []string{
		"{\"aiStatus\":\"Enable\",\"aiAccountId\":\"\",\"aiPrefix\":\"@ai\",\"aiRiskCommands\":\"[]\"}",
		"{\"aiStatus\":\"Disable\",\"aiPrefix\":\"bad prefix\",\"aiRiskCommands\":\"[]\"}",
		"{\"aiStatus\":\"Disable\",\"aiPrefix\":\"中文\",\"aiRiskCommands\":\"[]\"}",
		"{\"aiStatus\":\"Disable\",\"aiPrefix\":\"@ai\",\"aiRiskCommands\":\"not-json\"}",
		"{\"aiStatus\":\"unknown\",\"aiPrefix\":\"@ai\",\"aiRiskCommands\":\"[]\"}",
	} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/settings/terminal/ai/update", strings.NewReader(body)))
		if res.Code != http.StatusBadRequest {
			t.Fatalf("非法终端 AI 参数应被拒绝: body=%s status=%d response=%s", body, res.Code, res.Body.String())
		}
	}
}
