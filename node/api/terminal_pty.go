// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"io"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// terminalSession 表示一个终端子进程及其双向数据通道。
// Unix 系统优先使用真实 PTY；不支持 PTY 的平台保留管道模式并明确 resize 能力不可用。
type terminalSession struct {
	cmd       *exec.Cmd
	input     io.WriteCloser
	output    io.ReadCloser
	errOutput io.ReadCloser
	samePTY   bool
	resizeFn  func(int, int) error
	closeOnce sync.Once
}

// startTerminalSession 启动带窗口尺寸的终端会话。
func startTerminalSession(cmd *exec.Cmd, cols, rows int) (*terminalSession, error) {
	if cmd == nil {
		return nil, errors.New("终端命令不能为空")
	}
	if cols < 1 || cols > maxTerminalDimension || rows < 1 || rows > maxTerminalDimension {
		return nil, errors.New("终端窗口尺寸超出允许范围")
	}

	ptyFile, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err == nil {
		return &terminalSession{
			cmd:     cmd,
			input:   ptyFile,
			output:  ptyFile,
			samePTY: true,
			resizeFn: func(width, height int) error {
				if width < 1 || width > maxTerminalDimension || height < 1 || height > maxTerminalDimension {
					return errors.New("终端窗口尺寸无效")
				}
				return pty.Setsize(ptyFile, &pty.Winsize{Cols: uint16(width), Rows: uint16(height)})
			},
		}, nil
	}
	if !errors.Is(err, pty.ErrUnsupported) {
		return nil, errors.New("启动终端 PTY 失败: " + err.Error())
	}

	// Windows 等平台的 pty 实现可能只提供 ErrUnsupported。普通管道仍可用于命令执行，
	// 但不能伪造窗口调整成功，resizeFn 会返回明确错误。
	stdin, stdinErr := cmd.StdinPipe()
	if stdinErr != nil {
		return nil, errors.New("创建终端输入管道失败: " + stdinErr.Error())
	}
	stdout, stdoutErr := cmd.StdoutPipe()
	if stdoutErr != nil {
		_ = stdin.Close()
		return nil, errors.New("创建终端输出管道失败: " + stdoutErr.Error())
	}
	stderr, stderrErr := cmd.StderrPipe()
	if stderrErr != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, errors.New("创建终端错误管道失败: " + stderrErr.Error())
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, errors.New("启动终端进程失败: " + err.Error())
	}
	return &terminalSession{
		cmd:       cmd,
		input:     stdin,
		output:    stdout,
		errOutput: stderr,
		resizeFn: func(_, _ int) error {
			return errors.New("当前平台不支持 PTY 窗口调整")
		},
	}, nil
}

// Close 释放会话的所有文件描述符并终止仍在运行的子进程。
func (s *terminalSession) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		if s.input != nil {
			_ = s.input.Close()
		}
		if s.output != nil && !s.samePTY {
			_ = s.output.Close()
		}
		if s.errOutput != nil {
			_ = s.errOutput.Close()
		}
	})
}
