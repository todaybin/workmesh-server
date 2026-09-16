// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

func registerHostDiskOperationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/hosts/disks/partition", handleDiskPartition)
	mux.HandleFunc("POST /api/v2/hosts/disks/mount", handleDiskMount)
	mux.HandleFunc("POST /api/v2/hosts/disks/unmount", handleDiskUnmount)
}

type diskMutationRequest struct {
	Device     string `json:"device"`
	Filesystem string `json:"filesystem"`
	Label      string `json:"label"`
	AutoMount  bool   `json:"autoMount"`
	MountPoint string `json:"mountPoint"`
	NoFail     bool   `json:"noFail"`
}

func validateDiskDevice(device string) error {
	device = strings.TrimSpace(device)
	if !strings.HasPrefix(device, "/dev/") || strings.ContainsAny(device, "\x00\r\n") || strings.Contains(device, "..") {
		return errors.New("磁盘设备必须是安全的 /dev 路径")
	}
	if len(device) > 128 {
		return errors.New("磁盘设备路径过长")
	}
	return nil
}

func validateFilesystem(value string) error {
	if value != "ext4" && value != "xfs" {
		return errors.New("仅支持 ext4 或 xfs 文件系统")
	}
	return nil
}

func validateMountPoint(value string) error {
	value = filepath.Clean(strings.TrimSpace(value))
	if !filepath.IsAbs(value) || value == "/" || value == "/boot" || strings.ContainsAny(value, "\x00\r\n") {
		return errors.New("挂载点必须是非系统绝对路径")
	}
	return nil
}

func diskCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("%s 不可用: %w", name, err)
	}
	commandCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandCtx, path, args...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return output, fmt.Errorf("%s 失败: %s", name, message)
	}
	return output, nil
}

func handleDiskPartition(w http.ResponseWriter, r *http.Request) {
	var request diskMutationRequest
	if err := decodeJSON(r, &request); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if os.Getenv("WORKMESH_ALLOW_DISK_MUTATION") != "1" {
		wmhttp.JSON(w, http.StatusServiceUnavailable, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "DISK_MUTATION_DISABLED"}, "message": "分区操作默认关闭，请在隔离主机显式启用 WORKMESH_ALLOW_DISK_MUTATION=1"})
		return
	}
	if err := validateDiskDevice(request.Device); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if err := validateFilesystem(request.Filesystem); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if err := validateMountPoint(request.MountPoint); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if _, err := os.Stat(request.Device); err != nil {
		wmhttp.JSON(w, http.StatusNotFound, map[string]any{"code": "ERR", "message": "磁盘设备不存在"})
		return
	}
	if mounted, _ := diskIsMounted(r.Context(), request.Device); mounted {
		wmhttp.JSON(w, http.StatusConflict, map[string]any{"code": "ERR", "message": "磁盘设备已挂载，拒绝分区"})
		return
	}
	if _, err := diskCommand(r.Context(), "parted", "-s", request.Device, "mklabel", "gpt"); err != nil {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if _, err := diskCommand(r.Context(), "parted", "-s", request.Device, "mkpart", "primary", "1MiB", "100%"); err != nil {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	_, _ = diskCommand(r.Context(), "partprobe", request.Device)
	partition := request.Device + "1"
	if strings.HasSuffix(request.Device, "nvme0n1") || strings.HasSuffix(request.Device, "mmcblk0") || strings.HasSuffix(request.Device, "mmcblk1") {
		partition = request.Device + "p1"
	}
	if _, err := diskCommand(r.Context(), "mkfs."+request.Filesystem, "-F", partition); err != nil {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if request.Label != "" && strings.ContainsAny(request.Label, "\x00\r\n") {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": "磁盘标签无效"})
		return
	}
	if request.MountPoint != "" {
		if err := mountDisk(r.Context(), request.Device, partition, request.Filesystem, request.MountPoint, request.AutoMount, request.NoFail); err != nil {
			wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"device": partition, "mountPoint": request.MountPoint}})
}

func handleDiskMount(w http.ResponseWriter, r *http.Request) {
	var request diskMutationRequest
	if err := decodeJSON(r, &request); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if err := validateDiskDevice(request.Device); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if err := validateMountPoint(request.MountPoint); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if request.Filesystem != "" {
		if err := validateFilesystem(request.Filesystem); err != nil {
			wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
			return
		}
	}
	if err := mountDisk(r.Context(), request.Device, request.Device, request.Filesystem, request.MountPoint, request.AutoMount, request.NoFail); err != nil {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": nil})
}

func mountDisk(ctx context.Context, source, _ string, filesystem, mountPoint string, autoMount, noFail bool) error {
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("磁盘设备不存在: %w", err)
	}
	if err := os.MkdirAll(mountPoint, 0o755); err != nil {
		return fmt.Errorf("创建挂载点失败: %w", err)
	}
	if mounted, _ := diskIsMounted(ctx, mountPoint); mounted {
		return errors.New("挂载点已经被占用")
	}
	args := []string{}
	if filesystem != "" {
		args = append(args, "-t", filesystem)
	}
	args = append(args, source, mountPoint)
	if _, err := diskCommand(ctx, "mount", args...); err != nil {
		return err
	}
	if autoMount {
		if err := updateFstabEntry(source, mountPoint, filesystem, noFail); err != nil {
			_, _ = diskCommand(ctx, "umount", "-l", mountPoint)
			return err
		}
	}
	return nil
}

func handleDiskUnmount(w http.ResponseWriter, r *http.Request) {
	var request struct {
		MountPoint string `json:"mountPoint"`
	}
	if err := decodeJSON(r, &request); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if err := validateMountPoint(request.MountPoint); err != nil {
		wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if mounted, _ := diskIsMounted(r.Context(), request.MountPoint); !mounted {
		wmhttp.JSON(w, http.StatusConflict, map[string]any{"code": "ERR", "message": "挂载点当前未挂载"})
		return
	}
	if _, err := diskCommand(r.Context(), "umount", "-f", request.MountPoint); err != nil {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	if err := removeFstabEntry(request.MountPoint); err != nil {
		wmhttp.JSON(w, http.StatusBadGateway, map[string]any{"code": "ERR", "message": err.Error()})
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": nil})
}

func diskIsMounted(ctx context.Context, value string) (bool, error) {
	_, err := diskCommand(ctx, "findmnt", "-rn", "--target", value)
	return err == nil, err
}

func updateFstabEntry(source, mountPoint, filesystem string, noFail bool) error {
	path := "/etc/fstab"
	if configured := strings.TrimSpace(os.Getenv("WORKMESH_FSTAB_PATH")); configured != "" {
		path = configured
	}
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := filterFstabLines(string(old), mountPoint)
	options := "defaults"
	if noFail {
		options += ",nofail"
	}
	lines = append(lines, strings.Join([]string{source, mountPoint, filesystem, options, "0", "2"}, "\t"))
	return writeAtomicSSHFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func removeFstabEntry(mountPoint string) error {
	path := "/etc/fstab"
	if configured := strings.TrimSpace(os.Getenv("WORKMESH_FSTAB_PATH")); configured != "" {
		path = configured
	}
	old, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return writeAtomicSSHFile(path, []byte(strings.Join(filterFstabLines(string(old), mountPoint), "\n")+"\n"), 0o644)
}

func filterFstabLines(content, mountPoint string) []string {
	lines := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == mountPoint {
			continue
		}
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
