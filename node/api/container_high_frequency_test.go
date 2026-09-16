// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/node/service"
)

func TestContainerHighFrequencyUsesContainerIDAndValidatesPaths(t *testing.T) {
	for _, tc := range []struct {
		path string
		body string
	}{
		{path: "/api/v2/containers/files/content", body: `{"containerID":"web","path":"relative.txt"}`},
		{path: "/api/v2/containers/files/size", body: `{"containerID":"web","path":"/tmp/../etc/passwd"}`},
		{path: "/api/v2/containers/files/download", body: `{"containerID":"web","path":""}`},
	} {
		res := httptest.NewRecorder()
		handleContainerRequest(service.DockerService{}, res, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body)))
		if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), `"code":"ERR"`) {
			t.Fatalf("%s status=%d body=%s", tc.path, res.Code, res.Body.String())
		}
	}
}
