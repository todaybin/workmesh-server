// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"net"
	"os"
	"runtime"
	"strings"
	"time"
)

// dashboardOSInfo 是首页系统信息使用的发行版字段。
type dashboardOSInfo struct {
	Pretty  string
	ID      string
	Family  string
	Version string
}

// dashboardKernelVersion 返回内核发行号，不返回带编译器信息的 /proc/version 全文。
func dashboardKernelVersion() string {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// dashboardOSRelease 读取 os-release，并去掉 PRETTY_NAME 末尾括号，和 1Panel 的展示一致。
func dashboardOSRelease() dashboardOSInfo {
	info := dashboardOSInfo{ID: runtime.GOOS, Family: runtime.GOOS}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		data, err = os.ReadFile("/usr/lib/os-release")
	}
	if err != nil {
		return info
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), "\"")
		switch key {
		case "PRETTY_NAME":
			info.Pretty = dashboardPrettyDistro(value)
		case "ID":
			if value != "" {
				info.ID = value
			}
		case "ID_LIKE":
			if fields := strings.Fields(value); len(fields) > 0 {
				info.Family = fields[0]
			}
		case "VERSION_ID":
			info.Version = value
		}
	}
	if info.Family == "" || info.Family == runtime.GOOS {
		info.Family = info.ID
	}
	return info
}

func dashboardPrettyDistro(value string) string {
	value = strings.TrimSpace(value)
	if start := strings.LastIndex(value, "("); start > 0 && strings.HasSuffix(value, ")") {
		value = strings.TrimSpace(value[:start])
	}
	return value
}

// dashboardIPv4 优先取 UDP 出口地址，避免把 docker0 等第一块网卡当成主机地址。
func dashboardIPv4() string {
	dialer := net.Dialer{Timeout: 300 * time.Millisecond}
	conn, err := dialer.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok && addr.IP != nil {
			if ip := addr.IP.To4(); ip != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	return dashboardFirstIPv4()
}

func dashboardFirstIPv4() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.To4().String()
			}
		}
	}
	return ""
}

// unescapeProcField 还原 /proc/mounts 的八进制转义，避免带空格的挂载点 statfs 失败。
func unescapeProcField(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+3 < len(value) && isOctal(value[i+1]) && isOctal(value[i+2]) && isOctal(value[i+3]) {
			builder.WriteByte((value[i+1]-'0')*64 + (value[i+2]-'0')*8 + (value[i+3] - '0'))
			i += 3
			continue
		}
		builder.WriteByte(value[i])
	}
	return builder.String()
}

func isOctal(value byte) bool {
	return value >= '0' && value <= '7'
}
