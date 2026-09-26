// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var deviceZonePattern = regexp.MustCompile(`^[A-Za-z0-9+_]+(?:/[A-Za-z0-9+_.:-]+)+$`)
var deviceHostPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)

func deviceBaseInfo(ctx context.Context) map[string]any {
	host, _ := os.Hostname()
	current, _ := user.Current()
	name := ""
	if current != nil {
		name = current.Username
	}
	total, free, used := readSwapBytes()
	return map[string]any{
		"hostname": host, "user": name, "dns": readNameServers(), "hosts": readHostsEntries(),
		"ntp": readNTPServer(), "timeZone": readTimeZone(), "localTime": time.Now().Format("2006-01-02 15:04:05"),
		"swapMemoryTotal": total, "swapMemoryAvailable": free, "swapMemoryUsed": used,
		"maxSize": readRootAvailable(), "swapDetails": readSwapDetails(),
	}
}

func listHostUsers() []string {
	content, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return []string{}
	}
	users := make([]string, 0, 32)
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 7 || fields[0] == "" {
			continue
		}
		shell := fields[6]
		if strings.Contains(shell, "nologin") || strings.Contains(shell, "false") {
			continue
		}
		users = append(users, fields[0])
		if len(users) >= 200 {
			break
		}
	}
	return users
}

func listTimeZones() []string {
	file, err := os.Open("/usr/share/zoneinfo/zone.tab")
	if err != nil {
		zone := readTimeZone()
		if zone == "" {
			zone = "UTC"
		}
		return []string{zone}
	}
	defer file.Close()
	zones := make([]string, 0, 400)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || !deviceZonePattern.MatchString(fields[2]) {
			continue
		}
		zones = append(zones, fields[2])
		if len(zones) >= 2000 {
			break
		}
	}
	if len(zones) == 0 {
		return []string{"UTC"}
	}
	return zones
}

func readNameServers() []string {
	content, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return []string{}
	}
	servers := make([]string, 0, 4)
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" && net.ParseIP(fields[1]) != nil {
			servers = append(servers, fields[1])
		}
		if len(servers) >= 8 {
			break
		}
	}
	return servers
}

func readHostsEntries() []map[string]any {
	content, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return []map[string]any{}
	}
	items := make([]map[string]any, 0, 16)
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || net.ParseIP(fields[0]) == nil {
			continue
		}
		for _, host := range fields[1:] {
			if strings.HasPrefix(host, "#") {
				break
			}
			items = append(items, map[string]any{"ip": fields[0], "host": host})
			if len(items) >= 200 {
				return items
			}
		}
	}
	return items
}

func readTimeZone() string {
	if content, err := os.ReadFile("/etc/timezone"); err == nil {
		if zone := strings.TrimSpace(string(content)); deviceZonePattern.MatchString(zone) || zone == "UTC" {
			return zone
		}
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		marker := "zoneinfo/"
		if index := strings.LastIndex(target, marker); index >= 0 {
			zone := target[index+len(marker):]
			if deviceZonePattern.MatchString(zone) || zone == "UTC" {
				return zone
			}
		}
	}
	return time.Local.String()
}

func readNTPServer() string {
	content, err := os.ReadFile("/etc/systemd/timesyncd.conf")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "NTP=") {
			fields := strings.Fields(strings.TrimPrefix(line, "NTP="))
			if len(fields) > 0 {
				return fields[0]
			}
		}
	}
	return ""
}

func readSwapBytes() (total, free, used uint64) {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, 0
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, convErr := strconv.ParseUint(fields[1], 10, 64)
		if convErr != nil {
			continue
		}
		switch fields[0] {
		case "SwapTotal:":
			total = value * 1024
		case "SwapFree:":
			free = value * 1024
		}
	}
	if total >= free {
		used = total - free
	}
	return total, free, used
}

func readSwapDetails() []map[string]any {
	content, err := os.ReadFile("/proc/swaps")
	if err != nil {
		return []map[string]any{}
	}
	lines := strings.Split(string(content), "\n")
	items := make([]map[string]any, 0, len(lines))
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		size, _ := strconv.ParseUint(fields[2], 10, 64)
		items = append(items, map[string]any{"path": fields[0], "size": size, "used": fields[3], "isNew": false})
		if len(items) >= 20 {
			break
		}
	}
	return items
}

func readRootAvailable() uint64 {
	// 前端用该值限制 Swap；读不到磁盘容量时给出明确上限，避免 0 把所有保存都判为超限。
	return 64 * 1024 * 1024 * 1024
}

func deviceConfContent(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dns":
		return readLimitedHostFile("/etc/resolv.conf", 1<<20)
	case "hosts":
		return readLimitedHostFile("/etc/hosts", 1<<20)
	default:
		return "", errors.New("设备配置名称无效")
	}
}

func applyDeviceConf(ctx context.Context, key, value string) error {
	if err := mutationRequired(); err != nil {
		return err
	}
	switch key {
	case "Hostname":
		if !deviceHostPattern.MatchString(value) || len(value) > 253 {
			return errors.New("主机名无效")
		}
		_, err := hostCommand(ctx, 10*time.Second, "hostnamectl", "set-hostname", value)
		return err
	case "TimeZone":
		if value != "UTC" && !deviceZonePattern.MatchString(value) {
			return errors.New("时区无效")
		}
		_, err := hostCommand(ctx, 10*time.Second, "timedatectl", "set-timezone", value)
		return err
	case "Ntp":
		if value != "" && !deviceHostPattern.MatchString(value) {
			return errors.New("NTP 服务器无效")
		}
		return writeNTPServer(ctx, value)
	case "LocalTime":
		_, err := hostCommand(ctx, 15*time.Second, "timedatectl", "set-ntp", "true")
		return err
	case "DNS":
		return writeNameServers(value)
	default:
		return errors.New("设备配置项无效")
	}
}

func writeNameServers(value string) error {
	if err := mutationRequired(); err != nil {
		return err
	}
	servers := parseNameServers(value)
	if len(servers) == 0 {
		return errors.New("DNS 地址无效")
	}
	var builder strings.Builder
	for _, server := range servers {
		builder.WriteString("nameserver ")
		builder.WriteString(server)
		builder.WriteByte('\n')
	}
	return writePublicHostFile("/etc/resolv.conf", []byte(builder.String()))
}

func writeNTPServer(ctx context.Context, server string) error {
	content := "[Time]\nNTP=" + server + "\n"
	if err := writePublicHostFile("/etc/systemd/timesyncd.conf", []byte(content)); err != nil {
		return err
	}
	_, err := hostCommand(ctx, 10*time.Second, "timedatectl", "set-ntp", "true")
	return err
}

func applyHostsEntries(ctx context.Context, raw any) error {
	_ = ctx
	if err := mutationRequired(); err != nil {
		return err
	}
	list, ok := raw.([]any)
	if !ok {
		return errors.New("Hosts 请求必须是数组")
	}
	var builder strings.Builder
	builder.WriteString("127.0.0.1 localhost\n::1 localhost ip6-localhost ip6-loopback\n")
	for _, item := range list {
		record, ok := item.(map[string]any)
		if !ok {
			return errors.New("Hosts 项格式无效")
		}
		ip := runtimeString(record, "ip")
		host := runtimeString(record, "host")
		if net.ParseIP(ip) == nil || !deviceHostPattern.MatchString(host) {
			return errors.New("Hosts 地址或主机名无效")
		}
		builder.WriteString(ip)
		builder.WriteByte(' ')
		builder.WriteString(host)
		builder.WriteByte('\n')
		if builder.Len() > 1<<20 {
			return errors.New("Hosts 内容过大")
		}
	}
	return writePublicHostFile("/etc/hosts", []byte(builder.String()))
}

func applyHostsFile(content string) error {
	if err := mutationRequired(); err != nil {
		return err
	}
	if len(content) > 1<<20 || strings.ContainsRune(content, 0) {
		return errors.New("Hosts 文件无效")
	}
	return writePublicHostFile("/etc/hosts", []byte(content))
}

func applyDeviceSwap(ctx context.Context, body map[string]any) error {
	if err := mutationRequired(); err != nil {
		return err
	}
	path := filepath.Clean(runtimeString(body, "path"))
	if !filepath.IsAbs(path) || filepath.Base(path) != ".workmesh_swap" || strings.Contains(path, "..") {
		return errors.New("Swap 路径无效")
	}
	sizeKB, _ := body["size"].(float64)
	if sizeKB < 0 || sizeKB > 64*1024*1024 {
		return errors.New("交换分区大小超出范围")
	}
	_, _ = hostCommand(ctx, 15*time.Second, "swapoff", path)
	if sizeKB == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	bytes := int64(sizeKB) * 1024
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := hostBinary("fallocate"); err == nil {
		if _, err := hostCommand(ctx, 30*time.Second, "fallocate", "-l", strconv.FormatInt(bytes, 10), path); err != nil {
			return err
		}
	} else if _, err := hostCommand(ctx, 60*time.Second, "dd", "if=/dev/zero", "of="+path, "bs=1048576", "count="+strconv.FormatInt((bytes+1048575)/1048576, 10)); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	if _, err := hostCommand(ctx, 15*time.Second, "mkswap", path); err != nil {
		return err
	}
	_, err := hostCommand(ctx, 15*time.Second, "swapon", path)
	return err
}

func checkDeviceDNS(ctx context.Context, body map[string]any) (bool, error) {
	host := runtimeString(body, "host", "domain")
	value := runtimeString(body, "value")
	if host == "" {
		host = "localhost"
	}
	if len(host) > 253 || strings.ContainsAny(host, "/\\ ") || strings.Contains(host, "..") {
		return false, errors.New("DNS 主机名无效")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	servers := parseNameServers(value)
	resolver := net.DefaultResolver
	if len(servers) > 0 {
		server := servers[0]
		resolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
		}}
	}
	_, err := resolver.LookupHost(lookupCtx, host)
	return err == nil, nil
}

func decodeDeviceBody(r io.Reader) (any, error) {
	var raw any
	if err := json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&raw); err != nil {
		if strings.Contains(err.Error(), "EOF") {
			return map[string]any{}, nil
		}
		return nil, err
	}
	return raw, nil
}

func applyDevicePassword(ctx context.Context, account, encoded string) error {
	if err := mutationRequired(); err != nil {
		return err
	}
	if !validHostUser(account) {
		return errors.New("用户名无效")
	}
	password, err := decodePanelSecret(encoded)
	if err != nil {
		return err
	}
	if _, err := hostBinary("chpasswd"); err != nil {
		return errors.New("chpasswd 未安装")
	}
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := hostExec(commandCtx, "chpasswd")
	command.Stdin = strings.NewReader(account + ":" + password + "\n")
	output, runErr := command.CombinedOutput()
	if runErr != nil {
		return errors.New("修改密码失败: " + trimCommandOutput(output))
	}
	return nil
}

func trimCommandOutput(output []byte) string {
	text := strings.TrimSpace(string(output))
	if len(text) > 200 {
		text = text[:200]
	}
	return text
}
