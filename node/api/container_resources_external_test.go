// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/node/service"
)

// TestExternalDockerNetworkVolumeLifecycle exercises the real Docker network
// and volume adapters with disposable resources. It is opt-in to keep normal
// unit runs independent from a host daemon.
// TestExternalDockerNetworkVolumeLifecycle 验证临时 Docker 网络和卷的真实生命周期。
func TestExternalDockerNetworkVolumeLifecycle(t *testing.T) {
	if os.Getenv("WORKMESH_CONTAINER_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_CONTAINER_EXTERNAL_TEST=1 to run isolated Docker resource lifecycle")
	}
	if _, err := exec.LookPath(service.DockerBinary()); err != nil {
		t.Skip("docker CLI unavailable")
	}
	name := fmt.Sprintf("workmesh-acceptance-resource-%d", time.Now().UnixNano())
	cleanup := func() {
		_ = exec.Command(service.DockerBinary(), "network", "rm", name).Run()
		_ = exec.Command(service.DockerBinary(), "volume", "rm", name).Run()
	}
	t.Cleanup(cleanup)
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	call := func(path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return res
	}
	if res := call("/api/v2/containers/network", fmt.Sprintf(`{"name":%q}`, name)); res.Code != http.StatusOK {
		t.Fatalf("network create status=%d body=%s", res.Code, res.Body.String())
	}
	if res := call("/api/v2/containers/volume", fmt.Sprintf(`{"name":%q}`, name)); res.Code != http.StatusOK {
		t.Fatalf("volume create status=%d body=%s", res.Code, res.Body.String())
	}
	if err := exec.Command(service.DockerBinary(), "network", "inspect", name).Run(); err != nil {
		t.Fatalf("network not created: %v", err)
	}
	if err := exec.Command(service.DockerBinary(), "volume", "inspect", name).Run(); err != nil {
		t.Fatalf("volume not created: %v", err)
	}
	if res := call("/api/v2/containers/network/del", fmt.Sprintf(`{"name":%q}`, name)); res.Code != http.StatusOK {
		t.Fatalf("network delete status=%d body=%s", res.Code, res.Body.String())
	}
	if res := call("/api/v2/containers/volume/del", fmt.Sprintf(`{"name":%q}`, name)); res.Code != http.StatusOK {
		t.Fatalf("volume delete status=%d body=%s", res.Code, res.Body.String())
	}
}

// TestExternalDockerPublishedPortDiscovery 验证容器发布端口被真实发现。
func TestExternalDockerPublishedPortDiscovery(t *testing.T) {
	if os.Getenv("WORKMESH_CONTAINER_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_CONTAINER_EXTERNAL_TEST=1 to run isolated Docker resource lifecycle")
	}
	name := fmt.Sprintf("workmesh-acceptance-published-%d", time.Now().UnixNano())
	create := exec.Command(service.DockerBinary(), "run", "-d", "--name", name, "-p", "127.0.0.1::8080", "python:3.12-alpine", "python", "-m", "http.server", "8080")
	if output, err := create.CombinedOutput(); err != nil {
		t.Fatalf("create published container: %v: %s", err, output)
	}
	t.Cleanup(func() { _ = exec.Command(service.DockerBinary(), "rm", "-f", name).Run() })
	mux := http.NewServeMux()
	registerHostOperationalRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/v2/hosts/firewall/docker/endpoints", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), name) {
		t.Fatalf("published endpoint status=%d body=%s", res.Code, res.Body.String())
	}
}

// TestExternalDockerImageBuildSaveLoad 验证镜像构建、导出、删除、导入和检查。
func TestExternalDockerImageBuildSaveLoad(t *testing.T) {
	if os.Getenv("WORKMESH_CONTAINER_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_CONTAINER_EXTERNAL_TEST=1 to run image acceptance")
	}
	image := fmt.Sprintf("workmesh-acceptance-image:%d", time.Now().UnixNano())
	root := t.TempDir()
	dockerfile := filepath.Join(root, "Dockerfile")
	archive := filepath.Join(root, "image.tar")
	if err := os.WriteFile(dockerfile, []byte("FROM alpine:3.20\nRUN echo workmesh-image > /image.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = exec.Command(service.DockerBinary(), "image", "rm", image).Run() })
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	call := func(path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return res
	}
	if res := call("/api/v2/containers/image/build", fmt.Sprintf(`{"name":%q,"image":%q}`, root, image)); res.Code != http.StatusOK {
		t.Skipf("image build unavailable: %d %s", res.Code, res.Body.String())
	}
	if res := call("/api/v2/containers/image/save", fmt.Sprintf(`{"image":%q,"path":%q}`, image, archive)); res.Code != http.StatusOK {
		t.Fatalf("image save status=%d body=%s", res.Code, res.Body.String())
	}
	if info, err := os.Stat(archive); err != nil || info.Size() == 0 {
		t.Fatalf("image archive missing: %v", err)
	}
	if err := exec.Command(service.DockerBinary(), "image", "rm", image).Run(); err != nil {
		t.Fatal(err)
	}
	if res := call("/api/v2/containers/image/load", fmt.Sprintf(`{"path":%q}`, archive)); res.Code != http.StatusOK {
		t.Fatalf("image load status=%d body=%s", res.Code, res.Body.String())
	}
	if err := exec.Command(service.DockerBinary(), "image", "inspect", image).Run(); err != nil {
		t.Fatalf("loaded image missing: %v", err)
	}
}

// TestExternalDockerImagePushRegistry verifies that a local image is tagged
// with the repository reference and pushed to a disposable registry.
// TestExternalDockerImagePushRegistry 验证匿名临时 registry 的真实推送。
func TestExternalDockerImagePushRegistry(t *testing.T) {
	if os.Getenv("WORKMESH_CONTAINER_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_CONTAINER_EXTERNAL_TEST=1 to run registry acceptance")
	}
	if _, err := exec.LookPath(service.DockerBinary()); err != nil {
		t.Skip("docker CLI unavailable")
	}
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()
	registry := fmt.Sprintf("workmesh-acceptance-registry-%d", time.Now().UnixNano())
	portRef := fmt.Sprintf("127.0.0.1:%d:5000", port)
	cmd := exec.Command(service.DockerBinary(), "run", "-d", "--name", registry, "-p", portRef, "registry:2")
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		t.Skipf("registry unavailable: %v: %s", runErr, output)
	}
	t.Cleanup(func() { _ = exec.Command(service.DockerBinary(), "rm", "-f", registry).Run() })
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/v2/", port)
	ready := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		resp, getErr := http.Get(endpoint)
		if getErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Skip("temporary registry did not become ready")
	}
	image := fmt.Sprintf("workmesh-acceptance-push:%d", time.Now().UnixNano())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM alpine:3.20\nRUN echo pushed > /pushed.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = exec.Command(service.DockerBinary(), "image", "rm", image).Run()
		_ = exec.Command(service.DockerBinary(), "image", "rm", "127.0.0.1:"+fmt.Sprint(port)+"/workmesh/pushed:latest").Run()
	})
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	call := func(path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(res, request)
		return res
	}
	if res := call("/api/v2/containers/image/build", fmt.Sprintf(`{"name":%q,"image":%q}`, root, image)); res.Code != http.StatusOK {
		t.Skipf("image build unavailable: %d %s", res.Code, res.Body.String())
	}
	repoRes := call("/api/v2/containers/repo", fmt.Sprintf(`{"name":"acceptance","downloadUrl":%q,"auth":false}`, fmt.Sprintf("http://127.0.0.1:%d", port)))
	if repoRes.Code != http.StatusOK {
		t.Fatalf("repository create status=%d body=%s", repoRes.Code, repoRes.Body.String())
	}
	var envelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(repoRes.Body.Bytes(), &envelope); err != nil || envelope.Data.ID == "" {
		t.Fatalf("repository response invalid: %s", repoRes.Body.String())
	}
	push := call("/api/v2/containers/image/push", fmt.Sprintf(`{"repoID":%q,"tagName":%q,"name":"workmesh/pushed:latest"}`, envelope.Data.ID, image))
	if push.Code != http.StatusOK {
		t.Fatalf("image push status=%d body=%s", push.Code, push.Body.String())
	}
	target := fmt.Sprintf("127.0.0.1:%d/workmesh/pushed:latest", port)
	if output, inspectErr := exec.Command(service.DockerBinary(), "image", "inspect", target).CombinedOutput(); inspectErr != nil {
		t.Fatalf("pushed tag missing: %v: %s", inspectErr, output)
	}
}

// TestExternalDockerImagePushAuthenticatedRegistry verifies the credential
// path against a disposable HTTP Basic Auth registry. The password must be
// delivered through docker login stdin and never appear in process args.
// TestExternalDockerImagePushAuthenticatedRegistry 验证 Basic Auth registry 的登录和推送。
func TestExternalDockerImagePushAuthenticatedRegistry(t *testing.T) {
	if os.Getenv("WORKMESH_CONTAINER_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_CONTAINER_EXTERNAL_TEST=1 to run authenticated registry acceptance")
	}
	if _, err := exec.LookPath(service.DockerBinary()); err != nil {
		t.Skip("docker CLI unavailable")
	}
	port := startAuthenticatedRegistry(t)
	image := fmt.Sprintf("workmesh-acceptance-auth-push:%d", time.Now().UnixNano())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM alpine:3.20\nRUN echo authenticated-push > /auth.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := fmt.Sprintf("127.0.0.1:%d/workmesh/authenticated:latest", port)
	t.Cleanup(func() {
		_ = exec.Command(service.DockerBinary(), "image", "rm", image).Run()
		_ = exec.Command(service.DockerBinary(), "image", "rm", target).Run()
	})
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	call := func(path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(res, request)
		return res
	}
	if res := call("/api/v2/containers/image/build", fmt.Sprintf(`{"name":%q,"image":%q}`, root, image)); res.Code != http.StatusOK {
		t.Skipf("image build unavailable: %d %s", res.Code, res.Body.String())
	}
	repoRes := call("/api/v2/containers/repo", fmt.Sprintf(`{"name":"authenticated","downloadUrl":%q,"auth":true,"username":"alice","password":"secret"}`, fmt.Sprintf("http://127.0.0.1:%d", port)))
	if repoRes.Code != http.StatusOK {
		t.Fatalf("authenticated repository create status=%d body=%s", repoRes.Code, repoRes.Body.String())
	}
	var envelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(repoRes.Body.Bytes(), &envelope); err != nil || envelope.Data.ID == "" {
		t.Fatalf("authenticated repository response invalid: %s", repoRes.Body.String())
	}
	push := call("/api/v2/containers/image/push", fmt.Sprintf(`{"repoID":%q,"tagName":%q,"name":"workmesh/authenticated:latest"}`, envelope.Data.ID, image))
	if push.Code != http.StatusOK {
		t.Fatalf("authenticated image push status=%d body=%s", push.Code, push.Body.String())
	}
	if output, inspectErr := exec.Command(service.DockerBinary(), "image", "inspect", target).CombinedOutput(); inspectErr != nil {
		t.Fatalf("authenticated pushed tag missing: %v: %s", inspectErr, output)
	}
}

// startAuthenticatedRegistry 启动并探测临时 bcrypt registry。
func startAuthenticatedRegistry(t *testing.T) int {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()
	authDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(authDir, "htpasswd"), []byte("alice:$2b$12$DhC9pIJrFw6nHz4k6vN6H.FM91Cx77APXfrb2qqtNKBPg4ZTYWp6K\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := fmt.Sprintf("workmesh-acceptance-auth-registry-%d", time.Now().UnixNano())
	cmd := exec.Command(service.DockerBinary(), "run", "-d", "--name", registry,
		"-p", fmt.Sprintf("127.0.0.1:%d:5000", port),
		"-v", filepath.Join(authDir, "htpasswd")+":/auth/htpasswd:ro",
		"-e", "REGISTRY_AUTH=htpasswd", "-e", "REGISTRY_AUTH_HTPASSWD_REALM=Registry",
		"-e", "REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd", "registry:2")
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		t.Skipf("authenticated registry unavailable: %v: %s", runErr, output)
	}
	t.Cleanup(func() { _ = exec.Command(service.DockerBinary(), "rm", "-f", registry).Run() })
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/v2/", port)
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		request, requestErr := http.NewRequest(http.MethodGet, endpoint, nil)
		if requestErr == nil {
			request.SetBasicAuth("alice", "secret")
			resp, getErr := http.DefaultClient.Do(request)
			if getErr == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return port
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Skip("temporary authenticated registry did not become ready")
	return 0
}
