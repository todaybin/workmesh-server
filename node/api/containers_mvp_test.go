// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// installDockerStub 提供隔离的 Docker CLI，测试只验证参数和响应契约，不接触宿主 Docker。
func installDockerStub(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "docker")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestContainerMVPUsesRealDockerForListInspectAndImage(t *testing.T) {
	installDockerStub(t, `case "$1" in
ps) printf '%s\n' '{"ID":"abc123","Names":"web","Image":"nginx:latest","State":"running","Status":"Up 1 minute","CreatedAt":"2026-09-08 10:00:00 +0000 UTC","Ports":"80/tcp","Labels":""}' ;;
inspect) printf '%s\n' '{"Id":"abc123","Name":"/web","Config":{"Image":"nginx:latest"}}' ;;
image) printf '%s\n' '{"ID":"img123","Repository":"nginx","Tag":"latest","Size":"10MB","CreatedAt":"2026-09-08 10:00:00 +0000 UTC"}' ;;
version) printf '%s\n' '{"Server":{"Version":"26.0"}}' ;;
*) exit 0 ;;
esac`)
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	request := func(method, path, payload string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(method, path, bytes.NewBufferString(payload)))
		return res
	}
	list := request(http.MethodGet, "/api/v2/containers/list", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "web") {
		t.Fatalf("真实 Docker 列表失败: %d %s", list.Code, list.Body.String())
	}
	inspect := request(http.MethodPost, "/api/v2/containers/inspect", `{"id":"abc123","type":"container"}`)
	if inspect.Code != http.StatusOK || !strings.Contains(inspect.Body.String(), "nginx:latest") {
		t.Fatalf("真实 Docker inspect 失败: %d %s", inspect.Code, inspect.Body.String())
	}
	images := request(http.MethodPost, "/api/v2/containers/image/search", `{"name":"nginx"}`)
	if images.Code != http.StatusOK || !strings.Contains(images.Body.String(), "nginx:latest") {
		t.Fatalf("真实 Docker 镜像查询失败: %d %s", images.Code, images.Body.String())
	}
	options := request(http.MethodPost, "/api/v2/containers/list", `{}`)
	if options.Code != http.StatusOK || !strings.Contains(options.Body.String(), `"name":"web"`) {
		t.Fatalf("容器下拉列表失败: %d %s", options.Code, options.Body.String())
	}
	info := request(http.MethodPost, "/api/v2/containers/info", `{"name":"web"}`)
	if info.Code != http.StatusOK || !strings.Contains(info.Body.String(), "nginx:latest") {
		t.Fatalf("容器详情失败: %d %s", info.Code, info.Body.String())
	}
}

func TestContainerMVPComposeAndImageOperationsUseDockerAndPersistFiles(t *testing.T) {
	installDockerStub(t, `case "$1" in
compose) printf 'compose-ok\n' ;;
pull|rmi) printf 'operation-ok\n' ;;
*) exit 0 ;;
esac`)
	dir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dir)
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	req := func(path, payload string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)))
		return res
	}
	composePath := filepath.Join(dir, "compose", "demo", "docker-compose.yml")
	created := req("/api/v2/containers/compose", `{"name":"demo","path":"`+composePath+`","file":"services: {}"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("Compose 创建失败: %d %s", created.Code, created.Body.String())
	}
	if content, err := os.ReadFile(composePath); err != nil || string(content) != "services: {}" {
		t.Fatalf("Compose 文件未真实写入: %v %q", err, content)
	}
	updated := req("/api/v2/containers/compose/update", `{"path":"`+composePath+`","content":"services:\n  web:\n    image: nginx"}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("Compose 更新失败: %d %s", updated.Code, updated.Body.String())
	}
	operated := req("/api/v2/containers/compose/operate", `{"path":"`+composePath+`","operation":"config"}`)
	if operated.Code != http.StatusOK || !strings.Contains(operated.Body.String(), "compose-ok") {
		t.Fatalf("Compose 操作未执行真实 Docker: %d %s", operated.Code, operated.Body.String())
	}
	pulled := req("/api/v2/containers/image/pull", `{"image":"nginx:latest"}`)
	if pulled.Code != http.StatusOK || !strings.Contains(pulled.Body.String(), "operation-ok") {
		t.Fatalf("镜像拉取未执行真实 Docker: %d %s", pulled.Code, pulled.Body.String())
	}
	deleted := req("/api/v2/containers/compose/operate", `{"name":"demo","path":"`+composePath+`","operation":"delete","withFile":true}`)
	if deleted.Code != http.StatusOK {
		t.Fatalf("Compose 删除失败: %d %s", deleted.Code, deleted.Body.String())
	}
	if _, err := os.Stat(composePath); !os.IsNotExist(err) {
		t.Fatalf("withFile=true 后 Compose 文件仍存在: %v", err)
	}
}

func TestContainerMVPReturns503WhenDockerCLIUnavailable(t *testing.T) {
	// 保留真实 Docker CLI，但指向不存在的 socket，确保不会触碰宿主 daemon。
	t.Setenv("DOCKER_HOST", "unix:///workmesh-test/nonexistent/docker.sock")
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/containers/list", nil))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("Docker CLI 不可用应返回 503，实际 %d: %s", res.Code, res.Body.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil || envelope["code"] != "ERR" {
		t.Fatalf("Docker 不可用响应不是错误 envelope: %s", res.Body.String())
	}
}

func TestContainerStatsReturns503WhenDockerDaemonUnavailable(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///workmesh-test/nonexistent/docker.sock")
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	for _, path := range []string{"/api/v2/containers/list/stats", "/api/v2/containers/stats/example"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s Docker 不可用应返回 503，实际 %d: %s", path, res.Code, res.Body.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil || envelope["code"] != "ERR" {
			t.Fatalf("%s Docker 不可用响应不是错误 envelope: %s", path, res.Body.String())
		}
	}
}

// TestExternalContainerLifecycle uses a disposable Docker container to verify
// the real lifecycle and the stopped-container stats contract.
func TestExternalContainerLifecycle(t *testing.T) {
	if os.Getenv("WORKMESH_CONTAINER_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_CONTAINER_EXTERNAL_TEST=1 to run Docker container acceptance")
	}
	name := fmt.Sprintf("workmesh-acceptance-container-%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(res, req)
		return res
	}
	created := call(http.MethodPost, "/api/v2/containers", fmt.Sprintf(`{"image":"python:3.12-alpine","name":"%s","command":["python","-c","import time; time.sleep(30)"]}`, name))
	if created.Code != http.StatusOK {
		t.Fatalf("创建临时容器失败: %d %s", created.Code, created.Body.String())
	}
	running := call(http.MethodPost, "/api/v2/containers/search", `{"state":"running","page":1,"pageSize":100}`)
	if running.Code != http.StatusOK || !strings.Contains(running.Body.String(), name) {
		t.Fatalf("运行容器未出现在真实列表: %d %s", running.Code, running.Body.String())
	}
	stopped := call(http.MethodPost, "/api/v2/containers/operate", fmt.Sprintf(`{"container":"%s","operation":"stop"}`, name))
	if stopped.Code != http.StatusOK {
		t.Fatalf("停止临时容器失败: %d %s", stopped.Code, stopped.Body.String())
	}
	stats := call(http.MethodGet, "/api/v2/containers/list/stats", "")
	if stats.Code != http.StatusOK || strings.Contains(stats.Body.String(), name) {
		t.Fatalf("停止容器不应返回运行态 stats: %d %s", stats.Code, stats.Body.String())
	}
	restarted := call(http.MethodPost, "/api/v2/containers/operate", fmt.Sprintf(`{"container":"%s","operation":"start"}`, name))
	if restarted.Code != http.StatusOK {
		t.Fatalf("启动临时容器失败: %d %s", restarted.Code, restarted.Body.String())
	}
	stopped = call(http.MethodPost, "/api/v2/containers/operate", fmt.Sprintf(`{"container":"%s","operation":"stop"}`, name))
	if stopped.Code != http.StatusOK {
		t.Fatalf("删除前停止临时容器失败: %d %s", stopped.Code, stopped.Body.String())
	}
	deleted := call(http.MethodPost, "/api/v2/containers/operate", fmt.Sprintf(`{"container":"%s","operation":"remove"}`, name))
	if deleted.Code != http.StatusOK {
		t.Fatalf("删除临时容器失败: %d %s", deleted.Code, deleted.Body.String())
	}
}

// TestExternalComposeLifecycle verifies Compose file persistence and the
// create/up/stop/restart/down/delete path with a disposable project.
func TestExternalComposeLifecycle(t *testing.T) {
	if os.Getenv("WORKMESH_CONTAINER_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_CONTAINER_EXTERNAL_TEST=1 to run Docker Compose acceptance")
	}
	project := fmt.Sprintf("workmesh-acceptance-compose-%d", time.Now().UnixNano())
	root := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", root)
	composePath := filepath.Join(root, "compose", project, "docker-compose.yml")
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(res, req)
		return res
	}
	content := `services:
  worker:
    image: python:3.12-alpine
    command: ["python", "-c", "import time; time.sleep(60)"]
`
	created := call(http.MethodPost, "/api/v2/containers/compose", fmt.Sprintf(`{"name":%q,"path":%q,"content":%q}`, project, composePath, content))
	if created.Code != http.StatusOK {
		t.Fatalf("创建临时 Compose 失败: %d %s", created.Code, created.Body.String())
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "compose", "-f", composePath, "down", "--remove-orphans", "--volumes").Run()
	})

	for _, operation := range []string{"config", "up"} {
		res := call(http.MethodPost, "/api/v2/containers/compose/operate", fmt.Sprintf(`{"name":%q,"path":%q,"operation":%q}`, project, composePath, operation))
		if res.Code != http.StatusOK {
			t.Fatalf("Compose %s 失败: %d %s", operation, res.Code, res.Body.String())
		}
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("docker", "compose", "-f", composePath, "ps", "-q", "worker").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	search := call(http.MethodPost, "/api/v2/containers/compose/search", fmt.Sprintf(`{"name":%q,"page":1,"pageSize":100}`, project))
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), project) {
		t.Fatalf("Compose 项目未出现在真实列表: %d %s", search.Code, search.Body.String())
	}
	for _, operation := range []string{"stop", "start", "restart", "down"} {
		res := call(http.MethodPost, "/api/v2/containers/compose/operate", fmt.Sprintf(`{"name":%q,"path":%q,"operation":%q}`, project, composePath, operation))
		if res.Code != http.StatusOK {
			t.Fatalf("Compose %s 失败: %d %s", operation, res.Code, res.Body.String())
		}
	}
	deleted := call(http.MethodPost, "/api/v2/containers/compose/operate", fmt.Sprintf(`{"name":%q,"path":%q,"operation":"delete","withFile":true}`, project, composePath))
	if deleted.Code != http.StatusOK {
		t.Fatalf("删除临时 Compose 失败: %d %s", deleted.Code, deleted.Body.String())
	}
	if _, err := os.Stat(composePath); !os.IsNotExist(err) {
		t.Fatalf("删除后 Compose 文件仍存在: %v", err)
	}
}
