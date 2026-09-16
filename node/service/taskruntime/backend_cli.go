// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package taskruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// CLITaskBackend 通过固定 CLI 管理沙盒，不经过 Shell。
type CLITaskBackend struct {
	command     string
	digest      string
	timeout     time.Duration
	outputLimit int
	run         func(context.Context, string, []string, int) ([]byte, error)
}

// NewCLITaskBackend 创建 CLI 后端，并校验可执行文件的固定摘要。
func NewCLITaskBackend(command, digest string, timeout time.Duration, outputLimit int) (*CLITaskBackend, error) {
	command = strings.TrimSpace(command)
	if command == "" || !isAbsolutePath(command) || strings.ContainsAny(command, "\x00 \t\r\n;&|`$<>") {
		return nil, errors.New("任务 CLI 必须是无参数绝对路径")
	}
	if digest == "" {
		return nil, errors.New("任务 CLI 必须配置 sha256 摘要")
	}
	if err := verifyFileDigest(command, digest); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	if outputLimit <= 0 {
		outputLimit = 8 << 20
	}
	if outputLimit > 64<<20 {
		return nil, errors.New("任务 CLI 输出上限不能超过 64 MiB")
	}
	return &CLITaskBackend{command: command, digest: digest, timeout: timeout, outputLimit: outputLimit, run: runCLI}, nil
}

// verifyFileDigest 校验任务 CLI 的 SHA-256，防止执行未授权的替换文件。
func verifyFileDigest(path, expected string) error {
	expected = strings.TrimSpace(strings.TrimPrefix(expected, "sha256:"))
	if len(expected) != sha256.Size*2 {
		return errors.New("任务 CLI 摘要必须是 64 位十六进制")
	}
	if _, err := hex.DecodeString(expected); err != nil {
		return errors.New("任务 CLI 摘要格式无效")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开任务 CLI 失败: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, 128<<20)); err != nil {
		return fmt.Errorf("读取任务 CLI 失败: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("任务 CLI 摘要不匹配: expected=%s actual=%s", expected, actual)
	}
	return nil
}

// execute 使用受控 CLI 完成一次任务操作，并限制上下文、输出和错误信息。
func (b *CLITaskBackend) execute(ctx context.Context, operation string, payload any, result any) error {
	switch operation {
	case "create", "start", "exec", "collect", "cancel", "destroy":
	default:
		return errors.New("任务 CLI 操作不在白名单中")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("编码任务 CLI 请求失败: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	callCtx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	out, err := b.run(callCtx, b.command, []string{"--json", "task", operation, string(body)}, b.outputLimit)
	if err != nil {
		return fmt.Errorf("任务 CLI %s 失败: %w", operation, err)
	}
	var envelope struct {
		OK      *bool           `json:"ok"`
		Error   string          `json:"error"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil || envelope.OK == nil {
		return errors.New("任务 CLI 返回了无效结果")
	}
	if !*envelope.OK {
		message := strings.TrimSpace(envelope.Error)
		if message == "" {
			message = strings.TrimSpace(envelope.Message)
		}
		if message == "" {
			message = "任务 CLI 操作失败"
		}
		return errors.New(sanitizeCLIMessage(message))
	}
	if result != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, result); err != nil {
			return errors.New("任务 CLI data 无效")
		}
	}
	return nil
}

// Create 将任务规格提交给外部沙箱 CLI，并返回其持久化任务 ID。
func (b *CLITaskBackend) Create(ctx context.Context, spec TaskSpec) (string, error) {
	var response struct {
		SandboxID string `json:"sandboxId"`
	}
	if err := b.execute(ctx, "create", spec, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.SandboxID) == "" {
		return "", errors.New("任务 CLI create 未返回 sandboxId")
	}
	return response.SandboxID, nil
}

// Start 请求外部沙箱启动指定任务。
func (b *CLITaskBackend) Start(ctx context.Context, id string) error {
	return b.execute(ctx, "start", map[string]string{"sandboxId": id}, nil)
}

// Exec 在已启动任务中执行白名单参数，并返回受限的标准输出摘要。
func (b *CLITaskBackend) Exec(ctx context.Context, id string, argv []string) (TaskExecResult, error) {
	if err := validateArgv(argv); err != nil {
		return TaskExecResult{}, err
	}
	var result TaskExecResult
	err := b.execute(ctx, "exec", map[string]any{"sandboxId": id, "argv": argv}, &result)
	return result, err
}

// Cancel 请求外部沙箱停止任务并释放其运行资源。
func (b *CLITaskBackend) Cancel(ctx context.Context, id string) error {
	return b.execute(ctx, "cancel", map[string]string{"sandboxId": id}, nil)
}

// Collect 读取任务结果并按输出上限截断，避免响应占满内存。
func (b *CLITaskBackend) Collect(ctx context.Context, id string) (TaskExecResult, error) {
	var result TaskExecResult
	err := b.execute(ctx, "collect", map[string]string{"sandboxId": id}, &result)
	return result, err
}

// Destroy 删除外部沙箱任务及其临时资源。
func (b *CLITaskBackend) Destroy(ctx context.Context, id string) error {
	return b.execute(ctx, "destroy", map[string]string{"sandboxId": id}, nil)
}

// runCLI 以参数数组启动 CLI，设置上下文取消和输出上限，禁止 shell 拼接。
func runCLI(ctx context.Context, command string, args []string, outputLimit int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = restrictedEnvironment(os.Environ())
	var stdout, stderr limitedOutput
	stdout.limit, stderr.limit = outputLimit, outputLimit
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("任务 CLI 操作超时")
		}
		if strings.TrimSpace(stderr.String()) != "" {
			return nil, errors.New(sanitizeCLIMessage(stderr.String()))
		}
		return nil, errors.New(sanitizeCLIMessage(err.Error()))
	}
	if stdout.exceeded || stderr.exceeded {
		return nil, errors.New("任务 CLI 输出超过限制")
	}
	return stdout.Bytes(), nil
}

type limitedOutput struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

// Write 累积不超过配置上限的 CLI 输出，并在超限时返回明确错误。
func (b *limitedOutput) Write(data []byte) (int, error) {
	if b.limit > 0 && b.Len()+len(data) > b.limit {
		b.exceeded = true
		return len(data), io.ErrShortBuffer
	}
	return b.Buffer.Write(data)
}

// restrictedEnvironment 仅保留 CLI 所需的安全环境变量，过滤凭据和内部配置。
func restrictedEnvironment(source []string) []string {
	allowed := map[string]bool{"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "LANG": true, "TZ": true, "TMP": true, "TEMP": true, "TMPDIR": true, "SystemRoot": true, "WINDIR": true}
	result := make([]string, 0, len(source))
	for _, entry := range source {
		key, _, ok := strings.Cut(entry, "=")
		if ok && (allowed[key] || strings.HasPrefix(key, "LC_")) {
			result = append(result, entry)
		}
	}
	return result
}

var sensitivePattern = regexp.MustCompile(`(?i)(authorization|token|secret|password|private[_ -]?key|api[_ -]?key)\s*[:=]\s*[^\s,;]+`)

// sanitizeCLIMessage 脱敏错误文本中的 Token、密码、私钥和 API Key。
func sanitizeCLIMessage(message string) string {
	message = sensitivePattern.ReplaceAllString(message, "$1=[redacted]")
	message = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || (r < 0x20 && r != ' ') {
			return ' '
		}
		return r
	}, message)
	message = strings.TrimSpace(message)
	if len(message) > 512 {
		message = message[:512]
	}
	if message == "" {
		return "任务 CLI 操作失败"
	}
	return message
}
