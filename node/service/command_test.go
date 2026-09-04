// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/todaybin/workmesh-server/node/model"
)

func TestCommandServiceExecutesWithoutShellInterpolation(t *testing.T) {
	request := model.CommandRequest{Program: "go", Args: []string{"version"}}
	if runtime.GOOS == "windows" {
		request = model.CommandRequest{Program: "cmd", Args: []string{"/C", "echo workmesh"}}
	}
	result, err := (CommandService{}).Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("执行命令失败: %v", err)
	}
	if result.ExitCode != 0 || result.Stdout == "" {
		t.Fatalf("命令结果异常: %+v", result)
	}
}

func TestCommandServiceStreamsOutput(t *testing.T) {
	var chunks []string
	var mu sync.Mutex
	request := model.CommandRequest{Program: "sh", Args: []string{"-c", "printf stdout; printf stderr >&2"}, Output: func(stream string, chunk []byte) {
		mu.Lock()
		defer mu.Unlock()
		chunks = append(chunks, stream+":"+string(chunk))
	}}
	if runtime.GOOS == "windows" {
		request = model.CommandRequest{Program: "cmd", Args: []string{"/C", "<NUL set /p =stdout & <NUL set /p =stderr 1>&2"}, Output: request.Output}
	}
	if _, err := (CommandService{}).Execute(context.Background(), request); err != nil {
		t.Fatalf("流式命令执行失败: %v", err)
	}
	mu.Lock()
	joined := strings.Join(chunks, "|")
	mu.Unlock()
	if !strings.Contains(joined, "stdout:stdout") || !strings.Contains(joined, "stderr:stderr") {
		t.Fatalf("未收到标准输出和错误输出回调: %q", joined)
	}
}

func TestCommandServiceRejectsEmptyProgram(t *testing.T) {
	if _, err := (CommandService{}).Execute(context.Background(), model.CommandRequest{}); err == nil {
		t.Fatal("空命令应被拒绝")
	}
}
