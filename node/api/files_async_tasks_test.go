// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"testing"
	"time"
)

func TestFileAsyncTaskStopCancelsContext(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	ctx, err := startFileAsyncTask("file-stop-test", "move", "开始文件移动")
	if err != nil {
		t.Fatal(err)
	}
	if err := stopFileAsyncTask("file-stop-test", "move"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("文件任务 context 未被取消")
	}
	finishFileAsyncTask("file-stop-test", "cancelled", "文件任务已取消")
	if err := stopFileAsyncTask("file-stop-test", "move"); err == nil {
		t.Fatal("重复取消已完成任务应返回错误")
	}
}
