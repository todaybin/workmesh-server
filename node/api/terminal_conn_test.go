// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/todaybin/workmesh-server/internal/storage"
)

func TestTerminalSearchReturnsFrontendFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dir)
	store := openTerminalTestStore(t, dir)
	defer store.Close()
	mux := http.NewServeMux()
	registerBackupAlertLogSettingsRoutes(mux)

	search := httptest.NewRecorder()
	mux.ServeHTTP(search, httptest.NewRequest(http.MethodPost, "/api/v2/core/settings/terminal/search", strings.NewReader(`{}`)))
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), `"fontSize":"12"`) || strings.Contains(search.Body.String(), `"config"`) {
		t.Fatalf("default search=%d %s", search.Code, search.Body.String())
	}
	update := httptest.NewRecorder()
	mux.ServeHTTP(update, httptest.NewRequest(http.MethodPost, "/api/v2/core/settings/terminal/update", strings.NewReader(`{"fontSize":"16","cursorBlink":"Enable","lineHeight":"1.4"}`)))
	if update.Code != http.StatusOK {
		t.Fatalf("update=%d %s", update.Code, update.Body.String())
	}
	again := httptest.NewRecorder()
	mux.ServeHTTP(again, httptest.NewRequest(http.MethodPost, "/api/v2/core/settings/terminal/search", strings.NewReader(`{}`)))
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), `"fontSize":"16"`) || !strings.Contains(again.Body.String(), `"fontFamily"`) {
		t.Fatalf("saved search=%d %s", again.Code, again.Body.String())
	}
}

func TestSSHCheckAndDefaultConn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dir)
	functionalStoreInstance = nil
	t.Cleanup(func() { functionalStoreInstance = nil })
	store := openTerminalTestStore(t, dir)
	defer store.Close()
	mux := http.NewServeMux()
	RegisterRuntimeToolboxRoutes(mux)
	registerBackupAlertLogSettingsRoutes(mux)

	check := httptest.NewRecorder()
	mux.ServeHTTP(check, httptest.NewRequest(http.MethodPost, "/api/v2/settings/ssh/check", strings.NewReader(`{}`)))
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"data":false`) {
		t.Fatalf("check=%d %s", check.Code, check.Body.String())
	}
	def := httptest.NewRecorder()
	mux.ServeHTTP(def, httptest.NewRequest(http.MethodPost, "/api/v2/settings/ssh/default", strings.NewReader(`{"defaultConn":"Enable","withReset":false}`)))
	if def.Code != http.StatusOK {
		t.Fatalf("default=%d %s", def.Code, def.Body.String())
	}
	conn := httptest.NewRecorder()
	mux.ServeHTTP(conn, httptest.NewRequest(http.MethodGet, "/api/v2/settings/ssh/conn", nil))
	if conn.Code != http.StatusOK || !strings.Contains(conn.Body.String(), `"localSSHConnShow":"Enable"`) {
		t.Fatalf("conn=%d %s", conn.Code, conn.Body.String())
	}
	settings := httptest.NewRecorder()
	mux.ServeHTTP(settings, httptest.NewRequest(http.MethodPost, "/api/v2/settings/search", strings.NewReader(`{}`)))
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), `"localSSHConnShow":"Enable"`) {
		t.Fatalf("settings=%d %s", settings.Code, settings.Body.String())
	}
}

func TestHostUpdateKeepsCipherWithoutNewSecret(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dir)
	t.Setenv("WORKMESH_HOST_CREDENTIAL_KEY", "preserve-test-key")
	store := openTerminalTestStore(t, dir)
	defer store.Close()
	mux := http.NewServeMux()
	registerHostRoutes(mux)

	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/hosts", strings.NewReader(`{"name":"edge","address":"127.0.0.1","port":22,"user":"root","password":"secret"}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create=%d %s", create.Code, create.Body.String())
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil || created.Data.ID == "" {
		t.Fatalf("decode create: %v %s", err, create.Body.String())
	}
	before, err := storedHostCredentialCipher(created.Data.ID)
	if err != nil || before == "" {
		t.Fatalf("cipher before update: %v %q", err, before)
	}
	t.Setenv("WORKMESH_HOST_CREDENTIAL_KEY", "")
	update := httptest.NewRecorder()
	body := `{"id":"` + created.Data.ID + `","name":"edge-renamed","address":"127.0.0.1","port":22,"user":"root","password":""}`
	mux.ServeHTTP(update, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/update", strings.NewReader(body)))
	if update.Code != http.StatusOK {
		t.Fatalf("update=%d %s", update.Code, update.Body.String())
	}
	after, err := storedHostCredentialCipher(created.Data.ID)
	if err != nil || after != before {
		t.Fatalf("cipher changed: before=%q after=%q err=%v", before, after, err)
	}
	t.Setenv("WORKMESH_HOST_CREDENTIAL_KEY", "preserve-test-key")
	item, err := hostWithCredentials(created.Data.ID)
	if err != nil || item.Password != "secret" || item.Name != "edge-renamed" {
		t.Fatalf("credentials=%+v err=%v", item, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets", "host-credential.key")); err == nil {
		t.Fatal("已设置环境变量时不应创建密钥文件")
	}
}

func TestHostCredentialKeyFileCreatedWhenEnvMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dir)
	t.Setenv("WORKMESH_HOST_CREDENTIAL_KEY", "")
	store := openTerminalTestStore(t, dir)
	defer store.Close()
	mux := http.NewServeMux()
	registerHostRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/hosts", strings.NewReader(`{"name":"auto","address":"10.0.0.8","port":22,"user":"root","password":"secret"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("create=%d %s", res.Code, res.Body.String())
	}
	info, err := os.Stat(filepath.Join(dir, "secrets", "host-credential.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode=%o", info.Mode().Perm())
	}
}

func TestLocalTerminalUsesLoginShellAndWelcome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v2/hosts/terminal/local", nil)
	command, err := terminalCommand(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(command.Args) < 2 || command.Args[len(command.Args)-1] != "-il" {
		t.Fatalf("local shell = %#v", command.Args)
	}
	welcome := localTerminalWelcome()
	if !strings.Contains(welcome, "Welcome to WorkMesh") || !strings.Contains(welcome, "Host:") {
		t.Fatalf("welcome = %q", welcome)
	}
}

func TestTerminalAIEnterPastesCommandAndBlocksRisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dir)
	store := openTerminalTestStore(t, dir)
	defer store.Close()
	if err := saveNodeSetting(terminalAISettingKey, map[string]any{
		"aiStatus": "Enable", "aiAccountId": "account-1", "aiPrefix": "@ai", "aiRiskCommands": `["reboot"]`,
	}); err != nil {
		t.Fatal(err)
	}
	original := terminalAIComplete
	terminalAIComplete = func(prompt string) (string, error) {
		if strings.Contains(prompt, "危险") {
			return "reboot", nil
		}
		return "ls -la", nil
	}
	t.Cleanup(func() { terminalAIComplete = original })

	handled, input, err := handleTerminalAIEnter(nil, "root@host:~# @ai 列出文件")
	if err != nil || !handled || string(input) != string([]byte{terminalAILineClear})+"ls -la" {
		t.Fatalf("paste handled=%v input=%q err=%v", handled, input, err)
	}
	handled, input, err = handleTerminalAIEnter(nil, "@ai 危险")
	if err != nil || !handled || !strings.Contains(string(input), "已拦截风险命令") {
		t.Fatalf("risk handled=%v input=%q err=%v", handled, input, err)
	}
}

func openTerminalTestStore(t *testing.T, dir string) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(dir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := SetSharedStore(store); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	t.Cleanup(resetSharedStoreForTest)
	return store
}
