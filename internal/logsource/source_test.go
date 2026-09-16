// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package logsource

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileSourceUsesFallbackAndBoundsLines(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "access.log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := (FileSource{}).Read(context.Background(), Request{
		Paths:          []string{filepath.Join(root, "missing.log"), path},
		MaxLines:       2,
		FirstAvailable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 2 || result.Lines[0].Text != "one" || len(result.UsedPaths) != 1 || result.UsedPaths[0] != path {
		t.Fatalf("unexpected source result: %#v", result)
	}
}

func TestFileSourceHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (FileSource{}).Read(ctx, Request{Paths: []string{"/tmp/no-such-workmesh-log"}}); err == nil {
		t.Fatal("cancelled source read should return context error")
	}
}

func TestCommandSourceReadsBoundedLineOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command fixture requires POSIX")
	}
	result, err := (CommandSource{}).Read(context.Background(), CommandRequest{
		Program:  "/bin/sh",
		Args:     []string{"-c", "printf 'one\\ntwo\\nthree\\n'"},
		MaxBytes: 64,
		MaxLines: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 2 || result.Lines[0].Text != "one" || result.Lines[1].Text != "two" {
		t.Fatalf("unexpected command source result: %#v", result)
	}
}

func TestCommandSourceRejectsUnsafeArguments(t *testing.T) {
	if _, err := (CommandSource{}).Read(context.Background(), CommandRequest{
		Program: "/bin/echo",
		Args:    []string{"bad\narg"},
	}); err == nil {
		t.Fatal("unsafe command argument should be rejected")
	}
}
