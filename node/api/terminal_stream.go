// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type terminalClientMessage struct {
	Type      string `json:"type"`
	Data      string `json:"data"`
	Cols      int    `json:"cols,omitempty"`
	Rows      int    `json:"rows,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type terminalServerMessage struct {
	Type      string `json:"type"`
	Data      string `json:"data,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

const (
	// terminalIdleTimeout 限制终端连接长时间无输入时占用的资源。
	terminalIdleTimeout  = 30 * time.Minute
	maxTerminalDimension = 500
)

var terminalCommandCleanups sync.Map
var terminalCommandCredentials sync.Map

type terminalCommandCredential struct {
	payload []byte
	marker  []byte
}

// handleTerminalStream 建立本地、容器或 SSH 终端的双向 WebSocket 会话。
func handleTerminalStream(w http.ResponseWriter, r *http.Request) {
	if !requireStreamAuth(w, r, "WORKMESH_TERMINAL_TOKEN", "WORKMESH_STREAM_TOKEN") {
		return
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"websocket": true, "upgradeRequired": true}})
		return
	}
	cols, rows, err := terminalDimensions(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "TERMINAL_PARAMETERS_INVALID"}, "message": err.Error()})
		return
	}
	command, err := terminalCommand(r)
	if err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "TERMINAL_PARAMETERS_INVALID"}, "message": err.Error()})
		return
	}
	defer cleanupTerminalCommand(command)
	ws, err := upgradeStreamWebSocket(w, r)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() == "WEBSOCKET_ORIGIN_DENIED" {
			status = http.StatusForbidden
		} else if err.Error() == "WEBSOCKET_UNAVAILABLE" {
			status = http.StatusServiceUnavailable
		}
		wmhttp.JSON(w, status, map[string]any{"code": "ERR", "details": map[string]string{"errCode": err.Error()}, "message": err.Error()})
		return
	}
	defer ws.close()

	session, err := startTerminalSession(command, cols, rows)
	if err != nil {
		_ = writeTerminalError(ws, err)
		return
	}
	defer session.Close()
	if credential, ok := terminalCommandCredentials.LoadAndDelete(command); ok {
		if err := primeTerminalCredential(r.Context(), session, credential.(terminalCommandCredential)); err != nil {
			_ = writeTerminalError(ws, errors.New("数据库终端认证初始化失败"))
			return
		}
	}

	// 输出泵与输入循环共享完成信号；任一方向断开都会终止子进程并释放 PTY。
	done := make(chan struct{})
	var once sync.Once
	finish := func() {
		once.Do(func() {
			close(done)
			session.Close()
		})
	}
	defer finish()
	// 命令退出或输出通道断开时主动关闭网络连接，避免读循环长期阻塞而遗留 PTY。
	go func() {
		<-done
		_ = ws.close()
	}()
	var outputPumps sync.WaitGroup
	outputPumps.Add(1)
	go func() {
		defer outputPumps.Done()
		pumpTerminalOutput(ws, session.output, func() {})
	}()
	if session.errOutput != nil {
		outputPumps.Add(1)
		go func() {
			defer outputPumps.Done()
			pumpTerminalOutput(ws, session.errOutput, func() {})
		}()
	}
	go func() {
		_ = command.Wait()
		outputPumps.Wait()
		finish()
	}()

	for {
		opcode, payload, err := ws.readFrame()
		if err != nil {
			return
		}
		switch opcode {
		case 0x8:
			code := uint16(1000)
			if len(payload) >= 2 {
				code = binary.BigEndian.Uint16(payload[:2])
			}
			_ = ws.closeWithCode(code, "")
			return
		case 0x9:
			ws.writeMu.Lock()
			_ = writeStreamFrame(ws.conn, 0xA, payload)
			ws.writeMu.Unlock()
		case 0xA:
			continue
		case 0x1:
			if err := handleTerminalInputWithResize(ws, session.input, payload, session.resizeFn); err != nil {
				return
			}
		}
		select {
		case <-done:
			return
		default:
		}
	}
}

// terminalDimensions 解析并限制浏览器报告的终端列数和行数。
func terminalDimensions(r *http.Request) (int, int, error) {
	cols, rows := 80, 40
	if raw := strings.TrimSpace(r.URL.Query().Get("cols")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxTerminalDimension {
			return 0, 0, errors.New("cols 必须是 1 到 500 之间的整数")
		}
		cols = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("rows")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxTerminalDimension {
			return 0, 0, errors.New("rows 必须是 1 到 500 之间的整数")
		}
		rows = value
	}
	return cols, rows, nil
}

// terminalCommand 构造受限的本地、Docker 或 SSH 终端命令。
func terminalCommand(r *http.Request) (*exec.Cmd, error) {
	ctx := r.Context()
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case strings.HasSuffix(path, "/local"):
		shell := strings.TrimSpace(os.Getenv("SHELL"))
		if runtime.GOOS == "windows" {
			shell = strings.TrimSpace(os.Getenv("COMSPEC"))
		}
		if shell == "" {
			if runtime.GOOS == "windows" {
				shell = "cmd.exe"
			} else {
				shell = "/bin/sh"
			}
		}
		command := r.URL.Query().Get("command")
		if command == "" {
			return exec.CommandContext(ctx, shell), nil
		}
		if len(command) > 4096 || strings.IndexByte(command, 0) >= 0 {
			return nil, errors.New("command 参数无效")
		}
		if runtime.GOOS == "windows" {
			return exec.CommandContext(ctx, shell, "/C", command), nil
		}
		return exec.CommandContext(ctx, shell, "-c", command), nil
	case strings.HasSuffix(path, "/container"):
		if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("source")), "database") {
			return databaseTerminalCommand(ctx, r)
		}
		containerID := strings.TrimSpace(r.URL.Query().Get("containerid"))
		program := strings.TrimSpace(r.URL.Query().Get("command"))
		if !validDockerIdentifier(containerID) || !validTerminalProgram(program) {
			return nil, errors.New("containerid 或 command 参数无效")
		}
		// -i 与 -t 同时启用，使 Docker 会话接收控制序列和窗口尺寸。
		args := []string{"exec", "-i", "-t"}
		if user := strings.TrimSpace(r.URL.Query().Get("user")); user != "" {
			if !validDockerIdentifier(user) {
				return nil, errors.New("user 参数无效")
			}
			args = append(args, "-u", user)
		}
		args = append(args, containerID, program)
		return exec.CommandContext(ctx, service.DockerBinary(), args...), nil
	case strings.HasSuffix(path, "/ssh"):
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		var record hostRecord
		if id != "" {
			loaded, err := hostWithCredentials(id)
			if err != nil {
				return nil, fmt.Errorf("加载 SSH 主机失败: %w", err)
			}
			record = loaded
		} else {
			// 兼容旧客户端；新客户端必须使用 id，避免重复提交并信任连接参数。
			record = hostRecord{Address: strings.TrimSpace(r.URL.Query().Get("host")), User: strings.TrimSpace(r.URL.Query().Get("user")), Port: 22}
			if raw := r.URL.Query().Get("port"); raw != "" {
				value, err := strconv.Atoi(raw)
				if err != nil || value < 1 || value > 65535 {
					return nil, errors.New("port 参数无效")
				}
				record.Port = value
			}
		}
		host := strings.TrimSpace(record.Address)
		user := strings.TrimSpace(record.User)
		if host == "" || strings.ContainsAny(host, " \t\r\n\x00") || len(host) > 255 {
			return nil, errors.New("已保存主机的地址无效")
		}
		port := record.Port
		if port < 1 || port > 65535 {
			return nil, errors.New("已保存主机的 SSH 端口无效")
		}
		target := host
		if user != "" {
			if !validDockerIdentifier(user) {
				return nil, errors.New("user 参数无效")
			}
			target = user + "@" + host
		}
		// 认证目标完全来自 node_hosts，绝不从 URL 接收地址、用户名、端口或凭据。
		args := []string{"-tt", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-p", strconv.Itoa(port), target}
		// Deployments that keep host keys outside the process user's home can
		// provide an explicit file without putting it in the request URL.
		// The value is passed as an argv element, never through a shell.
		if knownHosts := strings.TrimSpace(os.Getenv("WORKMESH_SSH_KNOWN_HOSTS")); knownHosts != "" && !strings.ContainsAny(knownHosts, "\x00\r\n") {
			args = append(args[:len(args)-1], "-o", "UserKnownHostsFile="+knownHosts, args[len(args)-1])
		}
		if command := strings.TrimSpace(r.URL.Query().Get("command")); command != "" {
			if len(command) > 4096 || strings.IndexByte(command, 0) >= 0 {
				return nil, errors.New("command 参数无效")
			}
			args = append(args, command)
		}
		if record.PrivateKey != "" {
			if record.PassPhrase != "" {
				return nil, errors.New("带口令私钥需要先导入服务器 SSH Agent")
			}
			identity, cleanup, err := temporarySSHIdentity(record.PrivateKey)
			if err != nil {
				return nil, err
			}
			args = append([]string{"-i", identity, "-o", "IdentitiesOnly=yes"}, args...)
			cmd := exec.CommandContext(ctx, "ssh", args...)
			terminalCommandCleanups.Store(cmd, cleanup)
			return cmd, nil
		}
		if record.Password != "" {
			if _, err := exec.LookPath("sshpass"); err != nil {
				return nil, errors.New("密码认证需要服务器安装 sshpass，或改用私钥/SSH Agent")
			}
			cmd := exec.CommandContext(ctx, "sshpass", append([]string{"-e", "ssh"}, args...)...)
			cmd.Env = append(os.Environ(), "SSHPASS="+record.Password)
			return cmd, nil
		}
		return exec.CommandContext(ctx, "ssh", args...), nil
	default:
		return nil, errors.New("终端类型不受支持")
	}
}

// databaseTerminalCommand 按原版规则从数据库资源和应用安装记录解析容器及客户端命令。
func databaseTerminalCommand(ctx context.Context, r *http.Request) (*exec.Cmd, error) {
	databaseType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("databaseType")))
	databaseName := strings.TrimSpace(r.URL.Query().Get("database"))
	if databaseType == "" || databaseName == "" {
		return nil, errors.New("database 和 databaseType 参数不能为空")
	}
	aliases := map[string][]string{
		"mysql":              {"mysql", "mysql-cluster"},
		"mysql-cluster":      {"mysql-cluster", "mysql"},
		"mariadb":            {"mariadb"},
		"mongodb":            {"mongodb"},
		"postgres":           {"postgres", "postgresql", "postgresql-cluster"},
		"postgresql":         {"postgresql", "postgres", "postgresql-cluster"},
		"postgresql-cluster": {"postgresql-cluster", "postgresql", "postgres"},
		"redis":              {"redis", "redis-cluster"},
		"redis-cluster":      {"redis-cluster", "redis"},
	}
	types, supported := aliases[databaseType]
	if !supported {
		return nil, fmt.Errorf("不支持的数据库终端类型: %s", databaseType)
	}

	var connection service.Database
	foundConnection := false
	for _, typ := range types {
		if item, ok := databaseService.FindConnection(ctx, typ, databaseName); ok {
			connection, foundConnection = item, true
			break
		}
	}
	store := getAppStore()
	store.mu.RLock()
	var install appRecord
	if foundConnection && connection.AppInstallID > 0 {
		_, install = findApp(store.state.Apps, strconv.FormatInt(connection.AppInstallID, 10))
	}
	if install.ID == "" {
		for _, candidate := range store.state.Apps {
			matchesType := false
			for _, typ := range types {
				if strings.EqualFold(candidate.Key, typ) {
					matchesType = true
					break
				}
			}
			serviceName := appValue(candidate.Config, "serviceName", "SERVICE_NAME", "database")
			if matchesType && (strings.EqualFold(candidate.Name, databaseName) || strings.EqualFold(serviceName, databaseName)) {
				install = candidate
				break
			}
		}
	}
	store.mu.RUnlock()
	containerNames := appContainerNames(install)
	if len(containerNames) == 0 || !validDockerIdentifier(containerNames[0]) {
		return nil, fmt.Errorf("数据库 %s 没有关联可用容器", databaseName)
	}
	containerName := containerNames[0]
	username, password := connection.Username, connection.Password
	if username == "" {
		username = appValue(install.Config, "username", "user", "PANEL_DB_ROOT_USER")
	}
	if password == "" {
		password = appValue(install.Config, "password", "PANEL_DB_ROOT_PASSWORD")
	}
	if username == "" {
		username = "root"
	}

	args := []string{"exec", "-i", "-t", containerName}
	var script string
	switch databaseType {
	case "mysql", "mysql-cluster":
		script = terminalDatabasePasswordScript
		args = append(args, "sh", "-c", script, "--", "mysql", "-u"+username)
	case "mariadb":
		script = terminalDatabasePasswordScript
		args = append(args, "sh", "-c", script, "--", "mariadb", "-u"+username)
	case "mongodb":
		script = terminalMongoPasswordScript
		args = append(args, "sh", "-c", script, "--", "mongosh", "--username", username, "--password", "--authenticationDatabase", "admin")
	case "postgres", "postgresql", "postgresql-cluster":
		if username == "root" {
			username = "postgres"
		}
		script = terminalDatabasePasswordScript
		args = append(args, "sh", "-c", script, "--", "psql", "-t", "-U", username)
	case "redis", "redis-cluster":
		host := strings.TrimSpace(connection.Host)
		if host == "" {
			host = "127.0.0.1"
		}
		port := connection.Port
		if port <= 0 {
			port = 6379
		}
		if strings.ContainsAny(host, "\x00\r\n \t") || port > 65535 {
			return nil, errors.New("Redis 终端连接参数无效")
		}
		script = terminalDatabasePasswordScript
		args = append(args, "sh", "-c", script, "--", "redis-cli", "--raw", "-h", host, "-p", strconv.Itoa(port))
	}
	if script == "" {
		return nil, errors.New("数据库终端命令未配置")
	}
	command := exec.CommandContext(ctx, service.DockerBinary(), args...)
	command.Env = terminalDatabaseCommandEnvironment()
	payload, err := terminalCredentialPayload(password)
	if err != nil {
		return nil, err
	}
	terminalCommandCredentials.Store(command, terminalCommandCredential{
		payload: payload,
		marker:  []byte(terminalCredentialReadyMarker),
	})
	return command, nil
}

func terminalDatabaseCommandEnvironment() []string {
	blocked := map[string]struct{}{
		"MYSQL_PWD":     {},
		"PGPASSWORD":    {},
		"REDISCLI_AUTH": {},
	}
	result := make([]string, 0, len(os.Environ()))
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, blockedKey := blocked[key]; blockedKey {
				continue
			}
		}
		result = append(result, item)
	}
	return result
}

const terminalCredentialReadyMarker = "__WORKMESH_DATABASE_PASSWORD_READY__"

// terminalDatabasePasswordScript is fixed source. The executable and all
// user-controlled values are positional arguments, never shell source.
const terminalDatabasePasswordScript = `set -eu
stty -echo 2>/dev/null || :
printf '%s\n' '__WORKMESH_DATABASE_PASSWORD_READY__'
IFS= read -r _wm_pw_len
case "$_wm_pw_len" in
  ''|*[!0-9]*) exit 64 ;;
esac
_wm_pw=$(dd bs=1 count="$_wm_pw_len" 2>/dev/null; printf '\001')
_wm_pw=${_wm_pw%?}
IFS= read -r _wm_separator
stty echo 2>/dev/null || :
export MYSQL_PWD="$_wm_pw"
export PGPASSWORD="$_wm_pw"
export REDISCLI_AUTH="$_wm_pw"
exec "$@"
`

// terminalMongoPasswordScript keeps the password out of argv and feeds it to
// mongosh's password prompt after the fixed script has consumed the prefix.
const terminalMongoPasswordScript = `set -eu
stty -echo 2>/dev/null || :
printf '%s\n' '__WORKMESH_DATABASE_PASSWORD_READY__'
IFS= read -r _wm_pw_len
case "$_wm_pw_len" in
  ''|*[!0-9]*) exit 64 ;;
esac
_wm_pw=$(dd bs=1 count="$_wm_pw_len" 2>/dev/null; printf '\001')
_wm_pw=${_wm_pw%?}
IFS= read -r _wm_separator
stty echo 2>/dev/null || :
{ printf '%s\n' "$_wm_pw"; cat; } | exec "$@"
`

func terminalCredentialPayload(password string) ([]byte, error) {
	if strings.IndexByte(password, 0) >= 0 {
		return nil, errors.New("数据库密码包含不支持的空字节")
	}
	return []byte(strconv.Itoa(len(password)) + "\n" + password + "\n"), nil
}

func primeTerminalCredential(ctx context.Context, session *terminalSession, credential terminalCommandCredential) error {
	if session == nil || session.output == nil || session.input == nil {
		return errors.New("终端认证通道不可用")
	}
	readErr := make(chan error, 1)
	go func() {
		readErr <- readTerminalMarker(session.output, credential.marker)
	}()
	select {
	case err := <-readErr:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	if _, err := session.input.Write(credential.payload); err != nil {
		return errors.New("写入数据库终端认证信息失败")
	}
	return nil
}

func readTerminalMarker(reader io.Reader, marker []byte) error {
	if len(marker) == 0 {
		return errors.New("终端认证标记为空")
	}
	window := make([]byte, 0, len(marker))
	buf := []byte{0}
	for len(window) < 64*1024 {
		n, err := reader.Read(buf)
		if n > 0 {
			window = append(window, buf[:n]...)
			if len(window) > len(marker) {
				window = window[len(window)-len(marker):]
			}
			if bytes.Equal(window, marker) {
				return nil
			}
		}
		if err != nil {
			return err
		}
	}
	return errors.New("终端认证初始化响应过大")
}

// temporarySSHIdentity 将私钥安全写入数据目录下的临时文件。
func temporarySSHIdentity(privateKey string) (string, func(), error) {
	dataRoot := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dataRoot == "" {
		return "", nil, errors.New("未配置 WORKMESH_DATA_DIR，无法安全创建 SSH 临时密钥")
	}
	dir := filepath.Join(dataRoot, ".tmp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, fmt.Errorf("创建 SSH 临时目录失败: %w", err)
	}
	file, err := os.CreateTemp(dir, "ssh-identity-*")
	if err != nil {
		return "", nil, fmt.Errorf("创建 SSH 临时密钥失败: %w", err)
	}
	name := file.Name()
	if err := file.Chmod(0o600); err == nil {
		_, err = file.WriteString(privateKey)
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(name)
		if err == nil {
			err = closeErr
		}
		return "", nil, fmt.Errorf("写入 SSH 临时密钥失败: %w", err)
	}
	return name, func() { _ = os.Remove(name) }, nil
}

// cleanupTerminalCommand 删除终端命令关联的临时认证文件。
func cleanupTerminalCommand(command *exec.Cmd) {
	if cleanup, ok := terminalCommandCleanups.LoadAndDelete(command); ok {
		cleanup.(func())()
	}
	terminalCommandCredentials.Delete(command)
}

// validTerminalProgram 仅允许容器内单个可执行文件名，禁止注入 shell 参数。
func validTerminalProgram(program string) bool {
	if program == "" || len(program) > 128 || strings.ContainsAny(program, " \t\r\n\x00\\") || strings.Contains(program, "..") {
		return false
	}
	for _, ch := range program {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || strings.ContainsRune("._-/", ch)) {
			return false
		}
	}
	if strings.HasPrefix(program, "/") && !strings.HasPrefix(program, "/bin/") && !strings.HasPrefix(program, "/usr/bin/") && !strings.HasPrefix(program, "/usr/local/bin/") && !strings.HasPrefix(program, "/sbin/") && !strings.HasPrefix(program, "/usr/sbin/") {
		return false
	}
	return true
}

// handleTerminalInput 保留旧调用签名，供非 PTY 测试和内部调用使用。
func handleTerminalInput(ws *streamWebSocket, stdin io.Writer, payload []byte) error {
	return handleTerminalInputWithResize(ws, stdin, payload, nil)
}

// handleTerminalInputWithResize 处理终端输入、心跳和窗口调整消息。
func handleTerminalInputWithResize(ws *streamWebSocket, stdin io.Writer, payload []byte, resize func(int, int) error) error {
	var message terminalClientMessage
	if err := json.Unmarshal(payload, &message); err != nil {
		return writeTerminalError(ws, errors.New("终端消息不是有效 JSON"))
	}
	switch message.Type {
	case "cmd":
		decoded, err := base64.StdEncoding.DecodeString(message.Data)
		if err != nil || len(decoded) > 64<<10 {
			return writeTerminalError(ws, errors.New("终端输入无效或超过 64KiB"))
		}
		if stdin == nil {
			return writeTerminalError(ws, errors.New("终端输入通道不可用"))
		}
		_, err = stdin.Write(decoded)
		return err
	case "heartbeat":
		payload, _ := json.Marshal(terminalServerMessage{Type: "heartbeat", Timestamp: message.Timestamp})
		return ws.writeText(payload)
	case "resize":
		if message.Cols < 1 || message.Cols > maxTerminalDimension || message.Rows < 1 || message.Rows > maxTerminalDimension {
			return writeTerminalError(ws, errors.New("终端窗口尺寸必须在 1 到 500 之间"))
		}
		if resize == nil {
			return writeTerminalError(ws, errors.New("终端窗口调整不可用"))
		}
		if err := resize(message.Cols, message.Rows); err != nil {
			// 调整失败不终止会话，客户端可以继续使用当前尺寸。
			return writeTerminalError(ws, fmt.Errorf("调整终端窗口失败: %w", err))
		}
		return nil
	default:
		return writeTerminalError(ws, errors.New("终端消息类型不受支持"))
	}
}

// pumpTerminalOutput 将 PTY 或普通管道输出编码为前端约定的 base64 文本帧。
func pumpTerminalOutput(ws *streamWebSocket, reader io.Reader, finish func()) {
	defer finish()
	buffer := make([]byte, 32<<10)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			message, _ := json.Marshal(terminalServerMessage{Type: "cmd", Data: base64.StdEncoding.EncodeToString(buffer[:n])})
			if writeErr := ws.writeText(message); writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
