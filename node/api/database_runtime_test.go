// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
)

type databaseRuntimeFakeResponse struct {
	result model.CommandResult
	err    error
}

type fakeDatabaseRuntimeCommand struct {
	requests  []model.CommandRequest
	responses map[string][]databaseRuntimeFakeResponse
	fallback  databaseRuntimeFakeResponse
}

func (f *fakeDatabaseRuntimeCommand) Execute(_ context.Context, request model.CommandRequest) (model.CommandResult, error) {
	f.requests = append(f.requests, request)
	key := strings.Join(request.Args, "\x00")
	if queue := f.responses[key]; len(queue) > 0 {
		response := queue[0]
		f.responses[key] = queue[1:]
		return response.result, response.err
	}
	return f.fallback.result, f.fallback.err
}

func TestDatabaseRuntimeUpAlwaysValidatesConfigBeforeUp(t *testing.T) {
	dataDir, composePath := prepareDatabaseRuntimeCompose(t, "docker-compose.yml")
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	fake := newDatabaseRuntimeFake(composePath)
	restore := replaceDatabaseRuntimeCommand(fake)
	defer restore()

	response := serveDatabaseRuntime(t, http.MethodPost, "/api/v2/databases/runtime/up", `{"path":"docker-compose.yml"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(fake.requests) != 2 {
		t.Fatalf("requests=%d want 2: %#v", len(fake.requests), fake.requests)
	}
	assertDatabaseRuntimeRequest(t, fake.requests[0], []string{"compose", "-f", composePath, "config", "--format", "json"}, filepath.Dir(composePath))
	assertDatabaseRuntimeRequest(t, fake.requests[1], []string{"compose", "-f", composePath, "up", "-d"}, filepath.Dir(composePath))
	if !strings.Contains(response.Body.String(), `"configValidated":true`) {
		t.Fatalf("config validation not reported: %s", response.Body.String())
	}
}

func TestDatabaseRuntimeUpStopsWhenConfigValidationFails(t *testing.T) {
	dataDir, composePath := prepareDatabaseRuntimeCompose(t, "docker-compose.yml")
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	fake := newDatabaseRuntimeFake(composePath)
	configKey := strings.Join([]string{"compose", "-f", composePath, "config", "--format", "json"}, "\x00")
	fake.responses[configKey] = []databaseRuntimeFakeResponse{{
		result: model.CommandResult{ExitCode: 0, Stdout: `{"services":{"postgres":{"container_name":"other"}}}`},
	}}
	restore := replaceDatabaseRuntimeCommand(fake)
	defer restore()

	response := serveDatabaseRuntime(t, http.MethodPost, "/api/v2/databases/runtime/up", nil)
	if response.Code != http.StatusServiceUnavailable || len(fake.requests) != 1 {
		t.Fatalf("status=%d requests=%d body=%s", response.Code, len(fake.requests), response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"up"`) {
		t.Fatalf("up should not execute after config failure: %s", response.Body.String())
	}
}

func TestDatabaseRuntimeStatusReturnsStructuredPSAndInspectState(t *testing.T) {
	dataDir, composePath := prepareDatabaseRuntimeCompose(t, "docker-compose.yml")
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	fake := newDatabaseRuntimeFake(composePath)
	psKey := strings.Join([]string{"compose", "-f", composePath, "ps", "--all", "--format", "json"}, "\x00")
	inspectArgs := append([]string{"inspect", "--type", "container"}, databaseRuntimeContainerNames...)
	inspectKey := strings.Join(inspectArgs, "\x00")
	fake.responses[psKey] = []databaseRuntimeFakeResponse{{
		result: model.CommandResult{ExitCode: 0, Stdout: `[
			{"ID":"pg-id","Name":"workmesh-panel-postgres","State":"running","Health":"healthy"},
			{"ID":"redis-id","Name":"workmesh-panel-redis","State":"exited","Health":"unhealthy","Publishers":[{"URL":"127.0.0.1","TargetPort":6379,"PublishedPort":16379,"Protocol":"tcp"}]}
		]`},
	}}
	fake.responses[inspectKey] = []databaseRuntimeFakeResponse{{
		result: model.CommandResult{ExitCode: 0, Stdout: `[
			{"Id":"pg-id","Name":"/workmesh-panel-postgres","Config":{"Image":"postgres:18-alpine"},"State":{"Status":"running","Health":{"Status":"healthy"}},"NetworkSettings":{"Ports":{"5432/tcp":[]}}},
			{"Id":"redis-id","Name":"/workmesh-panel-redis","Config":{"Image":"redis:8-alpine"},"State":{"Status":"exited","ExitCode":17,"Error":"crashed","Health":{"Status":"unhealthy"}},"NetworkSettings":{"Ports":{"6379/tcp":[{"HostIp":"127.0.0.1","HostPort":"16379"}]}}}
		]`},
	}}
	restore := replaceDatabaseRuntimeCommand(fake)
	defer restore()

	response := serveDatabaseRuntime(t, http.MethodGet, "/api/v2/databases/runtime/status", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Operation          string                             `json:"operation"`
			Path               string                             `json:"path"`
			ConfigValidated    bool                               `json:"configValidated"`
			ExpectedContainers []string                           `json:"expectedContainers"`
			Containers         []service.DatabaseRuntimeContainer `json:"containers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 200 || envelope.Data.Operation != "status" || envelope.Data.Path != composePath || !envelope.Data.ConfigValidated {
		t.Fatalf("unexpected envelope: %s", response.Body.String())
	}
	if !reflect.DeepEqual(envelope.Data.ExpectedContainers, databaseRuntimeContainerNames) {
		t.Fatalf("expected containers=%v", envelope.Data.ExpectedContainers)
	}
	if len(envelope.Data.Containers) != 3 {
		t.Fatalf("containers=%#v", envelope.Data.Containers)
	}
	if envelope.Data.Containers[0].Image != "postgres:18-alpine" || envelope.Data.Containers[1].Error != "crashed" {
		t.Fatalf("structured state not merged: %#v", envelope.Data.Containers)
	}
	if envelope.Data.Containers[2].Status != "missing" {
		t.Fatalf("missing state=%#v", envelope.Data.Containers[2])
	}
	if len(fake.requests) != 3 {
		t.Fatalf("status should run config, ps and inspect: %#v", fake.requests)
	}
}

func TestDatabaseRuntimeStatusUsesInspectWhenPSJSONIsInvalid(t *testing.T) {
	dataDir, composePath := prepareDatabaseRuntimeCompose(t, "docker-compose.yml")
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	fake := newDatabaseRuntimeFake(composePath)
	psKey := strings.Join([]string{"compose", "-f", composePath, "ps", "--all", "--format", "json"}, "\x00")
	inspectKey := strings.Join(append([]string{"inspect", "--type", "container"}, databaseRuntimeContainerNames...), "\x00")
	fake.responses[psKey] = []databaseRuntimeFakeResponse{{result: model.CommandResult{ExitCode: 0, Stdout: "not-json"}}}
	fake.responses[inspectKey] = []databaseRuntimeFakeResponse{{result: model.CommandResult{ExitCode: 0, Stdout: `[
			{"Id":"pg","Name":"/workmesh-panel-postgres","Config":{"Image":"postgres:18-alpine"},"State":{"Status":"running"}}
		]`}}}
	restore := replaceDatabaseRuntimeCommand(fake)
	defer restore()

	response := serveDatabaseRuntime(t, http.MethodGet, "/api/v2/databases/runtime/status", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"postgres:18-alpine"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDatabaseRuntimeOperationsValidateConfigAndRejectDestructiveArguments(t *testing.T) {
	dataDir, composePath := prepareDatabaseRuntimeCompose(t, "custom.yml")
	t.Setenv("WORKMESH_DATA_DIR", dataDir)

	for _, operation := range []string{"config", "up", "stop", "restart"} {
		t.Run(operation, func(t *testing.T) {
			fake := newDatabaseRuntimeFake(composePath)
			restore := replaceDatabaseRuntimeCommand(fake)
			defer restore()
			fake.requests = nil
			response := serveDatabaseRuntime(t, http.MethodPost, "/api/v2/databases/runtime/"+operation, `{"path":"custom.yml"}`)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if len(fake.requests) != 1 && operation == "config" {
				t.Fatalf("config requests=%d", len(fake.requests))
			}
			if len(fake.requests) != 2 && operation != "config" {
				t.Fatalf("%s requests=%d: %#v", operation, len(fake.requests), fake.requests)
			}
			for _, request := range fake.requests {
				args := strings.Join(request.Args, " ")
				if strings.Contains(args, " down") || strings.Contains(args, "--volumes") {
					t.Fatalf("destructive argument found: %#v", request.Args)
				}
			}
		})
	}
}

func TestDatabaseRuntimeReturnsErrorsForConfigUpStatusStopAndRestart(t *testing.T) {
	dataDir, composePath := prepareDatabaseRuntimeCompose(t, "docker-compose.yml")
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	for _, testCase := range []struct {
		name      string
		operation string
		failAt    string
	}{
		{name: "config", operation: "config", failAt: "config"},
		{name: "up", operation: "up", failAt: "up"},
		{name: "status", operation: "status", failAt: "ps"},
		{name: "stop", operation: "stop", failAt: "stop"},
		{name: "restart", operation: "restart", failAt: "restart"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fake := newDatabaseRuntimeFake(composePath)
			failKey := strings.Join(runtimeCommandArgs(composePath, testCase.failAt), "\x00")
			fake.responses[failKey] = []databaseRuntimeFakeResponse{{
				result: model.CommandResult{ExitCode: 23, Stderr: testCase.name + " failed"},
			}}
			restore := replaceDatabaseRuntimeCommand(fake)
			defer restore()
			response := serveDatabaseRuntime(t, http.MethodPost, "/api/v2/databases/runtime/"+testCase.operation, nil)
			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"ERR"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestDatabaseRuntimeRejectsUnsupportedOperationsAndUnsafePaths(t *testing.T) {
	dataDir, _ := prepareDatabaseRuntimeCompose(t, "docker-compose.yml")
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	fake := &fakeDatabaseRuntimeCommand{}
	restore := replaceDatabaseRuntimeCommand(fake)
	defer restore()

	tests := []struct {
		name string
		path string
		url  string
	}{
		{name: "down operation", url: "/api/v2/databases/runtime/down"},
		{name: "ps operation", url: "/api/v2/databases/runtime/ps"},
		{name: "path traversal", path: "../outside.yml", url: "/api/v2/databases/runtime/config"},
		{name: "absolute outside path", path: filepath.Join(t.TempDir(), "outside.yml"), url: "/api/v2/databases/runtime/config"},
		{name: "missing compose file", path: "missing.yml", url: "/api/v2/databases/runtime/config"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var body *bytes.Reader
			if testCase.path == "" {
				body = bytes.NewReader(nil)
			} else {
				payload, err := json.Marshal(map[string]string{"path": testCase.path})
				if err != nil {
					t.Fatal(err)
				}
				body = bytes.NewReader(payload)
			}
			response := serveDatabaseRuntimeReader(t, http.MethodPost, testCase.url, body)
			if response.Code != http.StatusBadRequest && response.Code != http.StatusServiceUnavailable {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if len(fake.requests) != 0 {
				t.Fatalf("unsafe request reached Docker: %#v", fake.requests)
			}
		})
	}
}

func TestDatabaseRuntimeRejectsComposeSymlinkOutsideDatabaseRoot(t *testing.T) {
	dataDir := t.TempDir()
	databaseDir := filepath.Join(dataDir, "database")
	if err := os.MkdirAll(databaseDir, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.yml")
	if err := os.WriteFile(outside, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(databaseDir, "docker-compose.yml")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	fake := &fakeDatabaseRuntimeCommand{}
	restore := replaceDatabaseRuntimeCommand(fake)
	defer restore()

	response := serveDatabaseRuntime(t, http.MethodGet, "/api/v2/databases/runtime/status", nil)
	if response.Code != http.StatusServiceUnavailable || len(fake.requests) != 0 {
		t.Fatalf("status=%d body=%s calls=%d", response.Code, response.Body.String(), len(fake.requests))
	}
}

func newDatabaseRuntimeFake(composePath string) *fakeDatabaseRuntimeCommand {
	configKey := strings.Join([]string{"compose", "-f", composePath, "config", "--format", "json"}, "\x00")
	return &fakeDatabaseRuntimeCommand{
		responses: map[string][]databaseRuntimeFakeResponse{
			configKey: {{
				result: model.CommandResult{ExitCode: 0, Stdout: validDatabaseRuntimeComposeConfig},
			}},
		},
	}
}

const validDatabaseRuntimeComposeConfig = `{"services":{"postgres":{"container_name":"workmesh-panel-postgres"},"redis":{"container_name":"workmesh-panel-redis"},"mariadb":{"container_name":"workmesh-panel-mariadb"}}}`

func runtimeCommandArgs(composePath, operation string) []string {
	switch operation {
	case "config":
		return []string{"compose", "-f", composePath, "config", "--format", "json"}
	case "ps":
		return []string{"compose", "-f", composePath, "ps", "--all", "--format", "json"}
	case "up":
		return []string{"compose", "-f", composePath, "up", "-d"}
	case "stop", "restart":
		return []string{"compose", "-f", composePath, operation}
	default:
		return nil
	}
}

func serveDatabaseRuntime(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	switch value := body.(type) {
	case nil:
		return serveDatabaseRuntimeReader(t, method, path, bytes.NewReader(nil))
	case string:
		return serveDatabaseRuntimeReader(t, method, path, bytes.NewReader([]byte(value)))
	case *bytes.Reader:
		return serveDatabaseRuntimeReader(t, method, path, value)
	default:
		t.Fatalf("unsupported database runtime request body type %T", body)
		return nil
	}
}

func serveDatabaseRuntimeReader(t *testing.T, method, path string, body *bytes.Reader) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	registerDatabaseRuntimeRoutes(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(method, path, body))
	return response
}

func prepareDatabaseRuntimeCompose(t *testing.T, name string) (string, string) {
	t.Helper()
	dataDir := t.TempDir()
	databaseDir := filepath.Join(dataDir, "database")
	if err := os.MkdirAll(databaseDir, 0o750); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(databaseDir, name)
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dataDir, composePath
}

func replaceDatabaseRuntimeCommand(fake databaseRuntimeCommandExecutor) func() {
	previous := databaseRuntimeCommands
	databaseRuntimeCommands = fake
	return func() { databaseRuntimeCommands = previous }
}

func assertDatabaseRuntimeRequest(t *testing.T, request model.CommandRequest, wantArgs []string, wantDir string) {
	t.Helper()
	if request.Program != service.DockerBinary() {
		t.Fatalf("program=%q want=%q", request.Program, service.DockerBinary())
	}
	if !reflect.DeepEqual(request.Args, wantArgs) {
		t.Fatalf("args=%#v want=%#v", request.Args, wantArgs)
	}
	if request.Dir != wantDir {
		t.Fatalf("dir=%q want=%q", request.Dir, wantDir)
	}
}
