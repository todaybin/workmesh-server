// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"bytes"
	"encoding/base64"
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

	"github.com/todaybin/workmesh-server/internal/storage"
)

// TestExternalSSHPTY verifies a real sshd-backed terminal without touching the
// host service: all keys, config, HOME and SQLite state live below TempDir.
// TestExternalSSHPTY 验证临时 sshd、主机凭据持久化和远程 PTY 回显。
func TestExternalSSHPTY(t *testing.T) {
	if os.Getenv("WORKMESH_SSH_EXTERNAL_TEST") != "1" {
		t.Skip("set WORKMESH_SSH_EXTERNAL_TEST=1 to run isolated SSH PTY acceptance")
	}
	if _, err := exec.LookPath("sshd"); err != nil {
		t.Skip("sshd unavailable")
	}
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen unavailable")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(root, "data")
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	t.Setenv("WORKMESH_HOST_CREDENTIAL_KEY", "ssh-external-test-key")
	t.Setenv("HOME", home)
	store, err := storage.Open(filepath.Join(dataDir, "workmesh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		resetSharedStoreForTest()
		_ = store.Close()
	}()
	if err := SetSharedStore(store); err != nil {
		t.Fatal(err)
	}
	fixture := setupExternalSSHFixture(t, root, home)
	mux := http.NewServeMux()
	registerHostRoutes(mux)
	registerTerminalRoutes(mux)
	t.Setenv("WORKMESH_TERMINAL_TOKEN", "ssh-external-token")
	hostID := registerExternalSSHHost(t, mux, fixture.port, fixture.privateKey)
	readExternalSSHPTY(t, mux, hostID, fixture.output)
}

// registerExternalSSHHost 创建主机并确认元数据更新保留凭据。
func registerExternalSSHHost(t *testing.T, mux *http.ServeMux, port int, privateKey []byte) string {
	t.Helper()
	createReq := httptest.NewRequest(http.MethodPost, "/api/v2/hosts", bytes.NewBufferString(fmt.Sprintf(`{"name":"ssh-external","address":"127.0.0.1","port":%d,"user":"root","privateKey":%q}`, port, string(privateKey))))
	createReq.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	mux.ServeHTTP(created, createReq)
	if created.Code != http.StatusOK {
		t.Fatalf("create temporary SSH host status=%d body=%s", created.Code, created.Body.String())
	}
	var envelope struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil || envelope.Data.ID == "" {
		t.Fatalf("temporary SSH host response invalid: %s", created.Body.String())
	}
	loaded, err := hostWithCredentials(envelope.Data.ID)
	if err != nil || loaded.PrivateKey != string(privateKey) {
		t.Fatalf("stored SSH private key mismatch: err=%v equal=%v", err, loaded.PrivateKey == string(privateKey))
	}
	updateReq := httptest.NewRequest(http.MethodPost, "/api/v2/hosts/update", bytes.NewBufferString(fmt.Sprintf(`{"id":%q,"name":"ssh-external-renamed","address":"127.0.0.1","port":%d,"user":"root"}`, envelope.Data.ID, port)))
	updateReq.Header.Set("Content-Type", "application/json")
	updated := httptest.NewRecorder()
	mux.ServeHTTP(updated, updateReq)
	if updated.Code != http.StatusOK {
		t.Fatalf("metadata-only SSH host update status=%d body=%s", updated.Code, updated.Body.String())
	}
	loaded, err = hostWithCredentials(envelope.Data.ID)
	if err != nil || loaded.PrivateKey != string(privateKey) {
		t.Fatalf("metadata-only update lost SSH private key: err=%v equal=%v", err, loaded.PrivateKey == string(privateKey))
	}
	return envelope.Data.ID
}

// readExternalSSHPTY 消费真实 WebSocket 流，直到收到远端输出。
func readExternalSSHPTY(t *testing.T, mux *http.ServeMux, hostID string, sshdOutput *bytes.Buffer) {
	t.Helper()
	server := httptest.NewServer(mux)
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	path := "/api/v2/hosts/terminal/ssh?id=" + hostID + "&command=printf%20ssh-pty-ok"
	conn := dialTerminalTCP(t, host, path, websocketHeaders("ssh-external-token", server.URL))
	defer conn.Close()
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("SSH PTY handshake failed: status=%v err=%v", response, err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var frames []string
	for {
		opcode, payload, readErr := readServerWebSocketFrame(reader)
		if readErr != nil {
			t.Fatalf("SSH PTY stream ended: %v; frames=%v; sshd output: %s", readErr, frames, sshdOutput.String())
		}
		if opcode != 0x1 {
			continue
		}
		var output terminalServerMessage
		frames = append(frames, string(payload))
		if json.Unmarshal(payload, &output) == nil && output.Type == "cmd" {
			decoded, _ := base64.StdEncoding.DecodeString(output.Data)
			if strings.Contains(string(decoded), "ssh-pty-ok") {
				return
			}
		}
	}
}

type externalSSHFixture struct {
	port       int
	privateKey []byte
	keyBase    string
	output     *bytes.Buffer
}

// setupExternalSSHFixture 准备密钥、临时 sshd 和 known_hosts。
func setupExternalSSHFixture(t *testing.T, root, home string) externalSSHFixture {
	t.Helper()
	keyBase, privateKey, authorized := generateExternalSSHKeys(t, root)
	port, output := startExternalSSHDaemon(t, root, authorized)
	knownHosts, err := exec.Command("ssh-keyscan", "-p", fmt.Sprint(port), "127.0.0.1").Output()
	if err != nil {
		t.Fatalf("scan temporary ssh host key: %v", err)
	}
	knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
	if err := os.WriteFile(knownHostsPath, knownHosts, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_SSH_KNOWN_HOSTS", knownHostsPath)
	if outputBytes, err := exec.Command("ssh", "-tt", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "-o", "UserKnownHostsFile="+knownHostsPath, "-p", fmt.Sprint(port), "-i", keyBase, "root@127.0.0.1", "printf", "ssh-direct-ok").CombinedOutput(); err != nil {
		t.Fatalf("direct SSH probe failed: %v: %s", err, outputBytes)
	}
	return externalSSHFixture{port: port, privateKey: privateKey, keyBase: keyBase, output: output}
}

// generateExternalSSHKeys 创建临时用户密钥对和 authorized_keys 文件。
func generateExternalSSHKeys(t *testing.T, root string) (string, []byte, string) {
	t.Helper()
	keyBase := filepath.Join(root, "id_ed25519")
	if output, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", keyBase).CombinedOutput(); err != nil {
		t.Fatalf("generate SSH key: %v: %s", err, output)
	}
	privateKey, err := os.ReadFile(keyBase)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := os.ReadFile(keyBase + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	authorized := filepath.Join(root, "authorized_keys")
	if err := os.WriteFile(authorized, publicKey, 0o600); err != nil {
		t.Fatal(err)
	}
	return keyBase, privateKey, authorized
}

// startExternalSSHDaemon 在随机回环端口启动 sshd。
func startExternalSSHDaemon(t *testing.T, root, authorized string) (int, *bytes.Buffer) {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()
	hostKey := filepath.Join(root, "ssh_host_ed25519_key")
	if output, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", hostKey).CombinedOutput(); err != nil {
		t.Fatalf("generate host key: %v: %s", err, output)
	}
	config := filepath.Join(root, "sshd_config")
	configText := fmt.Sprintf("Port %d\nListenAddress 127.0.0.1\nHostKey %s\nAuthorizedKeysFile %s\nPidFile %s\nUsePAM no\nPasswordAuthentication no\nPubkeyAuthentication yes\nPermitRootLogin yes\nStrictModes no\nLogLevel ERROR\n", port, hostKey, authorized, filepath.Join(root, "sshd.pid"))
	if err := os.WriteFile(config, []byte(configText), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("sshd", "-t", "-f", config).CombinedOutput(); err != nil {
		t.Fatalf("validate temporary sshd config: %v: %s", err, output)
	}
	sshd := exec.Command("/usr/sbin/sshd", "-D", "-e", "-f", config)
	output := &bytes.Buffer{}
	sshd.Stdout, sshd.Stderr = output, output
	if err := sshd.Start(); err != nil {
		t.Fatalf("start temporary sshd: %v", err)
	}
	t.Cleanup(func() {
		if sshd.Process != nil {
			_ = sshd.Process.Kill()
			_ = sshd.Wait()
		}
	})
	if !waitTCP("127.0.0.1", port, 5*time.Second) {
		t.Fatalf("temporary sshd did not become ready: %s", output.String())
	}
	return port, output
}

// waitTCP 等待隔离 sshd 监听随机端口。
func waitTCP(host string, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
