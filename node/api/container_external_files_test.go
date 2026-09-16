// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/todaybin/workmesh-server/node/service"
)

func TestExternalContainerFileAndLogEndpoints(t *testing.T) {
	if os.Getenv("WORKMESH_CONTAINER_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_CONTAINER_EXTERNAL_TEST=1 to run the isolated Docker lifecycle")
	}
	name := fmt.Sprintf("workmesh-acceptance-container-files-%d", time.Now().UnixNano())
	create := exec.Command(service.DockerBinary(), "run", "-d", "--name", name, "python:3.12-alpine", "python", "-c", `from pathlib import Path; import time; Path('/tmp/wm').mkdir(); Path('/tmp/wm/demo.txt').write_text('workmesh-file\n'); print('workmesh-log', flush=True); time.sleep(3600)`)
	if output, err := create.CombinedOutput(); err != nil {
		t.Fatalf("create isolated container: %v: %s", err, output)
	}
	t.Cleanup(func() { _ = exec.Command(service.DockerBinary(), "rm", "-f", name).Run() })
	for attempt := 0; attempt < 20; attempt++ {
		if err := exec.Command(service.DockerBinary(), "exec", name, "test", "-f", "/tmp/wm/demo.txt").Run(); err == nil {
			break
		}
		if attempt == 19 {
			t.Fatal("isolated container did not create fixture file")
		}
		time.Sleep(50 * time.Millisecond)
	}
	mux := http.NewServeMux()
	registerContainerRoutes(mux)
	call := func(path, body string) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return res
	}
	body := fmt.Sprintf(`{"containerID":%q,"path":"/tmp/wm/demo.txt"}`, name)
	content := call("/api/v2/containers/files/content", body)
	if content.Code != http.StatusOK || !strings.Contains(content.Body.String(), `"content":"workmesh-file\n"`) {
		t.Fatalf("content status=%d body=%s", content.Code, content.Body.String())
	}
	size := call("/api/v2/containers/files/size", body)
	if size.Code != http.StatusOK || !strings.Contains(size.Body.String(), `"data":14`) {
		t.Fatalf("size status=%d body=%s", size.Code, size.Body.String())
	}
	listBody := fmt.Sprintf(`{"containerID":%q,"path":"/tmp/wm"}`, name)
	list := call("/api/v2/containers/files/search", listBody)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"name":"demo.txt"`) {
		t.Fatalf("search status=%d body=%s", list.Code, list.Body.String())
	}
	download := call("/api/v2/containers/files/download", body)
	if download.Code != http.StatusOK || !bytes.Equal(download.Body.Bytes(), []byte("workmesh-file\n")) || !strings.Contains(download.Header().Get("Content-Disposition"), "demo.txt") {
		t.Fatalf("download status=%d headers=%v body=%q", download.Code, download.Header(), download.Body.Bytes())
	}
	logBody := fmt.Sprintf(`{"container":%q,"since":"all","tail":20}`, name)
	logs := call("/api/v2/containers/download/log", logBody)
	if logs.Code != http.StatusOK || !strings.Contains(logs.Body.String(), "workmesh-log") || !strings.Contains(logs.Header().Get("Content-Disposition"), ".log") {
		t.Fatalf("logs status=%d headers=%v body=%q", logs.Code, logs.Header(), logs.Body.String())
	}
}
