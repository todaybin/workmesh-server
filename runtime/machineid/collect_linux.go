// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

//go:build linux

package machineid

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func collect() (Components, error) {
	components := Components{
		ProductUUID:   readFirst("/sys/class/dmi/id/product_uuid"),
		BoardSerial:   readFirst("/sys/class/dmi/id/board_serial"),
		ChassisSerial: readFirst("/sys/class/dmi/id/chassis_serial"),
	}
	components.CPUSerial, components.CPUIdentity = linuxCPUIdentity()
	components.PhysicalMACs = linuxPhysicalMACs("/sys/class/net")
	return components, nil
}

func readFirst(filename string) string {
	data, err := os.ReadFile(filename)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func linuxCPUIdentity() (string, string) {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "", ""
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if _, exists := values[key]; !exists {
			values[key] = strings.TrimSpace(value)
		}
	}
	serial := values["serial"]
	identity := strings.Join([]string{values["vendor_id"], values["cpu family"], values["model"], values["stepping"], values["model name"]}, "|")
	return serial, identity
}

func linuxPhysicalMACs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	macs := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if virtualInterfaceName(name) {
			continue
		}
		devicePath, err := filepath.EvalSymlinks(filepath.Join(root, name))
		if err != nil || strings.Contains(filepath.ToSlash(devicePath), "/devices/virtual/net/") {
			continue
		}
		mac := readFirst(filepath.Join(root, name, "address"))
		if mac != "" && mac != "00:00:00:00:00:00" {
			macs = append(macs, mac)
		}
	}
	sort.Strings(macs)
	return macs
}

func virtualInterfaceName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || name == "lo" {
		return true
	}
	for _, prefix := range []string{"docker", "veth", "br-", "virbr", "cni", "flannel", "tun", "tap", "wg", "zt", "tailscale"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
