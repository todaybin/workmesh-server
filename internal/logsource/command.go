// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package logsource

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
)

// CommandRequest describes a bounded command-backed log read.
// Program and arguments are supplied by the caller as separate values; the
// source never invokes a shell.
type CommandRequest struct {
	Program  string
	Args     []string
	MaxBytes int64
	MaxLines int
}

// CommandSource reads line-oriented command output with bounded result
// storage. The child process is still drained after the result limit is
// reached so Wait cannot deadlock on a full stdout pipe.
type CommandSource struct{}

func (CommandSource) Read(ctx context.Context, request CommandRequest) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(request.Program) == "" {
		return Result{}, errors.New("日志 command 未提供程序")
	}
	for _, arg := range request.Args {
		if strings.ContainsAny(arg, "\x00\r\n") {
			return Result{}, errors.New("日志 command 参数无效")
		}
	}
	maxBytes := request.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	maxLines := request.MaxLines
	if maxLines <= 0 {
		maxLines = DefaultMaxLines
	}

	cmd := exec.CommandContext(ctx, request.Program, request.Args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}

	result := Result{Lines: make([]Line, 0), UsedPaths: make([]string, 0)}
	reader := io.LimitReader(stdout, maxBytes)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return Result{}, err
		}
		result.Lines = append(result.Lines, Line{Text: scanner.Text()})
		if len(result.Lines) >= maxLines {
			break
		}
	}
	scanErr := scanner.Err()
	_, _ = io.Copy(io.Discard, stdout)
	waitErr := cmd.Wait()
	if scanErr != nil {
		return Result{}, scanErr
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if waitErr != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = waitErr.Error()
		}
		return Result{}, errors.New(message)
	}
	return result, nil
}
