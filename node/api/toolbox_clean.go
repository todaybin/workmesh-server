// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type cleanNode struct {
	ID          string      `json:"id"`
	Label       string      `json:"label"`
	Children    []cleanNode `json:"children"`
	Type        string      `json:"type"`
	Name        string      `json:"name"`
	Size        int64       `json:"size"`
	IsCheck     bool        `json:"isCheck"`
	IsRecommend bool        `json:"isRecommend"`
	CanDelete   bool        `json:"canDelete"`
}

type cleanTarget struct {
	TreeType string
	Name     string
	Path     string
	Docker   string
}

type cleanRoot struct {
	label, typ, category, dir string
	recommend                 bool
	filesOnly                 bool
	dirsOnly                  bool
	protected                 bool
	websiteLogs               bool
	skipNames                 []string
}

var (
	cleanSnapshotMu sync.Mutex
	cleanSnapshot   = map[string]cleanTarget{}
)

func registerToolboxCleanRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/toolbox/scan", handleToolboxScan)
	mux.HandleFunc("POST /api/v2/toolbox/clean", handleToolboxClean)
}

func handleToolboxScan(w http.ResponseWriter, r *http.Request) {
	runtimeOK(w, scanToolboxClean(r.Context()))
}

func handleToolboxClean(w http.ResponseWriter, r *http.Request) {
	if !hostMutationAllowed() {
		runtimeErr(w, http.StatusServiceUnavailable, "清理文件需要 WORKMESH_ALLOW_HOST_MUTATION=1")
		return
	}
	items, err := decodeCleanItems(r)
	if err != nil {
		runtimeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	removed, total, err := applyToolboxClean(r.Context(), items)
	if err != nil {
		runtimeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_ = setDomainSetting("lastCleanTime", now)
	_ = setDomainSetting("lastCleanData", strconv.Itoa(removed))
	_ = setDomainSetting("lastCleanSize", strconv.FormatInt(total, 10))
	runtimeOK(w, map[string]any{"removed": removed, "size": total, "lastCleanTime": now})
}

func decodeCleanItems(r *http.Request) ([]map[string]any, error) {
	if r.Body == nil {
		return nil, errors.New("清理请求不能为空")
	}
	var raw any
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&raw); err != nil {
		return nil, errors.New("解析清理请求失败")
	}
	switch value := raw.(type) {
	case []any:
		return cleanItemMaps(value), nil
	case map[string]any:
		if list, ok := value["items"].([]any); ok {
			return cleanItemMaps(list), nil
		}
		return []map[string]any{value}, nil
	default:
		return nil, errors.New("清理请求格式无效")
	}
}

func cleanItemMaps(list []any) []map[string]any {
	items := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if typed, ok := item.(map[string]any); ok {
			items = append(items, typed)
		}
		if len(items) >= 500 {
			break
		}
	}
	return items
}

func toolboxDataDir() string {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = "./data"
	}
	return filepath.Clean(dir)
}

func toolboxCleanRoots() []cleanRoot {
	return collectToolboxCleanRoots(toolboxDataDir(), legacyPanelDir())
}

// legacyPanelDir 只在显式配置或本机存在旧面板目录时返回路径。测试可把 WORKMESH_PANEL_DIR 设为空来关闭。
func legacyPanelDir() string {
	if value, ok := os.LookupEnv("WORKMESH_PANEL_DIR"); ok {
		return strings.TrimSpace(value)
	}
	const fallback = "/opt/1panel"
	info, err := os.Stat(fallback)
	if err == nil && info.IsDir() {
		return fallback
	}
	return ""
}

func collectToolboxCleanRoots(base, panel string) []cleanRoot {
	roots := []cleanRoot{
		{label: "workmesh_original", typ: "workmesh_original", category: "system", dir: filepath.Join(base, "workmesh_original"), recommend: true},
		{label: "upgrade", typ: "upgrade", category: "system", dir: filepath.Join(base, "releases"), recommend: false},
		{label: "upgrade", typ: "upgrade", category: "system", dir: filepath.Join(base, "upgrade"), recommend: false},
		{label: "agent_packages", typ: "agent_packages", category: "system", dir: filepath.Join(base, "agent", "packages"), recommend: false},
		{label: "snapshot", typ: "snapshot", category: "system", dir: filepath.Join(base, "tmp", "snapshot"), recommend: true},
		{label: "rollback", typ: "rollback", category: "system", dir: filepath.Join(base, "rollback"), recommend: true},
		{label: "tmp_backup", typ: "tmp_backup", category: "backup", dir: filepath.Join(base, "backups", "tmp"), recommend: true},
		{label: "tmp_backup", typ: "tmp_backup", category: "backup", dir: filepath.Join(base, "backup", "tmp"), recommend: true},
		{label: "unknown_app", typ: "unknown_app", category: "backup", dir: filepath.Join(base, "backups", "app"), recommend: false, protected: true},
		{label: "unknown_database", typ: "unknown_database", category: "backup", dir: filepath.Join(base, "backups", "database"), recommend: false, protected: true},
		{label: "unknown_website", typ: "unknown_website", category: "backup", dir: filepath.Join(base, "backups", "website"), recommend: false, protected: true},
		{label: "unknown_snapshot", typ: "unknown_snapshot", category: "backup", dir: filepath.Join(base, "backups"), recommend: false, protected: true, skipNames: []string{"tmp", "app", "database", "website"}},
		{label: "unknown_app", typ: "unknown_app", category: "backup", dir: filepath.Join(base, "backup", "unknown", "app"), recommend: false},
		{label: "unknown_database", typ: "unknown_database", category: "backup", dir: filepath.Join(base, "backup", "unknown", "database"), recommend: false},
		{label: "unknown_website", typ: "unknown_website", category: "backup", dir: filepath.Join(base, "backup", "unknown", "website"), recommend: false},
		{label: "upload", typ: "upload", category: "upload", dir: filepath.Join(base, "uploads"), recommend: true},
		{label: "upload", typ: "upload", category: "upload", dir: filepath.Join(base, "tmp", "upload"), recommend: true},
		{label: "upload", typ: "upload", category: "upload", dir: filepath.Join(base, "chunks"), recommend: false, protected: true},
		{label: "download", typ: "download", category: "download", dir: filepath.Join(base, "downloads"), recommend: true},
		{label: "download", typ: "download", category: "download", dir: filepath.Join(base, "tmp", "download"), recommend: true},
		{label: "app_tmp_download", typ: "app_tmp_download", category: "download", dir: filepath.Join(base, "runtimes", ".downloads"), recommend: false, protected: true},
		{label: "task_log", typ: "task_log", category: "log", dir: filepath.Join(base, "logs", "tasks"), recommend: true, dirsOnly: true, skipNames: []string{"ssl"}},
		{label: "task_log", typ: "task_log", category: "log", dir: filepath.Join(base, "logs", "task"), recommend: true, dirsOnly: true, skipNames: []string{"ssl"}},
		{label: "docker_log", typ: "docker_log", category: "log", dir: filepath.Join(base, "logs", "docker"), recommend: true},
		{label: "system_log", typ: "system_log", category: "log", dir: filepath.Join(base, "logs"), recommend: true, filesOnly: true, skipNames: []string{"server.log", "server-foreground.log"}},
		{label: "website_log", typ: "website_log", category: "log", dir: filepath.Join(base, "websites"), recommend: true, websiteLogs: true},
	}
	if panel == "" {
		return roots
	}
	return append(roots,
		cleanRoot{label: "upgrade", typ: "upgrade", category: "system", dir: filepath.Join(panel, "tmp", "upgrade"), recommend: false},
		cleanRoot{label: "rollback", typ: "rollback", category: "system", dir: filepath.Join(panel, "tmp"), recommend: true, dirsOnly: true, skipNames: []string{"upgrade", "snapshot"}},
		cleanRoot{label: "agent_packages", typ: "agent_packages", category: "system", dir: filepath.Join(panel, "agent", "package"), recommend: false},
		cleanRoot{label: "snapshot", typ: "snapshot", category: "system", dir: filepath.Join(panel, "tmp", "snapshot"), recommend: true},
		cleanRoot{label: "tmp_backup", typ: "tmp_backup", category: "backup", dir: filepath.Join(panel, "backup", "tmp"), recommend: true},
		cleanRoot{label: "unknown_snapshot", typ: "unknown_snapshot", category: "backup", dir: filepath.Join(panel, "backup"), recommend: false, protected: true, skipNames: []string{"tmp"}},
		cleanRoot{label: "upload", typ: "upload", category: "upload", dir: filepath.Join(panel, "uploads"), recommend: true},
		cleanRoot{label: "download", typ: "download", category: "download", dir: filepath.Join(panel, "download"), recommend: true},
		cleanRoot{label: "app_tmp_download", typ: "app_tmp_download", category: "download", dir: filepath.Join(panel, "resource", "apps", "remote"), recommend: false, protected: true},
		cleanRoot{label: "task_log", typ: "task_log", category: "log", dir: filepath.Join(panel, "log", "task"), recommend: true, dirsOnly: true, skipNames: []string{"ssl"}},
		cleanRoot{label: "system_log", typ: "system_log", category: "log", dir: filepath.Join(panel, "log"), recommend: true, filesOnly: true, skipNames: []string{"1Panel.log", "1Panel-Core.log"}},
	)
}

func scanToolboxClean(ctx context.Context) map[string]any {
	next := map[string]cleanTarget{}
	labels := map[string][]string{
		"system":   {"workmesh_original", "upgrade", "agent_packages", "snapshot", "rollback"},
		"backup":   {"tmp_backup", "unknown_app", "unknown_database", "unknown_website", "unknown_snapshot"},
		"upload":   {"upload"},
		"download": {"download", "app_tmp_download"},
		"log":      {"system_log", "task_log", "docker_log", "website_log"},
	}
	grouped := map[string]map[string]*cleanNode{}
	for category, names := range labels {
		grouped[category] = map[string]*cleanNode{}
		for _, name := range names {
			grouped[category][name] = &cleanNode{ID: name, Label: name, Name: name, Type: name, Children: []cleanNode{}}
		}
	}
	for _, root := range toolboxCleanRoots() {
		parent := grouped[root.category][root.label]
		if parent == nil {
			continue
		}
		scanned := scanCleanDirectory(root, next)
		parent.Children = append(parent.Children, scanned.Children...)
		parent.Size += scanned.Size
		if root.recommend {
			parent.IsRecommend = true
		}
	}
	containers := scanDockerClean(ctx, next)
	cleanSnapshotMu.Lock()
	cleanSnapshot = next
	cleanSnapshotMu.Unlock()
	return map[string]any{
		"systemClean": orderedCleanNodes(grouped["system"], labels["system"]), "backupClean": orderedCleanNodes(grouped["backup"], labels["backup"]),
		"uploadClean": orderedCleanNodes(grouped["upload"], labels["upload"]), "downloadClean": orderedCleanNodes(grouped["download"], labels["download"]),
		"systemLogClean": orderedCleanNodes(grouped["log"], labels["log"]), "containerClean": containers,
	}
}

func orderedCleanNodes(nodes map[string]*cleanNode, labels []string) []cleanNode {
	ordered := make([]cleanNode, 0, len(labels))
	for _, label := range labels {
		node := nodes[label]
		if node == nil {
			continue
		}
		ordered = append(ordered, *node)
	}
	return ordered
}

func scanCleanDirectory(root cleanRoot, targets map[string]cleanTarget) cleanNode {
	node := cleanNode{ID: root.typ, Label: root.label, Name: root.typ, Type: root.typ, Children: []cleanNode{}, IsRecommend: root.recommend}
	if root.websiteLogs {
		return scanWebsiteLogTree(root, targets)
	}
	entries, err := os.ReadDir(root.dir)
	if err != nil {
		return node
	}
	rootClean, err := filepath.Abs(root.dir)
	if err != nil {
		return node
	}
	for _, entry := range entries {
		if len(node.Children) >= 200 {
			break
		}
		name := entry.Name()
		if name == "" || name == "." || name == ".." || strings.Contains(name, "..") || cleanNameSkipped(name, root.skipNames) {
			continue
		}
		if root.filesOnly && entry.IsDir() {
			continue
		}
		if root.dirsOnly && !entry.IsDir() {
			continue
		}
		path := filepath.Join(rootClean, name)
		if !cleanPathInside(rootClean, path) {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		size := info.Size()
		if info.IsDir() {
			size = directorySize(path)
		}
		if size <= 0 {
			continue
		}
		deletable := !root.protected
		checked := deletable && root.recommend
		childName := uniqueCleanName(targets, root.typ, name)
		child := cleanNode{ID: root.typ + ":" + childName, Label: name, Name: childName, Type: root.typ, Size: size, Children: []cleanNode{}, IsCheck: checked, IsRecommend: checked, CanDelete: deletable}
		node.Children = append(node.Children, child)
		node.Size += size
		targets[cleanTargetKey(root.typ, childName)] = cleanTarget{TreeType: root.typ, Name: childName, Path: path}
	}
	return node
}

func scanWebsiteLogTree(root cleanRoot, targets map[string]cleanTarget) cleanNode {
	node := cleanNode{ID: root.typ, Label: root.label, Name: root.typ, Type: root.typ, Children: []cleanNode{}, IsRecommend: root.recommend}
	sites, err := os.ReadDir(root.dir)
	if err != nil {
		return node
	}
	for _, site := range sites {
		if len(node.Children) >= 200 || !site.IsDir() || cleanNameSkipped(site.Name(), root.skipNames) {
			continue
		}
		logDir := filepath.Join(root.dir, site.Name(), "logs")
		absLog, err := filepath.Abs(logDir)
		if err != nil {
			continue
		}
		absRoot, err := filepath.Abs(root.dir)
		if err != nil || !cleanPathInside(absRoot, absLog) {
			continue
		}
		size := directorySize(absLog)
		if size <= 0 {
			continue
		}
		childName := uniqueCleanName(targets, root.typ, site.Name())
		node.Children = append(node.Children, cleanNode{ID: root.typ + ":" + childName, Label: site.Name(), Name: childName, Type: root.typ, Size: size, Children: []cleanNode{}, IsCheck: root.recommend, IsRecommend: root.recommend, CanDelete: true})
		node.Size += size
		targets[cleanTargetKey(root.typ, childName)] = cleanTarget{TreeType: root.typ, Name: childName, Path: absLog}
	}
	return node
}

func cleanNameSkipped(name string, skipped []string) bool {
	for _, item := range skipped {
		if name == item {
			return true
		}
	}
	return false
}

func uniqueCleanName(targets map[string]cleanTarget, treeType, name string) string {
	if _, ok := targets[cleanTargetKey(treeType, name)]; !ok {
		return name
	}
	for index := 2; index < 1000; index++ {
		candidate := name + "#" + strconv.Itoa(index)
		if _, ok := targets[cleanTargetKey(treeType, candidate)]; !ok {
			return candidate
		}
	}
	return name
}

func directorySize(root string) int64 {
	var total int64
	count := 0
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || count >= 1<<20 {
			return filepath.SkipDir
		}
		count++
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, statErr := entry.Info()
		if statErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func cleanPathInside(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

func cleanTargetKey(treeType, name string) string { return treeType + "\x00" + name }

var dockerSystemDFLine = regexp.MustCompile(`^(Images|Containers|Local Volumes|Build Cache)\s+(\d+)\s+(\d+)\s+(\S+)\s+(\S+)`)

func scanDockerClean(ctx context.Context, targets map[string]cleanTarget) []cleanNode {
	nodes := []cleanNode{
		{ID: "container_images", Label: "container_images", Name: "container_images", Type: "images", Children: []cleanNode{}},
		{ID: "container_containers", Label: "container_containers", Name: "container_containers", Type: "containers", Children: []cleanNode{}},
		{ID: "container_volumes", Label: "container_volumes", Name: "container_volumes", Type: "volumes", Children: []cleanNode{}},
		{ID: "build_cache", Label: "build_cache", Name: "build_cache", Type: "build_cache", Children: []cleanNode{}},
	}
	if _, err := hostBinary("docker"); err != nil {
		return nodes
	}
	result, err := hostCommand(ctx, 8*time.Second, "docker", "system", "df")
	if err != nil && strings.TrimSpace(result.Stdout) == "" {
		return nodes
	}
	images, containers, volumes, buildCache := parseDockerSystemDF(result.Stdout)
	sizes := []int64{images, containers, volumes, buildCache}
	for index, size := range sizes {
		if size <= 0 {
			continue
		}
		nodes[index].Size = size
		nodes[index].CanDelete = true
		nodes[index].IsRecommend = nodes[index].Name != "container_volumes"
		nodes[index].IsCheck = nodes[index].IsRecommend
		targets[cleanTargetKey(nodes[index].Type, nodes[index].Name)] = cleanTarget{TreeType: nodes[index].Type, Name: nodes[index].Name, Docker: nodes[index].Name}
	}
	return nodes
}

// parseDockerSystemDF 读取 docker system df 的可回收字节。构建缓存使用总大小，避免活跃缓存显示为 0。
func parseDockerSystemDF(text string) (images, containers, volumes, buildCache int64) {
	for _, line := range strings.Split(text, "\n") {
		match := dockerSystemDFLine.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		reclaimable := parseByteSize(match[5])
		switch match[1] {
		case "Images":
			images = reclaimable
		case "Containers":
			containers = reclaimable
		case "Local Volumes":
			volumes = reclaimable
		case "Build Cache":
			buildCache = parseByteSize(match[4])
			if buildCache == 0 {
				buildCache = reclaimable
			}
		}
	}
	return images, containers, volumes, buildCache
}

func parseByteSize(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	index := 0
	for index < len(raw) && (raw[index] == '.' || (raw[index] >= '0' && raw[index] <= '9')) {
		index++
	}
	if index == 0 {
		return 0
	}
	value, err := strconv.ParseFloat(raw[:index], 64)
	if err != nil || value < 0 {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(raw[index:])) {
	case "", "b":
		return int64(value)
	case "kb", "k":
		return int64(value * 1000)
	case "kib":
		return int64(value * 1024)
	case "mb", "m":
		return int64(value * 1000 * 1000)
	case "mib":
		return int64(value * 1024 * 1024)
	case "gb", "g":
		return int64(value * 1000 * 1000 * 1000)
	case "gib":
		return int64(value * 1024 * 1024 * 1024)
	case "tb", "t":
		return int64(value * 1000 * 1000 * 1000 * 1000)
	default:
		return 0
	}
}

func applyToolboxClean(ctx context.Context, items []map[string]any) (int, int64, error) {
	cleanSnapshotMu.Lock()
	snapshot := make(map[string]cleanTarget, len(cleanSnapshot))
	for key, value := range cleanSnapshot {
		snapshot[key] = value
	}
	cleanSnapshotMu.Unlock()
	if len(snapshot) == 0 {
		return 0, 0, errors.New("请先扫描后再清理")
	}
	removed := 0
	var total int64
	for _, item := range items {
		treeType := runtimeString(item, "treeType", "type")
		name := runtimeString(item, "name")
		target, ok := snapshot[cleanTargetKey(treeType, name)]
		if !ok {
			return removed, total, errors.New("清理目标不在最近一次扫描结果中")
		}
		if target.Docker != "" {
			if err := pruneDockerClean(ctx, target.Docker); err != nil {
				return removed, total, err
			}
		} else if err := removeCleanPath(target.Path); err != nil {
			return removed, total, err
		}
		removed++
		if size, ok := item["size"].(float64); ok && size > 0 {
			total += int64(size)
		}
	}
	return removed, total, nil
}

func removeCleanPath(path string) error {
	path = filepath.Clean(path)
	allowedRoot := ""
	for _, root := range toolboxCleanRoots() {
		rootPath, err := filepath.Abs(root.dir)
		if err != nil || !cleanPathInside(rootPath, path) {
			continue
		}
		allowedRoot = rootPath
		break
	}
	if allowedRoot == "" {
		return errors.New("清理路径超出允许目录")
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		resolvedRoot := allowedRoot
		if rootResolved, rootErr := filepath.EvalSymlinks(allowedRoot); rootErr == nil {
			resolvedRoot = rootResolved
		}
		if !cleanPathInside(resolvedRoot, resolved) {
			return errors.New("清理路径超出允许目录")
		}
		path = resolved
	}
	return os.RemoveAll(path)
}

func pruneDockerClean(ctx context.Context, name string) error {
	args := map[string][]string{
		"container_images":     {"image", "prune", "-f"},
		"container_containers": {"container", "prune", "-f"},
		"container_volumes":    {"volume", "prune", "-f"},
		"build_cache":          {"builder", "prune", "-f"},
	}[name]
	if len(args) == 0 {
		return errors.New("容器清理目标无效")
	}
	if _, err := hostBinary("docker"); err != nil {
		return errors.New("docker CLI 未安装")
	}
	_, err := hostCommand(ctx, 30*time.Second, "docker", args...)
	return err
}
