// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func runAppInstallTask(store *appStore, item appRecord, downloadURL, compose string) {
	taskID := appValue(item.Config, "taskID", "taskId")
	update := func(status, message string) error {
		store.mu.Lock()
		var saveErr error
		var previous appRecord
		var found bool
		if index, _ := findApp(store.state.Apps, item.ID); index >= 0 {
			// 先复制记录再更新，避免后台状态写入与请求响应共享 Config map。
			previous = cloneAppRecord(store.state.Apps[index])
			found = true
			current := cloneAppRecord(store.state.Apps[index])
			current.Status, current.Message, current.UpdatedAt = status, message, time.Now().UTC()
			if compose != "" {
				current.Config["dockerCompose"] = compose
			}
			store.state.Apps[index] = current
			item = current
			saveErr = store.saveLocked()
			if saveErr != nil {
				store.state.Apps[index] = previous
			}
		}
		store.mu.Unlock()
		if saveErr != nil {
			return fmt.Errorf("应用状态保存失败: %w", saveErr)
		}
		if err := persistAppInstallRecord(item, appComposePath(item)); err != nil {
			if found {
				store.mu.Lock()
				if index, _ := findApp(store.state.Apps, item.ID); index >= 0 {
					store.state.Apps[index] = previous
					_ = store.saveLocked()
				}
				store.mu.Unlock()
			}
			return fmt.Errorf("应用关系状态保存失败: %w", err)
		}
		if err := ensureAppTaskLogChecked(taskID, item.ID, item.Name, status, message); err != nil {
			if found {
				store.mu.Lock()
				if index, _ := findApp(store.state.Apps, item.ID); index >= 0 {
					store.state.Apps[index] = previous
					_ = store.saveLocked()
				}
				store.mu.Unlock()
			}
			return fmt.Errorf("应用任务状态保存失败: %w", err)
		}
		return nil
	}
	if err := update("installing", "准备安装"); err != nil {
		return
	}
	installDir := appInstallPath(item)
	if err := os.MkdirAll(installDir, 0o750); err != nil {
		_ = update("failed", err.Error())
		return
	}
	if downloadURL != "" {
		if err := update("downloading", "正在下载应用包"); err != nil {
			return
		}
		archivePath := filepath.Join(installDir, "package.tar.gz")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		err := downloadAppArchive(ctx, downloadURL, archivePath)
		cancel()
		if err != nil {
			_ = update("failed", "下载应用包失败: "+err.Error())
			return
		}
		if err := update("installing", "正在解压应用包"); err != nil {
			return
		}
		if err := extractTarGz(archivePath, installDir); err != nil {
			_ = update("failed", "解压应用包失败: "+err.Error())
			return
		}
		if compose == "" {
			compose = findComposeContent(installDir)
		}
	}
	if compose == "" {
		_ = update("failed", "应用包未提供 docker-compose.yml")
		return
	}
	composePath := appComposePath(item)
	if err := writeAtomicFile(composePath, []byte(compose)); err != nil {
		_ = update("failed", "写入 Compose 文件失败: "+err.Error())
		return
	}
	params := map[string]any{}
	if raw, ok := item.Config["params"].(map[string]any); ok {
		params = raw
	}
	params["CONTAINER_NAME"] = item.ContainerName
	if params["PANEL_APP_PORT_HTTP"] == nil {
		if port := appConfiguredInt(item.Config, 0, "PANEL_APP_PORT_HTTP", "httpPort", "port"); port > 0 {
			params["PANEL_APP_PORT_HTTP"] = port
		}
	}
	if err := writeComposeEnv(filepath.Join(filepath.Dir(composePath), ".env"), params); err != nil {
		_ = update("failed", "写入 Compose 环境文件失败: "+err.Error())
		return
	}
	if strings.Contains(compose, "external: true") && strings.Contains(compose, "1panel-network") {
		if err := ensureDockerNetwork("1panel-network"); err != nil {
			_ = update("failed", "创建应用默认网络失败: "+err.Error())
			return
		}
	}
	composeStore := getContainerStore()
	composeStore.mu.Lock()
	foundCompose := false
	for i := range composeStore.state.Composes {
		if composeStore.state.Composes[i].Path == composePath {
			composeStore.state.Composes[i].Name = filepath.Base(filepath.Dir(composePath))
			composeStore.state.Composes[i].AppInstallID = item.ID
			composeStore.state.Composes[i].UpdatedAt = time.Now().UTC()
			foundCompose = true
			break
		}
	}
	if !foundCompose {
		now := time.Now().UTC()
		composeStore.state.Composes = append(composeStore.state.Composes, composeRecord{ID: idToken(), Name: filepath.Base(filepath.Dir(composePath)), Path: composePath, AppInstallID: item.ID, CreatedAt: now, UpdatedAt: now})
	}
	_ = composeStore.saveLocked()
	composeStore.mu.Unlock()
	if err := update("pulling", "正在拉取应用镜像"); err != nil {
		return
	}
	cmd := service.CommandService{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	pullImage := true
	if value, ok := item.Config["pullImage"].(bool); ok {
		pullImage = value
	}
	if result, err := cmd.Execute(ctx, model.CommandRequest{Program: "docker", Args: composePullArgs(composePath, pullImage), Dir: filepath.Dir(composePath), Timeout: 30 * time.Minute}); err != nil || result.ExitCode != 0 {
		message := result.Stderr
		if message == "" && err != nil {
			message = err.Error()
		}
		_ = update("failed", "拉取镜像失败: "+strings.TrimSpace(message))
		return
	}
	appendAppTaskLog(taskID, "应用镜像准备完成")
	if err := update("starting", "正在启动应用容器"); err != nil {
		return
	}
	result, err := cmd.Execute(ctx, model.CommandRequest{Program: "docker", Args: []string{"compose", "-f", composePath, "up", "-d"}, Dir: filepath.Dir(composePath), Timeout: 30 * time.Minute})
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" && err != nil {
			message = err.Error()
		}
		_ = update("failed", "启动容器失败: "+message)
		return
	}
	if containerNames := composeServiceContainers(composePath); containerNames != "" {
		item.ContainerName = containerNames
	}
	item.Config["composePath"] = composePath
	item.Config["composeProject"] = filepath.Base(filepath.Dir(composePath))
	_ = update("running", "安装完成")
	appendAppTaskLog(taskID, "[TASK-END]")
}

func composePullArgs(composePath string, pull bool) []string {
	if !pull {
		return []string{"compose", "-f", composePath, "config", "--quiet"}
	}
	return []string{"compose", "-f", composePath, "pull"}
}

func ensureDockerNetwork(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	commands := service.CommandService{}
	if result, err := commands.Execute(ctx, model.CommandRequest{Program: "docker", Args: []string{"network", "inspect", name}, Timeout: 30 * time.Second}); err == nil && result.ExitCode == 0 {
		return nil
	}
	result, err := commands.Execute(ctx, model.CommandRequest{Program: "docker", Args: []string{"network", "create", name}, Timeout: 30 * time.Second})
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" && err != nil {
			message = err.Error()
		}
		return errors.New(message)
	}
	return nil
}

func downloadAppArchive(ctx context.Context, source, target string) error {
	return downloadAppArchiveWithProgress(ctx, source, target, nil)
}

// downloadAppArchiveWithProgress 下载应用归档并按有限频率报告进度，避免任务日志被单字节写入淹没。
func downloadAppArchiveWithProgress(ctx context.Context, source, target string, progress func(downloaded, total int64)) error {
	parsed, err := validateAppArchiveURL(source)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Minute, CheckRedirect: func(next *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("运行时归档重定向次数过多")
		}
		if !strings.EqualFold(next.URL.Hostname(), parsed.Hostname()) {
			return errors.New("运行时归档禁止跨主机重定向")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	const maxArchiveBytes int64 = 512 << 20
	if resp.ContentLength > maxArchiveBytes {
		return fmt.Errorf("应用归档超过 %d 字节限制", maxArchiveBytes)
	}
	tmp := target + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writer := &archiveProgressWriter{writer: file, total: resp.ContentLength, nextReport: 1 << 20, report: progress}
	written, copyErr := io.Copy(writer, io.LimitReader(resp.Body, maxArchiveBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if written > maxArchiveBytes {
		_ = os.Remove(tmp)
		return fmt.Errorf("应用归档超过 %d 字节限制", maxArchiveBytes)
	}
	return os.Rename(tmp, target)
}

type archiveProgressWriter struct {
	writer     io.Writer
	total      int64
	written    int64
	nextReport int64
	report     func(downloaded, total int64)
}

func (w *archiveProgressWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.written += int64(n)
	if w.report != nil && (w.written >= w.nextReport || err != nil) {
		w.report(w.written, w.total)
		for w.nextReport <= w.written {
			w.nextReport += 1 << 20
		}
	}
	return n, err
}

func validateAppArchiveURL(source string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("应用归档地址无效")
	}
	if parsed.Scheme != "https" {
		if parsed.Scheme != "http" || !strings.EqualFold(os.Getenv("WORKMESH_ALLOW_LOCAL_APP_ARCHIVES"), "true") {
			return nil, errors.New("应用归档只允许 HTTPS 地址")
		}
		ip := net.ParseIP(parsed.Hostname())
		if parsed.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, errors.New("本地 HTTP 应用归档只允许回环地址")
		}
	}
	allowed := map[string]struct{}{}
	for _, candidate := range []string{appStoreRemoteBase(), os.Getenv("WORKMESH_APP_MIRROR")} {
		if value, parseErr := url.Parse(strings.TrimSpace(candidate)); parseErr == nil && value.Hostname() != "" {
			allowed[strings.ToLower(value.Hostname())] = struct{}{}
		}
	}
	for _, host := range strings.Split(os.Getenv("WORKMESH_APP_ARCHIVE_HOSTS"), ",") {
		if host = strings.ToLower(strings.TrimSpace(host)); host != "" {
			allowed[host] = struct{}{}
		}
	}
	if parsed.Scheme == "https" {
		if _, ok := allowed[strings.ToLower(parsed.Hostname())]; !ok {
			return nil, errors.New("应用归档主机不在允许列表")
		}
	}
	return parsed, nil
}

func extractTarGz(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var extracted int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(header.Name)
		if name == "." || name == ".." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("压缩包包含非法路径: %s", header.Name)
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink || header.Typeflag == tar.TypeChar || header.Typeflag == tar.TypeBlock || header.Typeflag == tar.TypeFifo {
			return fmt.Errorf("压缩包包含不安全文件类型: %s", header.Name)
		}
		target := filepath.Join(destination, name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if header.Size < 0 || header.Size > 512<<20 || extracted+header.Size > 512<<20 {
				return errors.New("压缩包解压内容超过 512 MiB 限制")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			written, copyErr := io.Copy(out, io.LimitReader(reader, header.Size+1))
			_ = out.Close()
			if copyErr != nil {
				return copyErr
			}
			if written != header.Size {
				return errors.New("压缩包文件大小不匹配")
			}
			extracted += written
		default:
			return fmt.Errorf("压缩包包含不支持的文件类型: %s", header.Name)
		}
	}
}

func findComposeContent(root string) string {
	var result string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || result != "" || info == nil || info.IsDir() {
			return nil
		}
		base := strings.ToLower(info.Name())
		if base == "docker-compose.yml" || base == "docker-compose.yaml" || base == "compose.yml" || base == "compose.yaml" {
			if data, readErr := os.ReadFile(path); readErr == nil {
				result = string(data)
			}
		}
		return nil
	})
	return result
}

func appComposePath(item appRecord) string {
	return filepath.Join(appInstallPath(item), "docker-compose.yml")
}

func composeServiceContainers(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := (service.CommandService{}).Execute(ctx, model.CommandRequest{Program: service.DockerBinary(), Args: []string{"compose", "-f", path, "ps", "--format", "{{.Name}}"}, Dir: filepath.Dir(path), Timeout: 30 * time.Second})
	if err != nil || result.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

func writeComposeEnv(path string, params map[string]any) error {
	lines := make([]string, 0, len(params))
	for key, value := range params {
		if !validEnvKey(key) {
			continue
		}
		text := strings.ReplaceAll(fmt.Sprint(value), "\n", "")
		lines = append(lines, key+"="+text)
	}
	sort.Strings(lines)
	return writeAtomicFile(path, []byte(strings.Join(lines, "\n")+"\n"))
}
