// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDiskMutationDisabledByDefault(t *testing.T) {
	t.Setenv("WORKMESH_ALLOW_DISK_MUTATION", "")
	mux := http.NewServeMux()
	registerHostDiskOperationRoutes(mux)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/api/v2/hosts/disks/partition", bytes.NewBufferString(`{"device":"/dev/loop99","filesystem":"ext4","mountPoint":"/tmp/workmesh-test"}`)))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("partition should be disabled: status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestDiskMutationValidationProtectsSystemPaths(t *testing.T) {
	root := t.TempDir()
	fstab := filepath.Join(root, "fstab")
	if err := os.WriteFile(fstab, []byte("# test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_FSTAB_PATH", fstab)
	if err := validateDiskDevice("/dev/../sda"); err == nil {
		t.Fatal("path traversal device accepted")
	}
	if err := validateMountPoint("/"); err == nil {
		t.Fatal("root mount point accepted")
	}
	if err := updateFstabEntry("/dev/loop99", "/tmp/workmesh-test", "ext4", true); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(fstab)
	if err != nil || !bytes.Contains(content, []byte("nofail")) {
		t.Fatalf("fstab entry missing: err=%v content=%s", err, content)
	}
	if err := removeFstabEntry("/tmp/workmesh-test"); err != nil {
		t.Fatal(err)
	}
	content, _ = os.ReadFile(fstab)
	if bytes.Contains(content, []byte("workmesh-test")) {
		t.Fatalf("fstab entry not removed: %s", content)
	}
}
