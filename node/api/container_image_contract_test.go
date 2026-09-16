// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContainerImageRemoveAcceptsLegacyPayloadsAndWritesTaskLog(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	logPath := filepath.Join(root, "docker.args")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatal(err)
	}
	docker := filepath.Join(bin, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\nprintf 'docker-output\\n'\n"
	if err := os.WriteFile(docker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("WORKMESH_DATA_DIR", filepath.Join(root, "data"))
	resetSharedStoreForTest()

	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	call := func(body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/containers/image/remove", strings.NewReader(body)))
		return res
	}
	for _, body := range []string{
		`{"names":["alpine:names"]}`,
		`{"imageName":["alpine:array"]}`,
		`{"imageName":"alpine:single"}`,
		`{"image":"alpine:image"}`,
		`{"name":"alpine:name"}`,
	} {
		if res := call(body); res.Code != http.StatusOK {
			t.Fatalf("image remove payload failed: %s status=%d body=%s", body, res.Code, res.Body.String())
		}
	}
	if res := call(`{"names":["  ",""],"imageName":["alpine:fallback"]}`); res.Code != http.StatusOK {
		t.Fatalf("blank names did not fall back to imageName: status=%d body=%s", res.Code, res.Body.String())
	}
	if res := call(`{}`); res.Code != http.StatusBadRequest {
		t.Fatalf("missing image name should return 400, got %d: %s", res.Code, res.Body.String())
	}
	if res := call(`{"imageName":123}`); res.Code != http.StatusBadRequest {
		t.Fatalf("invalid imageName should return 400, got %d: %s", res.Code, res.Body.String())
	}
	task := call(`{"names":["alpine:task"],"taskID":"image-remove-task"}`)
	if task.Code != http.StatusOK {
		t.Fatalf("task image remove failed: %d %s", task.Code, task.Body.String())
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(content)
	for _, want := range []string{
		"rmi alpine:names",
		"rmi alpine:array",
		"rmi alpine:single",
		"rmi alpine:image",
		"rmi alpine:name",
		"rmi alpine:fallback",
		"rmi alpine:task",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("docker invocation missing %q in %s", want, log)
		}
	}
	taskLog, err := os.ReadFile(appTaskLogPath("image-remove-task"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"开始删除 Docker 镜像", "docker-output", "Docker 镜像删除完成", "[TASK-END]"} {
		if !strings.Contains(string(taskLog), want) {
			t.Fatalf("task log missing %q: %s", want, taskLog)
		}
	}

	containerRemove := httptest.NewRecorder()
	mux.ServeHTTP(containerRemove, httptest.NewRequest(http.MethodPost, "/api/v2/containers/operate", strings.NewReader(`{"container":"container-one","operation":"remove"}`)))
	if containerRemove.Code != http.StatusOK {
		t.Fatalf("container remove failed: %d %s", containerRemove.Code, containerRemove.Body.String())
	}
	content, err = os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "rm container-one") {
		t.Fatalf("container removal did not execute docker rm: %s", content)
	}
}

// TestContainerImageFrontendPayloadsReachDocker 验证镜像页面字段映射和敏感参数隔离。
func TestContainerImageFrontendPayloadsReachDocker(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	logPath := filepath.Join(root, "docker.args")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatal(err)
	}
	docker := filepath.Join(bin, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + logPath + "\"\nif [ \"$1\" = login ]; then cat >/dev/null; fi\nif [ \"$1\" = push ] && [ \"$WORKMESH_FAKE_DOCKER_PUSH_FAIL\" = 1 ]; then exit 17; fi\n"
	if err := os.WriteFile(docker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("WORKMESH_DATA_DIR", filepath.Join(root, "data"))
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	call := func(path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return res
	}
	repo := call("/api/v2/containers/repo", `{"name":"private","downloadUrl":"registry.test:5000","auth":true,"username":"alice","password":"secret"}`)
	if repo.Code != http.StatusOK {
		t.Fatalf("repo status=%d body=%s", repo.Code, repo.Body.String())
	}
	var repoEnvelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(repo.Body.Bytes(), &repoEnvelope); err != nil || repoEnvelope.Data.ID == "" {
		t.Fatalf("invalid repo response: %s", repo.Body.String())
	}
	if res := call("/api/v2/containers/image/push", `{"repoID":"`+repoEnvelope.Data.ID+`","tagName":"alpine:latest","name":"alpine:latest"}`); res.Code != http.StatusOK {
		t.Fatalf("push status=%d body=%s", res.Code, res.Body.String())
	}
	if res := call("/api/v2/containers/image/pull", `{"repoID":"`+repoEnvelope.Data.ID+`","imageName":["alpine:latest"]}`); res.Code != http.StatusOK {
		t.Fatalf("pull status=%d body=%s", res.Code, res.Body.String())
	}
	if res := call("/api/v2/containers/image/tag", `{"sourceID":"alpine:latest","tags":["registry.test:5000/alpine:latest"]}`); res.Code != http.StatusOK {
		t.Fatalf("tag status=%d body=%s", res.Code, res.Body.String())
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(content)
	for _, want := range []string{"login registry.test:5000 --username alice --password-stdin", "push registry.test:5000/alpine:latest", "pull registry.test:5000/alpine:latest", "tag alpine:latest registry.test:5000/alpine:latest"} {
		if !strings.Contains(log, want) {
			t.Fatalf("docker invocation missing %q in %s", want, log)
		}
	}
	if strings.Contains(log, "secret") {
		t.Fatalf("registry password leaked into argv log: %s", log)
	}
	t.Setenv("WORKMESH_FAKE_DOCKER_PUSH_FAIL", "1")
	failed := call("/api/v2/containers/image/push", `{"repoID":"`+repoEnvelope.Data.ID+`","tagName":"alpine:latest","name":"alpine:failed"}`)
	if failed.Code == http.StatusOK {
		t.Fatalf("failed push unexpectedly succeeded: %s", failed.Body.String())
	}
	content, err = os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log = string(content)
	pushIndex := strings.Index(log, "push registry.test:5000/alpine:failed")
	rmiIndex := strings.Index(log, "rmi registry.test:5000/alpine:failed")
	if pushIndex < 0 || rmiIndex <= pushIndex {
		t.Fatalf("failed push did not rollback generated tag: %s", log)
	}
}
