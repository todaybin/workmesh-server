// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDockerSystemDFUsesBytes(t *testing.T) {
	text := strings.Join([]string{
		"TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE",
		"Images          6         5         3.031GB   1.294GB (42%)",
		"Containers      5         5         847.9kB   0B (0%)",
		"Local Volumes   1         0         0B        0B",
		"Build Cache     5         5         85.58MB   0B",
	}, "\n")
	images, containers, volumes, buildCache := parseDockerSystemDF(text)
	if !nearByteSize(images, 1294000000) || containers != 0 || volumes != 0 || !nearByteSize(buildCache, 85580000) {
		t.Fatalf("docker df parse mismatch: images=%d containers=%d volumes=%d build=%d", images, containers, volumes, buildCache)
	}
}

func nearByteSize(got, want int64) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return diff < 1000
}

func TestToolboxScanUsesDataAndPanelLayout(t *testing.T) {
	dataDir := t.TempDir()
	panel := t.TempDir()
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	t.Setenv("WORKMESH_PANEL_DIR", panel)
	writeCleanFile(t, filepath.Join(dataDir, "uploads", "cache.tmp"), "upload-cache")
	writeCleanFile(t, filepath.Join(dataDir, "logs", "tasks", "App", "job.log"), "task-log")
	writeCleanFile(t, filepath.Join(dataDir, "backups", "legacy-demo", "backup.tar"), "backup")
	writeCleanFile(t, filepath.Join(dataDir, "websites", "demo", "logs", "access.log"), "site-log")
	writeCleanFile(t, filepath.Join(panel, "log", "task", "Website", "run.log"), "panel-task")
	writeCleanFile(t, filepath.Join(panel, "resource", "apps", "remote", "go", "1.26", "pkg.tgz"), "remote-cache")

	scanned := scanToolboxClean(t.Context())
	body := cleanScanText(scanned)
	for _, needle := range []string{"cache.tmp", "App", "legacy-demo", "demo", "Website", "go"} {
		if !strings.Contains(body, needle) {
			t.Fatalf("扫描结果缺少 %s: %s", needle, body)
		}
	}
	logs := scanned["systemLogClean"].([]cleanNode)
	var taskSize int64
	for _, item := range logs {
		if item.Label == "task_log" {
			taskSize = item.Size
		}
	}
	if taskSize <= 0 {
		t.Fatalf("任务日志大小应大于 0: %#v", logs)
	}
	backups := scanned["backupClean"].([]cleanNode)
	for _, item := range backups {
		if item.Label != "unknown_snapshot" {
			continue
		}
		if len(item.Children) == 0 || item.Children[0].CanDelete {
			t.Fatalf("已有备份只能展示、不能默认删除: %#v", item)
		}
	}
}

func writeCleanFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func cleanScanText(scanned map[string]any) string {
	var b strings.Builder
	for _, key := range []string{"systemClean", "backupClean", "uploadClean", "downloadClean", "systemLogClean", "containerClean"} {
		nodes, _ := scanned[key].([]cleanNode)
		appendCleanText(&b, nodes)
	}
	return b.String()
}

func appendCleanText(b *strings.Builder, nodes []cleanNode) {
	for _, node := range nodes {
		b.WriteString(node.Label)
		b.WriteByte('\n')
		b.WriteString(node.Name)
		b.WriteByte('\n')
		appendCleanText(b, node.Children)
	}
}
