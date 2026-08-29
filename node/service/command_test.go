// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"runtime"
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

func TestCommandServiceRejectsEmptyProgram(t *testing.T) {
	if _, err := (CommandService{}).Execute(context.Background(), model.CommandRequest{}); err == nil {
		t.Fatal("空命令应被拒绝")
	}
}
