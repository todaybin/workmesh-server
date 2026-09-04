// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
)

const maxCommandOutput = 1 << 20

// CommandService 执行受控的系统程序。调用者必须在 HTTP 层完成认证和审计。
type CommandService struct{}

// Execute 以独立参数执行程序，禁止通过 shell 字符串拼接；普通调用默认五分钟，构建类调用最多一小时。
func (CommandService) Execute(ctx context.Context, request model.CommandRequest) (model.CommandResult, error) {
	if strings.TrimSpace(request.Program) == "" {
		return model.CommandResult{}, errors.New("系统命令不能为空")
	}
	if strings.IndexByte(request.Program, 0) >= 0 {
		return model.CommandResult{}, errors.New("系统命令包含非法字符")
	}
	timeout := request.Timeout
	if timeout <= 0 || timeout > time.Hour {
		timeout = 5 * time.Minute
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(commandCtx, request.Program, request.Args...)
	command.Dir = request.Dir
	if len(request.Env) > 0 {
		command.Env = os.Environ()
		for key, value := range request.Env {
			if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "=\x00") {
				return model.CommandResult{}, fmt.Errorf("环境变量名无效: %q", key)
			}
			command.Env = append(command.Env, key+"="+value)
		}
	}
	var stdout, stderr limitedBuffer
	command.Stdout = io.MultiWriter(&stdout, commandOutputWriter{stream: "stdout", callback: request.Output})
	command.Stderr = io.MultiWriter(&stderr, commandOutputWriter{stream: "stderr", callback: request.Output})
	started := time.Now()
	err := command.Run()
	result := model.CommandResult{ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(started).Milliseconds()}
	if err != nil {
		result.ExitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
			return result, context.DeadlineExceeded
		}
		return result, err
	}
	return result, nil
}

type limitedBuffer struct{ bytes.Buffer }

type commandOutputWriter struct {
	stream   string
	callback func(string, []byte)
}

func (w commandOutputWriter) Write(p []byte) (int, error) {
	if w.callback != nil && len(p) > 0 {
		w.callback(w.stream, append([]byte(nil), p...))
	}
	return len(p), nil
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := maxCommandOutput - b.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return len(p), nil
}
