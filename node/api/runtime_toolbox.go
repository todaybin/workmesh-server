// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var nodeModuleNamePattern = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`)
var phpExtensionNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
var supervisorProcessNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var supervisorUserPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)

// runtimeRecord 是运行时及其扩展的最小持久化模型。
type runtimeRecord struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Type            string         `json:"type"`
	Version         string         `json:"version"`
	CodeDir         string         `json:"codeDir,omitempty"`
	WorkDir         string         `json:"workDir,omitempty"`
	Status          string         `json:"status"`
	Remark          string         `json:"remark,omitempty"`
	Resource        string         `json:"resource,omitempty"`
	Source          string         `json:"source,omitempty"`
	DownloadURL     string         `json:"downloadUrl,omitempty"`
	PackageModified int            `json:"packageModified,omitempty"`
	Image           string         `json:"image,omitempty"`
	DockerCompose   string         `json:"dockerCompose,omitempty"`
	Params          map[string]any `json:"params,omitempty"`
	Env             string         `json:"env,omitempty"`
	AppDetailID     string         `json:"appDetailID,omitempty"`
	AppID           string         `json:"appID,omitempty"`
	Port            int            `json:"port,omitempty"`
	Container       string         `json:"container,omitempty"`
	ExposedPorts    []any          `json:"exposedPorts,omitempty"`
	Environments    []any          `json:"environments,omitempty"`
	Volumes         []any          `json:"volumes,omitempty"`
	ExtraHosts      []any          `json:"extraHosts,omitempty"`
	TaskID          string         `json:"taskID,omitempty"`
	TaskStatus      string         `json:"taskStatus,omitempty"`
	Message         string         `json:"message,omitempty"`
	Error           string         `json:"error,omitempty"`
	InstallPath     string         `json:"path,omitempty"`
	ComposePath     string         `json:"composePath,omitempty"`
	Extensions      []string       `json:"extensions,omitempty"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

type runtimeState struct {
	Runtimes []runtimeRecord `json:"runtimes"`
	Settings map[string]any  `json:"settings"`
}

type phpExtensionDefinition struct {
	Name        string
	Description string
	Check       string
	File        string
	Versions    []string
}

type phpExtensionTemplate struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Extensions string `json:"extensions"`
	CreatedAt  string `json:"createdAt,omitempty"`
	UpdatedAt  string `json:"updatedAt,omitempty"`
}

const phpExtensionTemplatesSetting = "php_extension_templates"

var defaultPHPExtensionTemplates = []phpExtensionTemplate{
	{ID: 1, Name: "Default", Extensions: "bcmath,ftp,gd,gettext,intl,mysqli,pcntl,pdo_mysql,shmop,soap,sockets,sysvsem,xmlrpc,zip"},
	{ID: 2, Name: "WordPress", Extensions: "exif,igbinary,imagick,intl,zip,apcu,memcached,opcache,redis,shmop,mysqli,pdo_mysql,gd"},
	{ID: 3, Name: "Flarum", Extensions: "curl,gd,pdo_mysql,mysqli,bz2,exif,yaf,imap"},
	{ID: 4, Name: "SeaCMS", Extensions: "mysqli,pdo_mysql,gd,curl"},
	{ID: 5, Name: "Dev", Extensions: "bcmath,ftp,gd,gettext,intl,mysqli,pcntl,pdo_mysql,shmop,soap,sockets,sysvsem,xmlrpc,zip,exif,igbinary,imagick,apcu,memcached,opcache,redis,bc,image,dom,iconv,mbstring,mysqlnd,openssl,pdo,tokenizer,xml,curl,bz2,yaf,imap,xdebug,swoole,pdo_pgsql,fileinfo,pgsql,calendar,gmp"},
}

var phpExtensionCatalog = buildPHPExtensionCatalog()

func buildPHPExtensionCatalog() []phpExtensionDefinition {
	all := []string{"56", "70", "71", "72", "73", "74", "80", "81", "82", "83", "84", "85"}
	from70 := []string{"70", "71", "72", "73", "74", "80", "81", "82", "83", "84", "85"}
	until74 := []string{"56", "70", "71", "72", "73", "74"}
	item := func(name, check, file string, versions []string) phpExtensionDefinition {
		return phpExtensionDefinition{Name: name, Check: check, File: file, Versions: append([]string(nil), versions...)}
	}
	return []phpExtensionDefinition{
		item("amqp", "amqp", "amqp.so", all),
		item("apcu", "apcu", "apcu.so", all),
		item("bcmath", "bcmath", "bcmath.so", all),
		item("ionCube", "ionCube Loader", "ioncube_loader.so", []string{"56", "70", "71", "72", "73", "74", "81", "82"}),
		item("opcache", "Zend OPcache", "opcache.so", all),
		item("memcache", "memcache", "memcache.so", []string{"56", "70", "71", "72", "73", "74", "80"}),
		item("memcached", "memcached", "memcached.so", all),
		item("redis", "redis", "redis.so", all),
		item("mcrypt", "mcrypt", "mcrypt.so", from70),
		item("imagick", "imagick", "imagick.so", all),
		item("xdebug", "xdebug", "xdebug.so", all),
		item("imap", "imap", "imap.so", all),
		item("exif", "exif", "exif.so", all),
		item("intl", "intl", "intl.so", all),
		item("xsl", "xsl", "xsl.so", []string{"56", "70", "71", "72", "73", "74", "80", "81", "82"}),
		item("swoole", "swoole", "swoole.so", all),
		item("zstd", "zstd", "zstd.so", all),
		item("xlswriter", "xlswriter", "xlswriter.so", from70),
		item("oci8", "oci8", "oci8.so", from70),
		item("pdo_oci", "pdo_oci", "pdo_oci.so", from70),
		item("pdo_sqlsrv", "pdo_sqlsrv", "pdo_sqlsrv.so", from70),
		item("sqlsrv", "sqlsrv", "sqlsrv.so", []string{"81", "82", "83", "84"}),
		item("yaf", "yaf", "yaf.so", all),
		item("mongodb", "mongodb", "mongodb.so", all),
		item("yac", "yac", "yac.so", from70),
		item("pgsql", "pgsql", "pgsql.so", all),
		item("ssh2", "ssh2", "ssh2.so", all),
		item("grpc", "grpc", "grpc.so", all),
		item("xhprof", "xhprof", "xhprof.so", all),
		item("protobuf", "protobuf", "protobuf.so", all),
		item("pdo_pgsql", "pdo_pgsql", "pdo_pgsql.so", all),
		item("snmp", "snmp", "snmp.so", all),
		item("ldap", "ldap", "ldap.so", all),
		item("recode", "recode", "recode.so", []string{"56", "70", "71", "72", "73"}),
		item("enchant", "enchant", "enchant.so", all),
		item("pspell", "pspell", "pspell.so", all),
		item("bz2", "bz2", "bz2.so", all),
		item("sysvshm", "sysvshm", "sysvshm.so", all),
		item("calendar", "calendar", "calendar.so", all),
		item("gmp", "gmp", "gmp.so", all),
		item("wddx", "wddx", "wddx.so", until74),
		item("sysvmsg", "sysvmsg", "sysvmsg.so", all),
		item("igbinary", "igbinary", "igbinary.so", all),
		item("zmq", "zmq", "zmq.so", all),
		item("smbclient", "smbclient", "smbclient.so", all),
		item("event", "event", "event.so", all),
		item("mailparse", "mailparse", "mailparse.so", all),
		item("yaml", "yaml", "yaml.so", all),
		item("sg16", "SourceGuardian", "sourceguardian.so", all),
		item("mysqli", "mysqli", "mysqli.so", all),
		item("pdo_mysql", "pdo_mysql", "pdo_mysql.so", all),
		item("zip", "zip", "zip.so", all),
		item("shmop", "shmop", "shmop.so", all),
		item("gd", "gd", "gd.so", all),
		item("pcntl", "pcntl", "pcntl.so", all),
		item("sodium", "sodium", "sodium.so", from70),
		item("gettext", "gettext", "gettext.so", all),
		item("soap", "soap", "soap.so", all),
		item("sysvsem", "sysvsem", "sysvsem.so", all),
		item("sockets", "sockets", "sockets.so", all),
		item("xmlrpc", "xmlrpc", "xmlrpc.so", all),
		item("lz4", "lz4", "lz4.so", all),
		item("msgpack", "msgpack", "msgpack.so", all),
	}
}

func (definition phpExtensionDefinition) toMap(installed bool) map[string]any {
	return map[string]any{"name": definition.Name, "description": definition.Description, "installed": installed, "check": definition.Check, "file": definition.File, "versions": definition.Versions}
}

var runtimeStoreMu sync.Mutex
var runtimeStoreInstance *runtimeStore

type runtimeStore struct {
	mu         sync.RWMutex
	path       string
	repository runtimeRepository
	commands   runtimeCommandExecutor
	state      runtimeState
	loadErr    error
}

type runtimeCommandExecutor interface {
	Execute(context.Context, model.CommandRequest) (model.CommandResult, error)
}

func getRuntimeStore() *runtimeStore {
	dir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dir == "" {
		dir = "./data"
	}
	path := filepath.Join(dir, "runtime.json")
	runtimeStoreMu.Lock()
	defer runtimeStoreMu.Unlock()
	if runtimeStoreInstance != nil && runtimeStoreInstance.path == path && runtimeStoreInstance.repository.db == sharedDB() {
		return runtimeStoreInstance
	}
	s := &runtimeStore{path: path, repository: runtimeRepository{db: sharedDB()}, commands: service.CommandService{}, state: runtimeState{Runtimes: []runtimeRecord{}, Settings: map[string]any{}}}
	if s.repository.db == nil {
		s.loadErr = errors.New("运行时 SQLite 未初始化")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.state, s.loadErr = s.repository.load(ctx)
		cancel()
	}
	if s.state.Settings == nil {
		s.state.Settings = map[string]any{}
	}
	interrupted := false
	for index := range s.state.Runtimes {
		switch strings.ToLower(strings.TrimSpace(s.state.Runtimes[index].Status)) {
		case "creating", "building", "recreating", "installing", "downloading", "pulling", "starting":
			s.state.Runtimes[index].Status = "Error"
			s.state.Runtimes[index].TaskStatus = "failed"
			s.state.Runtimes[index].Message = "系统重启导致任务中断"
			s.state.Runtimes[index].Error = "系统重启导致任务中断"
			s.state.Runtimes[index].UpdatedAt = time.Now().UTC()
			interrupted = true
		}
	}
	if interrupted && s.loadErr == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.loadErr = s.repository.save(ctx, s.state)
		cancel()
	}
	if _, exists := s.state.Settings[phpExtensionTemplatesSetting]; !exists {
		s.state.Settings[phpExtensionTemplatesSetting] = append([]phpExtensionTemplate(nil), defaultPHPExtensionTemplates...)
		if s.loadErr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			s.loadErr = s.repository.save(ctx, s.state)
			cancel()
		}
	}
	runtimeStoreInstance = s
	return s
}

func (s *runtimeStore) saveLocked() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.repository.save(ctx, s.state); err != nil {
		if restored, loadErr := s.repository.load(ctx); loadErr == nil {
			s.state = restored
		}
		return err
	}
	s.loadErr = nil
	return nil
}

func (s *runtimeStore) commandExecutor() runtimeCommandExecutor {
	if s.commands != nil {
		return s.commands
	}
	return service.CommandService{}
}

func runtimeBody(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, nil
	}
	var v map[string]any
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	if err := decoder.Decode(&v); err != nil {
		if strings.Contains(err.Error(), "EOF") {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("请求 JSON 只能包含一个对象")
	}
	if v == nil {
		return nil, errors.New("请求 JSON 必须是对象")
	}
	if v == nil {
		v = map[string]any{}
	}
	return v, nil
}

func runtimeString(v map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := v[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
		if n, ok := v[k].(float64); ok && n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		if n, ok := v[k].(int); ok {
			return strconv.Itoa(n)
		}
	}
	return ""
}
func runtimeOK(w http.ResponseWriter, data any) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
}
func runtimeErr(w http.ResponseWriter, status int, msg string) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": msg})
}

func runtimeErrData(w http.ResponseWriter, status int, msg string, data any) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": msg, "data": data})
}

// RegisterRuntimeToolboxRoutes 注册运行时、终端、SSH 与工具箱接口。
func RegisterRuntimeToolboxRoutes(mux *http.ServeMux) {
	s := getRuntimeStore()
	registerRuntimeRoutes(mux, s)
	registerTerminalRoutes(mux)
	registerSSHRoutes(mux, s)
	registerToolboxRoutes(mux, s)
}

func isRuntimeToolboxRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	for _, prefix := range []string{"/api/v2/runtimes", "/api/v2/hosts/terminal", "/api/v2/settings/ssh", "/api/v2/settings/terminal/ai", "/api/v2/toolbox"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func runtimePage(body map[string]any) (int, int, error) {
	parse := func(key string, fallback int) (int, error) {
		raw, ok := body[key]
		if !ok || raw == nil {
			return fallback, nil
		}
		value, ok := raw.(float64)
		if !ok || value < 1 || value != float64(int(value)) || value > 10000 {
			return 0, fmt.Errorf("%s 必须是正整数", key)
		}
		return int(value), nil
	}
	page, err := parse("page", 1)
	if err != nil {
		return 0, 0, err
	}
	pageSize, err := parse("pageSize", 20)
	if err != nil {
		return 0, 0, err
	}
	if pageSize > 200 {
		return 0, 0, errors.New("pageSize 不能超过 200")
	}
	return page, pageSize, nil
}

func runtimeStatusMatches(item runtimeRecord, filter string) bool {
	status := strings.ToLower(strings.TrimSpace(item.Status))
	if filter == "normal" && strings.EqualFold(item.Type, "php") {
		// PHP uses Normal as the list's healthy bucket while its actual state is
		// often Running/Building/Stopped.  Keep failures out of this bucket.
		return status != "error" && status != "failed" && status != "unhealthy"
	}
	return status == filter
}

func runtimeRecordFromRequest(v map[string]any) (runtimeRecord, error) {
	name := runtimeString(v, "name")
	if name == "" {
		return runtimeRecord{}, errors.New("运行时名称不能为空")
	}
	if strings.ContainsAny(name, "\r\n\x00/\\") {
		return runtimeRecord{}, errors.New("运行时名称无效")
	}
	item := runtimeRecord{
		ID: runtimeString(v, "id", "runtimeId"), Name: name, Type: runtimeString(v, "type"),
		Version: runtimeString(v, "version"), CodeDir: runtimeString(v, "codeDir", "path"),
		WorkDir: runtimeString(v, "workDir"), Resource: runtimeString(v, "resource"), Source: runtimeString(v, "source"),
		Image: runtimeString(v, "image"), DockerCompose: runtimeString(v, "dockerCompose", "compose"),
		DownloadURL: runtimeString(v, "downloadUrl", "downloadURL"),
		Env:         runtimeString(v, "env"), AppDetailID: runtimeString(v, "appDetailID", "appDetailId"), AppID: runtimeString(v, "appID", "appId"),
		Remark: runtimeString(v, "remark"), TaskID: runtimeString(v, "taskID", "taskId"), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		Params: map[string]any{},
	}
	if raw, ok := v["params"].(map[string]any); ok {
		for key, value := range raw {
			item.Params[key] = value
		}
	}
	if normalizeRuntimeTypeFilter(item.Type) == "node" {
		if item.Source != "" {
			item.Params["CONTAINER_PACKAGE_URL"] = item.Source
		}
		if install, ok := v["install"].(bool); ok {
			if install {
				item.Params["RUN_INSTALL"] = "1"
			} else {
				item.Params["RUN_INSTALL"] = "0"
			}
		}
	}
	if normalizeRuntimeTypeFilter(item.Type) == "php" {
		if item.Source != "" {
			item.Params["CONTAINER_PACKAGE_URL"] = item.Source
		}
		if raw, exists := item.Params["PHP_EXTENSIONS"]; exists {
			extensions, extensionErr := normalizePHPExtensions(raw)
			if extensionErr != nil {
				return runtimeRecord{}, extensionErr
			}
			item.Extensions = strings.Split(extensions, ",")
			if extensions == "" {
				item.Extensions = []string{}
			}
			item.Params["PHP_EXTENSIONS"] = extensions
		}
	}
	item.Container = runtimeString(v, "container", "containerName")
	if item.Container == "" {
		item.Container = runtimeString(item.Params, "CONTAINER_NAME", "containerName")
	}
	if item.Container == "" && runtimeInstallRequested(v, item) {
		item.Container = item.Name
	}
	if item.Container != "" && !validDockerIdentifier(item.Container) {
		if runtimeInstallRequested(v, item) {
			return runtimeRecord{}, errors.New("容器名无效")
		}
		item.Container = ""
	}
	if rawPort, exists := v["port"]; exists {
		value, ok := runtimeNumberValue(rawPort)
		if !ok || value < 1 || value > 65535 {
			return runtimeRecord{}, errors.New("端口必须在 1-65535 范围内")
		}
		item.Port = value
	}
	for _, key := range []string{"exposedPorts", "environments", "volumes", "extraHosts"} {
		if raw, ok := v[key].([]any); ok {
			switch key {
			case "exposedPorts":
				item.ExposedPorts = raw
			case "environments":
				item.Environments = raw
			case "volumes":
				item.Volumes = raw
			case "extraHosts":
				item.ExtraHosts = raw
			}
		}
	}
	if item.Port == 0 && len(item.ExposedPorts) > 0 {
		if portMap, ok := item.ExposedPorts[0].(map[string]any); ok {
			item.Port, _ = runtimeNumberValue(portMap["hostPort"])
		}
	}
	if item.Port < 0 || item.Port > 65535 {
		return runtimeRecord{}, errors.New("端口必须在 1-65535 范围内")
	}
	if err := normalizeRuntimePorts(&item, runtimeInstallRequested(v, item)); err != nil {
		return runtimeRecord{}, err
	}
	if item.CodeDir != "" {
		clean, err := filepath.Abs(filepath.Clean(item.CodeDir))
		if err != nil {
			return runtimeRecord{}, errors.New("代码目录无效")
		}
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return runtimeRecord{}, errors.New("代码目录无效")
		}
		item.CodeDir = clean
	}
	hydrateRuntimePaths(&item)
	if err := validateRuntimeCollections(item); err != nil {
		return runtimeRecord{}, err
	}
	if err := validateRuntimeCreateLocked(nil, item); err != nil {
		return runtimeRecord{}, err
	}
	return item, nil
}

func validateRuntimeCollections(item runtimeRecord) error {
	for _, raw := range item.Environments {
		entry, ok := raw.(map[string]any)
		if !ok || !validEnvKey(runtimeString(entry, "key")) {
			return errors.New("环境变量参数无效")
		}
		if strings.ContainsRune(fmt.Sprint(entry["value"]), '\x00') {
			return errors.New("环境变量值无效")
		}
	}
	for _, raw := range item.Volumes {
		entry, ok := raw.(map[string]any)
		source, target := runtimeString(entry, "source"), runtimeString(entry, "target")
		mode := strings.ToLower(runtimeString(entry, "mode"))
		if !ok || source == "" || !filepath.IsAbs(target) || strings.ContainsAny(source+target, "\r\n\x00") || (mode != "" && mode != "ro" && mode != "rw") {
			return errors.New("卷映射参数无效")
		}
	}
	for _, raw := range item.ExtraHosts {
		entry, ok := raw.(map[string]any)
		hostname, ip := runtimeString(entry, "hostname", "host"), runtimeString(entry, "ip")
		if !ok || hostname == "" || len(hostname) > 253 || strings.ContainsAny(hostname, " /\\\r\n\x00") || net.ParseIP(ip) == nil {
			return errors.New("extra_hosts 参数无效")
		}
	}
	return nil
}

func runtimeNumberValue(raw any) (int, bool) {
	switch value := raw.(type) {
	case float64:
		return int(value), value == float64(int(value))
	case int:
		return value, true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(value))
		return n, err == nil
	default:
		return 0, false
	}
}

// normalizeRuntimePorts keeps the host and container port variables in sync
// with the 1Panel runtime contract. All language runtime templates expose
// ${APP_PORT} inside the container and ${PANEL_APP_PORT_HTTP} on the host.
func normalizeRuntimePorts(item *runtimeRecord, required bool) error {
	if item == nil || normalizeRuntimeTypeFilter(item.Type) == "php" {
		return nil
	}
	if item.Params == nil {
		item.Params = map[string]any{}
	}
	containerPort := 0
	if raw, exists := item.Params["APP_PORT"]; exists {
		value, ok := runtimeNumberValue(raw)
		if !ok || value < 1 || value > 65535 {
			return errors.New("APP_PORT 必须在 1-65535 范围内")
		}
		containerPort = value
	}
	if containerPort == 0 && len(item.ExposedPorts) > 0 {
		if value, ok := runtimeNumberValue(runtimeMapValue(item.ExposedPorts[0], "containerPort")); ok {
			containerPort = value
		}
	}
	if containerPort == 0 {
		containerPort = item.Port
	}
	if containerPort == 0 {
		if required {
			return errors.New("运行时容器端口不能为空，请设置 params.APP_PORT、exposedPorts.containerPort 或 port")
		}
		return nil
	}
	if containerPort < 1 || containerPort > 65535 {
		return errors.New("APP_PORT 必须在 1-65535 范围内")
	}
	item.Params["APP_PORT"] = containerPort
	if item.Port == 0 && len(item.ExposedPorts) > 0 {
		item.Port, _ = runtimeNumberValue(runtimeMapValue(item.ExposedPorts[0], "hostPort"))
	}
	if item.Port == 0 {
		item.Port = containerPort
	}
	if item.Port < 1 || item.Port > 65535 {
		return errors.New("端口必须在 1-65535 范围内")
	}
	return nil
}

func runtimeMapValue(raw any, key string) any {
	if value, ok := raw.(map[string]any); ok {
		return value[key]
	}
	return nil
}

func validateRuntimeCreateLocked(items []runtimeRecord, candidate runtimeRecord) error {
	seenHost := map[string]bool{}
	seenContainer := map[string]bool{}
	for _, raw := range candidate.ExposedPorts {
		portMap, ok := raw.(map[string]any)
		if !ok {
			return errors.New("exposedPorts 参数无效")
		}
		host, hostOK := runtimeNumberValue(portMap["hostPort"])
		container, containerOK := runtimeNumberValue(portMap["containerPort"])
		if !hostOK || !containerOK || host < 1 || host > 65535 || container < 1 || container > 65535 {
			return errors.New("映射端口必须在 1-65535 范围内")
		}
		protocol := strings.ToLower(runtimeString(portMap, "protocol"))
		if protocol == "" {
			protocol = "tcp"
		}
		if protocol != "tcp" && protocol != "udp" {
			return errors.New("端口协议只允许 tcp 或 udp")
		}
		hostKey, containerKey := fmt.Sprintf("%d/%s", host, protocol), fmt.Sprintf("%d/%s", container, protocol)
		if seenHost[hostKey] || seenContainer[containerKey] {
			return errors.New("映射端口重复")
		}
		seenHost[hostKey], seenContainer[containerKey] = true, true
	}
	for _, item := range items {
		if strings.EqualFold(item.Name, candidate.Name) || (candidate.Container != "" && strings.EqualFold(item.Container, candidate.Container)) {
			return errors.New("运行时名称或容器名已存在")
		}
		if candidate.CodeDir != "" && strings.EqualFold(item.CodeDir, candidate.CodeDir) {
			return errors.New("代码目录已被其他运行时使用")
		}
		if candidate.Image != "" && item.Image != "" && strings.EqualFold(item.Image, candidate.Image) {
			return errors.New("运行时镜像已被使用")
		}
		if candidate.Port > 0 && item.Port == candidate.Port || runtimePortsConflict(item.ExposedPorts, candidate.ExposedPorts) {
			return errors.New("运行时端口已被占用")
		}
	}
	return nil
}

func runtimePortsConflict(left, right []any) bool {
	seen := map[string]struct{}{}
	for _, raw := range left {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		port, valid := runtimeNumberValue(entry["hostPort"])
		if !valid {
			continue
		}
		protocol := strings.ToLower(runtimeString(entry, "protocol"))
		if protocol == "" {
			protocol = "tcp"
		}
		seen[fmt.Sprintf("%d/%s", port, protocol)] = struct{}{}
	}
	for _, raw := range right {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		port, valid := runtimeNumberValue(entry["hostPort"])
		if !valid {
			continue
		}
		protocol := strings.ToLower(runtimeString(entry, "protocol"))
		if protocol == "" {
			protocol = "tcp"
		}
		if _, exists := seen[fmt.Sprintf("%d/%s", port, protocol)]; exists {
			return true
		}
	}
	return false
}

func runtimeInstallRequested(v map[string]any, item runtimeRecord) bool {
	if value, ok := v["install"].(bool); ok {
		return value
	}
	return strings.EqualFold(item.Resource, "appstore") || item.Image != "" || item.DockerCompose != ""
}

func runtimeTaskLogPath(taskID string) string {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	return filepath.Join(root, "logs", "tasks", "runtime", taskID+".log")
}

func runtimeCanonicalInstallPath(item runtimeRecord) string {
	typ := normalizeRuntimeTypeFilter(item.Type)
	if item.Name == "" || (typ != "php" && typ != "go" && typ != "java" && typ != "node" && typ != "python" && typ != "dotnet") {
		return ""
	}
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	return filepath.Join(root, "runtimes", typ, item.Name)
}

// hydrateRuntimePaths 补齐 1Panel RuntimeDTO 中的 path、Compose 和容器字段，兼容旧记录。
func hydrateRuntimePaths(item *runtimeRecord) {
	if item == nil {
		return
	}
	if item.Container == "" {
		item.Container = runtimeString(item.Params, "CONTAINER_NAME", "containerName")
	}
	installPath := runtimeCanonicalInstallPath(*item)
	if installPath == "" {
		return
	}
	item.InstallPath = installPath
	item.ComposePath = filepath.Join(installPath, "docker-compose.yml")
}

func persistRuntimeInstallPath(s *runtimeStore, id string, installPath string) {
	if s == nil || id == "" || installPath == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.state.Runtimes {
		if s.state.Runtimes[index].ID != id {
			continue
		}
		hydrateRuntimePaths(&s.state.Runtimes[index])
		if s.state.Runtimes[index].InstallPath == "" {
			s.state.Runtimes[index].InstallPath = installPath
			s.state.Runtimes[index].ComposePath = filepath.Join(installPath, "docker-compose.yml")
		}
		_ = s.saveLocked()
		return
	}
}

func appendRuntimeTaskLog(taskID, message string) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(message) == "" {
		return
	}
	path := runtimeTaskLogPath(taskID)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintf(file, "%s %s\n", time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(message))
}

// appendRuntimeTaskOutput 将 Docker stdout/stderr 同步写入统一任务日志，供任务抽屉实时轮询。
func appendRuntimeTaskOutput(taskID, stream string, data []byte) {
	if strings.TrimSpace(taskID) == "" || len(data) == 0 {
		return
	}
	label := strings.ToUpper(strings.TrimSpace(stream))
	for _, line := range strings.Split(strings.TrimRight(string(data), "\r\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		message := line
		if label != "" {
			message = "[" + label + "] " + line
		}
		appendAppTaskLog(taskID, message)
		appendRuntimeTaskLog(taskID, message)
	}
}

func updateRuntimeTask(s *runtimeStore, id, status, message string) runtimeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Runtimes {
		if s.state.Runtimes[i].ID != id {
			continue
		}
		item := s.state.Runtimes[i]
		hydrateRuntimePaths(&item)
		item.TaskStatus = strings.ToLower(status)
		item.UpdatedAt = time.Now().UTC()
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "running":
			item.Status, item.Message, item.Error = "Running", "", ""
		case "failed", "error":
			item.Status, item.Message, item.Error = "Error", message, message
		case "building":
			item.Status = "Building"
		case "recreating":
			item.Status = "ReCreating"
		case "restarting":
			item.Status = "Restarting"
		case "stopped":
			item.Status = "Stopped"
		case "downloading", "installing", "pulling", "starting":
			item.Status = "Creating"
		default:
			item.Status = "Creating"
		}
		if strings.TrimSpace(message) != "" && strings.ToLower(strings.TrimSpace(status)) != "running" {
			item.Message = message
		}
		s.state.Runtimes[i] = item
		_ = s.saveLocked()
		return item
	}
	return runtimeRecord{}
}

func runRuntimeInstallTask(s *runtimeStore, item runtimeRecord) {
	taskID := item.TaskID
	if taskID == "" {
		taskID = idToken()
	}
	// The executor callback uses the record's task ID to stream Docker output
	// into the shared task log.  Persist the generated ID on the working copy
	// before any build/download command starts.
	item.TaskID = taskID
	s.mu.Lock()
	for index := range s.state.Runtimes {
		if s.state.Runtimes[index].ID == item.ID {
			s.state.Runtimes[index].TaskID = taskID
			_ = s.saveLocked()
			break
		}
	}
	s.mu.Unlock()
	appendRuntimeTaskLog(taskID, "开始安装运行时")
	defer appendRuntimeTaskLog(taskID, "[TASK-END]")
	update := func(status, message string) {
		item = updateRuntimeTask(s, item.ID, status, message)
		// The task drawer reads the shared task-log domain.  Keep a runtime task
		// there as well as the dedicated runtime log file for restart recovery.
		ensureAppTaskLog(taskID, item.ID, item.Name, status, message)
		appendRuntimeTaskLog(taskID, message)
	}
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	installDir := filepath.Join(root, "runtimes", strings.ToLower(item.Type), item.Name)
	item.InstallPath = installDir
	item.ComposePath = filepath.Join(installDir, "docker-compose.yml")
	persistRuntimeInstallPath(s, item.ID, installDir)
	compose := strings.TrimSpace(item.DockerCompose)
	if item.DownloadURL != "" {
		update("downloading", "正在下载运行时归档")
		downloadDir := filepath.Join(root, "runtimes", ".downloads")
		if err := os.MkdirAll(downloadDir, 0o750); err != nil {
			update("failed", "创建下载目录失败: "+err.Error())
			return
		}
		archivePath := filepath.Join(downloadDir, item.ID+".tar.gz")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		err := downloadAppArchiveWithProgress(ctx, item.DownloadURL, archivePath, func(downloaded, total int64) {
			if total > 0 {
				update("downloading", fmt.Sprintf("正在下载运行时归档: %d%% (%s/%s)", downloaded*100/total, formatRuntimeBytes(downloaded), formatRuntimeBytes(total)))
				return
			}
			update("downloading", fmt.Sprintf("正在下载运行时归档: %s", formatRuntimeBytes(downloaded)))
		})
		cancel()
		if err != nil {
			update("failed", "下载运行时归档失败: "+err.Error())
			return
		}
		update("installing", "正在解压运行时归档")
		if err := deployRuntimeArchive(archivePath, installDir, item.Type, item.Version); err != nil {
			update("failed", "解压运行时归档失败: "+err.Error())
			return
		}
		_ = os.Remove(archivePath)
		packageCompose, err := os.ReadFile(filepath.Join(installDir, "docker-compose.yml"))
		if err != nil {
			update("failed", "运行时包缺少 docker-compose.yml: "+err.Error())
			return
		}
		compose = string(packageCompose)
	} else if err := os.MkdirAll(installDir, 0o750); err != nil {
		update("failed", "创建运行目录失败: "+err.Error())
		return
	}
	if compose == "" && item.Image != "" {
		compose = fmt.Sprintf("services:\n  runtime:\n    image: %s\n    container_name: %s\n", item.Image, item.Container)
		if item.Port > 0 {
			compose += fmt.Sprintf("    ports:\n      - \"%d:%d\"\n", item.Port, item.Port)
		}
	}
	if compose == "" {
		update("failed", "运行时未提供 Docker Compose 或镜像")
		return
	}
	composePath := filepath.Join(installDir, "docker-compose.yml")
	if item.DownloadURL == "" {
		if err := writeAtomicRuntimeFile(composePath, []byte(compose)); err != nil {
			update("failed", "写入 Compose 文件失败: "+err.Error())
			return
		}
	}
	if err := writeRuntimeComposeOverride(installDir, item); err != nil {
		update("failed", "写入 Compose 文件失败: "+err.Error())
		return
	}
	item.ComposePath, item.DockerCompose = composePath, compose
	env, envErr := runtimeEnvironment(item)
	if envErr != nil {
		update("failed", envErr.Error())
		return
	}
	switch normalizeRuntimeTypeFilter(item.Type) {
	case "php":
		phpVersion := runtimeString(item.Params, "PHP_VERSION")
		if phpVersion == "" {
			update("failed", "PHP_VERSION 不能为空")
			return
		}
		item.Image = "1panel-php-fpm:" + phpVersion
		env["IMAGE_NAME"] = item.Image
	}
	if err := writeRuntimeEnv(filepath.Join(installDir, ".env"), env); err != nil {
		update("failed", "写入环境变量失败: "+err.Error())
		return
	}
	s.mu.Lock()
	for i := range s.state.Runtimes {
		if s.state.Runtimes[i].ID == item.ID {
			s.state.Runtimes[i] = item
			_ = s.saveLocked()
			break
		}
	}
	s.mu.Unlock()
	if err := ensureRuntimeNetwork(s.commandExecutor(), "1panel-network"); err != nil {
		update("failed", "创建运行时网络失败: "+err.Error())
		return
	}
	if err := executeRuntimeInstall(s.commandExecutor(), item, func(status, message string) { update(status, message) }); err != nil {
		update("failed", err.Error())
		return
	}
	update("running", "运行时安装完成")
}

func deployRuntimeArchive(archivePath, installDir, runtimeType, version string) error {
	parent := filepath.Dir(installDir)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".runtime-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := extractTarGz(archivePath, stage); err != nil {
		return err
	}
	source := filepath.Join(stage, strings.ToLower(runtimeType), version)
	info, err := os.Stat(source)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("归档缺少 %s/%s 版本目录", strings.ToLower(runtimeType), version)
	}
	for _, required := range []string{"docker-compose.yml"} {
		entry, statErr := os.Stat(filepath.Join(source, required))
		if statErr != nil || !entry.Mode().IsRegular() {
			return fmt.Errorf("归档缺少 %s", required)
		}
	}
	backup := installDir + ".previous-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	hadExisting := false
	if _, err := os.Stat(installDir); err == nil {
		if err := os.Rename(installDir, backup); err != nil {
			return fmt.Errorf("备份旧运行时目录失败: %w", err)
		}
		hadExisting = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(source, installDir); err != nil {
		if hadExisting {
			_ = os.Rename(backup, installDir)
		}
		return fmt.Errorf("部署运行时目录失败: %w", err)
	}
	return nil
}

func runtimeComposeServiceName(runtimeType string) string {
	switch normalizeRuntimeTypeFilter(runtimeType) {
	case "go":
		return "golang"
	case "php", "java", "node", "python", "dotnet":
		return normalizeRuntimeTypeFilter(runtimeType)
	default:
		return "runtime"
	}
}

func writeRuntimeComposeOverride(installDir string, item runtimeRecord) error {
	serviceConfig := map[string]any{}
	ports := make([]string, 0, len(item.ExposedPorts))
	for _, raw := range item.ExposedPorts {
		entry, _ := raw.(map[string]any)
		hostPort, _ := runtimeNumberValue(entry["hostPort"])
		containerPort, _ := runtimeNumberValue(entry["containerPort"])
		hostIP := runtimeString(entry, "hostIP")
		if hostIP == "" {
			hostIP = "0.0.0.0"
		}
		protocol := strings.ToLower(runtimeString(entry, "protocol"))
		if protocol == "" {
			protocol = "tcp"
		}
		ports = append(ports, fmt.Sprintf("%s:%d:%d/%s", hostIP, hostPort, containerPort, protocol))
	}
	if len(ports) > 0 {
		serviceConfig["ports"] = ports
	}
	volumes := make([]string, 0, len(item.Volumes))
	for _, raw := range item.Volumes {
		entry, _ := raw.(map[string]any)
		mapping := runtimeString(entry, "source") + ":" + runtimeString(entry, "target")
		if mode := strings.ToLower(runtimeString(entry, "mode")); mode != "" {
			mapping += ":" + mode
		}
		volumes = append(volumes, mapping)
	}
	if len(volumes) > 0 {
		serviceConfig["volumes"] = volumes
	}
	extraHosts := make([]string, 0, len(item.ExtraHosts))
	for _, raw := range item.ExtraHosts {
		entry, _ := raw.(map[string]any)
		extraHosts = append(extraHosts, runtimeString(entry, "hostname", "host")+":"+runtimeString(entry, "ip"))
	}
	if len(extraHosts) > 0 {
		serviceConfig["extra_hosts"] = extraHosts
	}
	path := filepath.Join(installDir, "docker-compose.override.json")
	if len(serviceConfig) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	payload, err := json.MarshalIndent(map[string]any{"services": map[string]any{runtimeComposeServiceName(item.Type): serviceConfig}}, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicRuntimeFile(path, append(payload, '\n'))
}

func runtimeComposeArgs(item runtimeRecord, operation ...string) []string {
	args := []string{"compose", "-f", item.ComposePath}
	if override := filepath.Join(filepath.Dir(item.ComposePath), "docker-compose.override.json"); func() bool { _, err := os.Stat(override); return err == nil }() {
		args = append(args, "-f", override)
	}
	return append(args, operation...)
}

// runtimeCommandEnv 返回 Compose 解析所需的统一环境变量。
// Compose 在缺少变量时会把挂载源解析为空字符串，最终形成类似 `:/www/` 的非法规格；
// 因此生命周期操作也必须使用与创建/编辑相同的默认值。
func runtimeCommandEnv(item runtimeRecord) (map[string]string, error) {
	values, err := runtimeEnvironment(item)
	if err != nil {
		return nil, err
	}
	env := make(map[string]string, len(values))
	for key, value := range values {
		if validEnvKey(key) {
			env[key] = fmt.Sprint(value)
		}
	}
	return env, nil
}

func validateRuntimeCompose(executor runtimeCommandExecutor, item runtimeRecord, env map[string]string) error {
	args := runtimeComposeArgs(item, "config")
	_, err := runtimeCommandWithEnv(executor, item, 2*time.Minute, env, nil, args...)
	if err != nil {
		return fmt.Errorf("Docker Compose 配置校验失败: %w", err)
	}
	return nil
}

func runtimeCommand(executor runtimeCommandExecutor, item runtimeRecord, timeout time.Duration, args ...string) (model.CommandResult, error) {
	return runtimeCommandWithOutput(executor, item, timeout, nil, args...)
}

func runtimeCommandWithOutput(executor runtimeCommandExecutor, item runtimeRecord, timeout time.Duration, output func(string, []byte), args ...string) (model.CommandResult, error) {
	return runtimeCommandWithEnv(executor, item, timeout, nil, output, args...)
}

func runtimeCommandWithEnv(executor runtimeCommandExecutor, item runtimeRecord, timeout time.Duration, env map[string]string, output func(string, []byte), args ...string) (model.CommandResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := executor.Execute(ctx, model.CommandRequest{Program: service.DockerBinary(), Args: args, Dir: filepath.Dir(item.ComposePath), Env: env, Timeout: timeout, Output: output})
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = strings.TrimSpace(result.Stdout)
		}
		if message == "" && err != nil {
			message = err.Error()
		}
		if message == "" {
			message = fmt.Sprintf("命令退出码 %d", result.ExitCode)
		}
		return result, errors.New(message)
	}
	return result, nil
}

func ensureRuntimeNetwork(executor runtimeCommandExecutor, name string) error {
	probe := runtimeRecord{ComposePath: filepath.Join(os.TempDir(), "runtime-network-probe.yml")}
	if _, err := runtimeCommand(executor, probe, 30*time.Second, "network", "inspect", name); err == nil {
		return nil
	}
	_, err := runtimeCommand(executor, probe, 30*time.Second, "network", "create", name)
	return err
}

func executeRuntimeInstall(executor runtimeCommandExecutor, item runtimeRecord, update func(string, string)) error {
	run := func(status, message string, timeout time.Duration, args ...string) error {
		update(status, message)
		appendRuntimeBuildLog(item, "$ "+service.DockerBinary()+" "+strings.Join(args, " "))
		output := func(stream string, chunk []byte) {
			appendRuntimeBuildLog(item, "["+stream+"] "+string(chunk))
			if item.TaskID != "" {
				appendAppTaskLog(item.TaskID, string(chunk))
			}
		}
		result, err := runtimeCommandWithOutput(executor, item, timeout, output, args...)
		appendRuntimeBuildLog(item, result.Stdout)
		appendRuntimeBuildLog(item, result.Stderr)
		if strings.TrimSpace(result.Stdout) != "" {
			update(status, result.Stdout)
		}
		if err != nil {
			appendRuntimeBuildLog(item, "命令失败: "+err.Error())
			return fmt.Errorf("%s: %w", message, err)
		}
		return nil
	}
	if normalizeRuntimeTypeFilter(item.Type) != "php" {
		if err := run("pulling", "正在拉取运行时镜像", 30*time.Minute, runtimeComposeArgs(item, "pull")...); err != nil {
			return err
		}
		return run("starting", "正在启动运行时容器", 30*time.Minute, runtimeComposeArgs(item, "up", "-d")...)
	}
	if err := run("Building", "正在构建 PHP 运行时镜像", time.Hour, runtimeComposeArgs(item, "build")...); err != nil {
		return err
	}
	if err := run("Creating", "正在启动 PHP 构建容器", 30*time.Minute, runtimeComposeArgs(item, "up", "-d")...); err != nil {
		return err
	}
	extensions := strings.TrimSpace(runtimeString(item.Params, "PHP_EXTENSIONS"))
	if extensions != "" {
		if err := run("Building", "正在安装 PHP 扩展", time.Hour, "exec", "-i", item.Container, "install-ext", extensions); err != nil {
			return err
		}
		if err := run("Building", "正在提交 PHP 运行时镜像", 15*time.Minute, "commit", item.Container, item.Image); err != nil {
			return err
		}
	}
	if err := run("ReCreating", "正在停止 PHP 构建容器", 10*time.Minute, runtimeComposeArgs(item, "down")...); err != nil {
		return err
	}
	return run("ReCreating", "正在使用最终镜像启动 PHP 运行时", 30*time.Minute, runtimeComposeArgs(item, "up", "-d")...)
}

func appendRuntimeBuildLog(item runtimeRecord, output string) {
	if strings.TrimSpace(item.InstallPath) == "" || strings.TrimSpace(output) == "" {
		return
	}
	path := filepath.Join(item.InstallPath, "build.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	if info, statErr := file.Stat(); statErr == nil && info.Size() >= 8<<20 {
		return
	}
	_, _ = fmt.Fprintln(file, strings.TrimRight(output, "\r\n"))
}

func formatRuntimeBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	if value < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(value)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(value)/(1024*1024))
}

func writeAtomicRuntimeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeRuntimeEnv(path string, values map[string]any) error {
	lines := make([]string, 0, len(values))
	for key, value := range values {
		if !validEnvKey(key) {
			continue
		}
		lines = append(lines, key+"="+strings.ReplaceAll(fmt.Sprint(value), "\n", ""))
	}
	sort.Strings(lines)
	return writeAtomicRuntimeFile(path, []byte(strings.Join(lines, "\n")+"\n"))
}

func runtimeStatusForOperation(operation string) string {
	op := strings.ToLower(strings.TrimSpace(operation))
	switch op {
	case "停止":
		op = "stop"
	case "启动":
		op = "start"
	case "重启":
		op = "restart"
	}
	switch op {
	case "up", "start":
		return "Running"
	case "down", "stop":
		return "Stopped"
	case "restart":
		return "Running"
	default:
		return "Normal"
	}
}

func syncRuntimeContainerStatus(s *runtimeStore) error {
	s.mu.RLock()
	items := append([]runtimeRecord(nil), s.state.Runtimes...)
	s.mu.RUnlock()
	statuses := make(map[string]struct {
		status string
		err    string
	}, len(items))
	failures := make([]string, 0)
	for _, item := range items {
		if strings.TrimSpace(item.Container) == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		result, err := s.commandExecutor().Execute(ctx, model.CommandRequest{Program: service.DockerBinary(), Args: []string{"inspect", "--format", "{{.State.Status}}", item.Container}, Timeout: 20 * time.Second})
		cancel()
		value := struct{ status, err string }{status: "Stopped"}
		if err == nil && result.ExitCode == 0 {
			switch strings.ToLower(strings.TrimSpace(result.Stdout)) {
			case "running":
				value.status = "Running"
			case "restarting":
				value.status = "Restarting"
			case "created":
				value.status = "Creating"
			default:
				value.status = "Stopped"
			}
		} else {
			value.err = strings.TrimSpace(result.Stderr)
			if value.err == "" && err != nil {
				value.err = err.Error()
			}
			// 容器已被外部删除属于已停止状态，不应让同步接口整体失败。
			missing := strings.Contains(strings.ToLower(value.err), "no such object") || strings.Contains(strings.ToLower(value.err), "not found")
			if missing {
				value.status, value.err = "Stopped", ""
			} else {
				value.status = "Error"
				failures = append(failures, item.Name+": "+value.err)
			}
		}
		statuses[item.ID] = value
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for index := range s.state.Runtimes {
		value, exists := statuses[s.state.Runtimes[index].ID]
		if !exists {
			continue
		}
		if s.state.Runtimes[index].Status != value.status || s.state.Runtimes[index].Error != value.err || s.state.Runtimes[index].Message != value.err {
			s.state.Runtimes[index].Status = value.status
			s.state.Runtimes[index].Message = value.err
			s.state.Runtimes[index].Error = value.err
			s.state.Runtimes[index].UpdatedAt = time.Now().UTC()
			changed = true
		}
	}
	if changed {
		if err := s.saveLocked(); err != nil {
			return err
		}
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func operateRuntimeContainer(executor runtimeCommandExecutor, item runtimeRecord, operation string) error {
	op := strings.ToLower(strings.TrimSpace(operation))
	switch op {
	case "停止":
		op = "stop"
	case "启动":
		op = "start"
	case "重启":
		op = "restart"
	case "删除", "卸载":
		op = "down"
	}
	if op == "" {
		return errors.New("运行时操作不能为空")
	}
	if item.ComposePath != "" {
		if _, err := os.Stat(item.ComposePath); err == nil {
			env, err := runtimeCommandEnv(item)
			if err != nil {
				return err
			}
			if err := validateRuntimeCompose(executor, item, env); err != nil {
				return err
			}
			if op == "up" || op == "start" {
				op = "up"
			}
			args := runtimeComposeArgs(item, op)
			if op == "up" {
				args = append(args, "-d")
			}
			if _, err := runtimeCommandWithEnv(executor, item, 10*time.Minute, env, nil, args...); err != nil {
				return fmt.Errorf("Docker Compose 操作失败: %w", err)
			}
			return nil
		}
	}
	if strings.TrimSpace(item.Container) == "" {
		return errors.New("运行时缺少 Compose 文件和容器名")
	}
	dockerOp := op
	if dockerOp == "up" {
		dockerOp = "start"
	}
	_, err := runtimeCommand(executor, item, 10*time.Minute, dockerOp, item.Container)
	return err
}

func registerRuntimeRoutes(mux *http.ServeMux, s *runtimeStore) {
	list := func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		page, pageSize, err := runtimePage(body)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		typeFilter := strings.ToLower(runtimeString(body, "type"))
		nameFilter := strings.ToLower(runtimeString(body, "name"))
		statusFilter := strings.ToLower(runtimeString(body, "status"))
		s.mu.RLock()
		filtered := make([]runtimeRecord, 0, len(s.state.Runtimes))
		for _, item := range s.state.Runtimes {
			hydrateRuntimePaths(&item)
			if typeFilter != "" && typeFilter != "all" && !strings.EqualFold(item.Type, typeFilter) {
				continue
			}
			if nameFilter != "" && !strings.Contains(strings.ToLower(item.Name), nameFilter) {
				continue
			}
			if statusFilter != "" && statusFilter != "all" && !runtimeStatusMatches(item, statusFilter) {
				continue
			}
			filtered = append(filtered, item)
		}
		s.mu.RUnlock()
		sort.SliceStable(filtered, func(i, j int) bool {
			if filtered[i].Name == filtered[j].Name {
				return filtered[i].ID < filtered[j].ID
			}
			return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
		})
		total := len(filtered)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		items := filtered[start:end]
		if items == nil {
			items = []runtimeRecord{}
		}
		runtimeOK(w, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
	}
	mux.HandleFunc("GET /api/v2/runtimes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, v := range s.state.Runtimes {
			hydrateRuntimePaths(&v)
			if v.ID == id {
				runtimeOK(w, v)
				return
			}
		}
		runtimeErr(w, 404, "运行时不存在")
	})
	mux.HandleFunc("GET /api/v2/runtimes/installed/delete/check/{id}", func(w http.ResponseWriter, r *http.Request) {
		resources, err := runtimeWebsiteReferences(r.Context(), r.PathValue("id"))
		if err != nil {
			runtimeErr(w, http.StatusInternalServerError, "检查运行时引用失败: "+err.Error())
			return
		}
		runtimeOK(w, resources)
	})
	mux.HandleFunc("POST /api/v2/runtimes/search", list)
	mux.HandleFunc("POST /api/v2/runtimes/sync", func(w http.ResponseWriter, r *http.Request) {
		if _, err := runtimeBody(r); err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := syncRuntimeContainerStatus(s); err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		// 同步接口与 1Panel 一样返回最新列表，查询条件保持空对象。
		list(w, &http.Request{Body: http.NoBody})
	})
	mux.HandleFunc("POST /api/v2/runtimes", func(w http.ResponseWriter, r *http.Request) {
		v, e := runtimeBody(r)
		if e != nil {
			runtimeErr(w, 400, e.Error())
			return
		}
		item, err := runtimeRecordFromRequest(v)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := hydrateRuntimeFromAppStore(&item); err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if item.ID == "" {
			item.ID = item.Name
		}
		if item.ID == "" {
			runtimeErr(w, 400, "运行时名称不能为空")
			return
		}
		s.mu.Lock()
		if validationErr := validateRuntimeCreateLocked(s.state.Runtimes, item); validationErr != nil {
			s.mu.Unlock()
			runtimeErr(w, http.StatusConflict, validationErr.Error())
			return
		}
		install := runtimeInstallRequested(v, item)
		if install && normalizeRuntimeTypeFilter(item.Type) != "php" {
			if err := validateRuntimeCodeDirectory(item); err != nil {
				s.mu.Unlock()
				runtimeErr(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		if install {
			if strings.TrimSpace(item.TaskID) == "" {
				item.TaskID = idToken()
			}
			item.Status = "Creating"
			item.TaskStatus = "installing"
		} else {
			item.Status = "Running"
		}
		s.state.Runtimes = append(s.state.Runtimes, item)
		saveErr := s.saveLocked()
		s.mu.Unlock()
		if saveErr != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "运行时存储不可用: "+saveErr.Error())
			return
		}
		if install {
			// Create the task record and first log line before returning. This
			// lets the task drawer attach immediately and keeps installation
			// independent from the lifecycle of the creating page.
			ensureAppTaskLog(item.TaskID, item.ID, item.Name, "installing", "开始安装运行时")
			go runRuntimeInstallTask(s, item)
		}
		runtimeOK(w, item)
	})
	mux.HandleFunc("POST /api/v2/runtimes/operate", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, index := runtimeByID(s, runtimeString(body, "id", "runtimeId", "ID"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "运行时不存在")
			return
		}
		operation := runtimeString(body, "operation", "operate")
		if err := operateRuntimeContainer(s.commandExecutor(), item, operation); err != nil {
			s.mu.Lock()
			if index >= 0 && index < len(s.state.Runtimes) {
				s.state.Runtimes[index].Status = "Error"
				s.state.Runtimes[index].TaskStatus = "failed"
				s.state.Runtimes[index].Message = err.Error()
				s.state.Runtimes[index].Error = err.Error()
				s.state.Runtimes[index].UpdatedAt = time.Now().UTC()
				_ = s.saveLocked()
			}
			s.mu.Unlock()
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		s.mu.Lock()
		item.Status = runtimeStatusForOperation(operation)
		item.Message, item.Error, item.TaskStatus = "", "", ""
		item.UpdatedAt = time.Now().UTC()
		s.state.Runtimes[index] = item
		saveErr := s.saveLocked()
		s.mu.Unlock()
		if saveErr != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "运行时存储不可用: "+saveErr.Error())
			return
		}
		runtimeOK(w, item)
	})
	mux.HandleFunc("POST /api/v2/runtimes/remark", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, index := runtimeByID(s, runtimeString(body, "id", "runtimeId"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "运行时不存在")
			return
		}
		item.Remark = runtimeString(body, "remark")
		item.UpdatedAt = time.Now().UTC()
		s.mu.Lock()
		s.state.Runtimes[index] = item
		saveErr := s.saveLocked()
		s.mu.Unlock()
		if saveErr != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "运行时存储不可用: "+saveErr.Error())
			return
		}
		runtimeOK(w, item)
	})
	mux.HandleFunc("POST /api/v2/runtimes/update", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		current, index := runtimeByID(s, runtimeString(body, "id", "runtimeId"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "运行时不存在")
			return
		}
		updated, err := mergeRuntimeUpdate(current, body)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.mu.RLock()
		others := append([]runtimeRecord(nil), s.state.Runtimes[:index]...)
		others = append(others, s.state.Runtimes[index+1:]...)
		s.mu.RUnlock()
		if err := validateRuntimeCreateLocked(others, updated); err != nil {
			runtimeErr(w, http.StatusConflict, err.Error())
			return
		}
		rollbackPackage, err := refreshRuntimePackageIfNeeded(current, &updated)
		if err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		if err := applyRuntimeConfiguration(s.commandExecutor(), current, updated); err != nil {
			rollbackPackage()
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		updated.Status, updated.Message, updated.Error, updated.TaskStatus, updated.UpdatedAt = "Running", "", "", "", time.Now().UTC()
		s.mu.Lock()
		s.state.Runtimes[index] = updated
		saveErr := s.saveLocked()
		s.mu.Unlock()
		if saveErr != nil {
			runtimeErr(w, http.StatusServiceUnavailable, "运行时存储不可用: "+saveErr.Error())
			return
		}
		runtimeOK(w, updated)
	})
	mux.HandleFunc("POST /api/v2/runtimes/del", func(w http.ResponseWriter, r *http.Request) {
		v, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id := runtimeString(v, "id", "runtimeId")
		if id == "" {
			runtimeErr(w, http.StatusBadRequest, "运行时 ID 不能为空")
			return
		}
		resources, referenceErr := runtimeWebsiteReferences(r.Context(), id)
		if referenceErr != nil {
			runtimeErr(w, http.StatusInternalServerError, "检查运行时引用失败: "+referenceErr.Error())
			return
		}
		if len(resources) > 0 {
			runtimeErrData(w, http.StatusConflict, "运行时仍被网站引用", resources)
			return
		}
		forceDelete, _ := v["forceDelete"].(bool)
		deleteImage, _ := v["deleteImage"].(bool)
		taskID := runtimeString(v, "taskID", "taskId")
		s.mu.Lock()
		for i, x := range s.state.Runtimes {
			if x.ID == id {
				if x.ComposePath != "" || x.Container != "" {
					s.mu.Unlock()
					if err := operateRuntimeContainer(s.commandExecutor(), x, "down"); err != nil && !forceDelete {
						runtimeErr(w, http.StatusBadGateway, err.Error())
						return
					}
					if deleteImage && strings.TrimSpace(x.Image) != "" {
						if _, err := runtimeCommand(s.commandExecutor(), x, 10*time.Minute, "image", "rm", x.Image); err != nil && !forceDelete {
							runtimeErr(w, http.StatusBadGateway, "删除运行时镜像失败: "+err.Error())
							return
						}
					}
					if x.InstallPath != "" {
						if err := removeRuntimeInstallPath(x.InstallPath); err != nil && !forceDelete {
							runtimeErr(w, http.StatusInternalServerError, "删除运行时目录失败: "+err.Error())
							return
						}
					}
					s.mu.Lock()
				}
				s.state.Runtimes = append(s.state.Runtimes[:i], s.state.Runtimes[i+1:]...)
				saveErr := s.saveLocked()
				s.mu.Unlock()
				if saveErr != nil {
					runtimeErr(w, http.StatusServiceUnavailable, "运行时存储不可用: "+saveErr.Error())
					return
				}
				if taskID != "" {
					ensureAppTaskLog(taskID, x.ID, x.Name, "running", "运行时删除完成")
					appendRuntimeTaskLog(taskID, "运行时删除完成")
					appendRuntimeTaskLog(taskID, "[TASK-END]")
				}
				runtimeOK(w, nil)
				return
			}
		}
		s.mu.Unlock()
		runtimeErr(w, 404, "运行时不存在")
	})
	registerRuntimeSubroutes(mux, s)
}

func validateRuntimeCodeDirectory(item runtimeRecord) error {
	if normalizeRuntimeTypeFilter(item.Type) == "php" {
		return nil
	}
	if strings.TrimSpace(item.CodeDir) == "" {
		return errors.New("代码目录不能为空")
	}
	realCode, err := filepath.EvalSymlinks(item.CodeDir)
	if err != nil {
		return errors.New("代码目录不存在或无法访问")
	}
	info, err := os.Stat(realCode)
	if err != nil || !info.IsDir() {
		return errors.New("代码目录必须是目录")
	}
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	runtimeRoot, err := filepath.Abs(filepath.Join(root, "runtimes"))
	if err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(filepath.Dir(runtimeRoot))
	if err == nil {
		runtimeRoot = filepath.Join(realRoot, filepath.Base(runtimeRoot))
	}
	relative, err := filepath.Rel(runtimeRoot, realCode)
	if err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))) {
		return errors.New("代码目录不能位于运行时目录内")
	}
	return nil
}

func runtimeWebsiteReferences(ctx context.Context, runtimeID string) ([]map[string]any, error) {
	resources := []map[string]any{}
	if strings.TrimSpace(runtimeID) == "" || sharedDB() == nil {
		return resources, nil
	}
	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := sharedDB().QueryContext(queryCtx, `SELECT id,primary_domain FROM websites WHERE runtime_id=? ORDER BY id LIMIT 201`, runtimeID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return resources, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		if len(resources) >= 200 {
			return nil, errors.New("运行时网站引用超过 200 条上限")
		}
		var id int64
		var domain string
		if err := rows.Scan(&id, &domain); err != nil {
			return nil, err
		}
		resources = append(resources, map[string]any{"id": id, "name": domain, "type": "website"})
	}
	return resources, rows.Err()
}

func runtimeByID(s *runtimeStore, id string) (runtimeRecord, int) {
	if strings.TrimSpace(id) == "" {
		return runtimeRecord{}, -1
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for index, item := range s.state.Runtimes {
		if item.ID == id {
			// Older SQLite records predate persisted Compose/container fields.
			// Hydrate them at every operational entry point so stop/restart and
			// secondary PHP extension installs use the same canonical paths/env.
			hydrateRuntimePaths(&item)
			if item.Container == "" {
				item.Container = runtimeString(item.Params, "CONTAINER_NAME", "containerName")
			}
			if normalizeRuntimeTypeFilter(item.Type) == "php" && item.Image == "" {
				if version := runtimeString(item.Params, "PHP_VERSION"); version != "" {
					item.Image = "1panel-php-fpm:" + version
				}
			}
			return item, index
		}
	}
	return runtimeRecord{}, -1
}

func mergeRuntimeUpdate(current runtimeRecord, body map[string]any) (runtimeRecord, error) {
	updated := current
	if params, exists := body["params"]; exists {
		values, ok := params.(map[string]any)
		if !ok {
			return runtimeRecord{}, errors.New("params 必须是对象")
		}
		updated.Params = cloneRuntimeMap(values)
	}
	if source, exists := body["source"]; exists {
		value, ok := source.(string)
		if !ok {
			return runtimeRecord{}, errors.New("source 必须是字符串")
		}
		updated.Source = strings.TrimSpace(value)
		if updated.Source != "" {
			updated.Params["CONTAINER_PACKAGE_URL"] = updated.Source
		}
	}
	if install, ok := body["install"].(bool); ok && normalizeRuntimeTypeFilter(updated.Type) == "node" {
		if install {
			updated.Params["RUN_INSTALL"] = "1"
		} else {
			updated.Params["RUN_INSTALL"] = "0"
		}
	}
	if raw, exists := body["port"]; exists {
		port, ok := runtimeNumberValue(raw)
		if !ok || port < 1 || port > 65535 {
			return runtimeRecord{}, errors.New("端口必须在 1-65535 范围内")
		}
		updated.Port = port
	}
	for _, field := range []string{"exposedPorts", "environments", "volumes", "extraHosts"} {
		raw, exists := body[field]
		if !exists {
			continue
		}
		values, ok := raw.([]any)
		if !ok {
			return runtimeRecord{}, fmt.Errorf("%s 必须是数组", field)
		}
		switch field {
		case "exposedPorts":
			updated.ExposedPorts = values
		case "environments":
			updated.Environments = values
		case "volumes":
			updated.Volumes = values
		case "extraHosts":
			updated.ExtraHosts = values
		}
	}
	if value := runtimeString(updated.Params, "CONTAINER_NAME"); value != "" {
		if !validDockerIdentifier(value) {
			return runtimeRecord{}, errors.New("容器名无效")
		}
		updated.Container = value
	}
	if remark, exists := body["remark"]; exists {
		value, ok := remark.(string)
		if !ok {
			return runtimeRecord{}, errors.New("remark 必须是字符串")
		}
		updated.Remark = strings.TrimSpace(value)
	}
	if normalizeRuntimeTypeFilter(updated.Type) == "php" {
		extensions, err := normalizePHPExtensions(updated.Params["PHP_EXTENSIONS"])
		if err != nil {
			return runtimeRecord{}, err
		}
		updated.Params["PHP_EXTENSIONS"] = extensions
		updated.Extensions = []string{}
		if extensions != "" {
			updated.Extensions = strings.Split(extensions, ",")
		}
		phpVersion := runtimeString(updated.Params, "PHP_VERSION")
		if phpVersion == "" {
			return runtimeRecord{}, errors.New("PHP_VERSION 不能为空")
		}
		updated.Image = "1panel-php-fpm:" + phpVersion
	}
	if err := normalizeRuntimePorts(&updated, runtimeInstallRequested(body, updated)); err != nil {
		return runtimeRecord{}, err
	}
	if err := validateRuntimeCollections(updated); err != nil {
		return runtimeRecord{}, err
	}
	if err := validateRuntimeCreateLocked(nil, updated); err != nil {
		return runtimeRecord{}, err
	}
	return updated, nil
}

func runtimeEnvironment(item runtimeRecord) (map[string]any, error) {
	if err := normalizeRuntimePorts(&item, normalizeRuntimeTypeFilter(item.Type) != "php"); err != nil {
		return nil, err
	}
	values := cloneRuntimeMap(item.Params)
	for _, raw := range item.Environments {
		entry, _ := raw.(map[string]any)
		if key := runtimeString(entry, "key"); validEnvKey(key) {
			values[key] = fmt.Sprint(entry["value"])
		}
	}
	if appPort, ok := item.Params["APP_PORT"]; ok {
		values["APP_PORT"] = appPort
	}
	values["CONTAINER_NAME"] = item.Container
	if item.Port > 0 {
		values["PANEL_APP_PORT_HTTP"] = item.Port
	}
	values["CODE_DIR"] = item.CodeDir
	// Compose 使用这两个变量构造 PHP 及通用运行时挂载/时区；缺失时 Compose
	// 会把变量替换为空字符串，生成非法的 `:/www/` 挂载。
	websiteDirValue, websiteDirSet := values["PANEL_WEBSITE_DIR"]
	if !websiteDirSet || strings.TrimSpace(fmt.Sprint(websiteDirValue)) == "" {
		websiteDir := strings.TrimSpace(item.CodeDir)
		if websiteDir == "" {
			websiteDir = "/www/wwwroot"
		}
		values["PANEL_WEBSITE_DIR"] = websiteDir
	}
	tzValue, tzSet := values["TZ"]
	if !tzSet || strings.TrimSpace(fmt.Sprint(tzValue)) == "" {
		values["TZ"] = "Asia/Shanghai"
	}
	switch normalizeRuntimeTypeFilter(item.Type) {
	case "php":
		phpVersion := runtimeString(item.Params, "PHP_VERSION")
		if phpVersion == "" {
			phpVersion = strings.TrimSpace(item.Version)
		}
		if phpVersion != "" {
			values["PHP_VERSION"] = phpVersion
			values["IMAGE_NAME"] = "1panel-php-fpm:" + phpVersion
		}
	case "java":
		values["JAVA_VERSION"] = item.Version
	case "node":
		values["NODE_VERSION"] = item.Version
	case "go":
		values["GO_VERSION"] = item.Version
	case "python":
		values["PYTHON_VERSION"] = item.Version
	case "dotnet":
		values["DOTNET_VERSION"] = item.Version
	}
	return values, nil
}

func applyRuntimeConfiguration(executor runtimeCommandExecutor, current, updated runtimeRecord) error {
	if strings.TrimSpace(updated.ComposePath) == "" {
		return nil
	}
	installDir := filepath.Dir(updated.ComposePath)
	envPath := filepath.Join(installDir, ".env")
	overridePath := filepath.Join(installDir, "docker-compose.override.json")
	oldEnv, envErr := os.ReadFile(envPath)
	oldOverride, overrideErr := os.ReadFile(overridePath)
	rollbackFiles := func() {
		if envErr == nil {
			_ = writeAtomicRuntimeFile(envPath, oldEnv)
		}
		if overrideErr == nil {
			_ = writeAtomicRuntimeFile(overridePath, oldOverride)
		} else {
			_ = os.Remove(overridePath)
		}
	}
	values, err := runtimeEnvironment(updated)
	if err != nil {
		return err
	}
	if err := writeRuntimeEnv(envPath, values); err != nil {
		return fmt.Errorf("写入运行时环境变量失败: %w", err)
	}
	if err := writeRuntimeComposeOverride(installDir, updated); err != nil {
		rollbackFiles()
		return fmt.Errorf("写入运行时 Compose 覆盖配置失败: %w", err)
	}
	if normalizeRuntimeTypeFilter(updated.Type) == "php" {
		if err := executeRuntimeInstall(executor, updated, func(string, string) {}); err != nil {
			rollbackFiles()
			_ = operateRuntimeContainer(executor, current, "down")
			_ = operateRuntimeContainer(executor, current, "up")
			return fmt.Errorf("重建 PHP 运行时失败: %w", err)
		}
		return nil
	}
	if err := operateRuntimeContainer(executor, current, "down"); err != nil {
		rollbackFiles()
		return err
	}
	if err := operateRuntimeContainer(executor, updated, "up"); err != nil {
		rollbackFiles()
		_ = operateRuntimeContainer(executor, current, "up")
		return err
	}
	return nil
}

func hydrateRuntimeFromAppStore(item *runtimeRecord) error {
	if item == nil {
		return errors.New("运行时参数不能为空")
	}
	runtimeType := normalizeRuntimeTypeFilter(item.Type)
	if runtimeType != "php" && runtimeType != "go" && runtimeType != "java" && runtimeType != "node" && runtimeType != "python" && runtimeType != "dotnet" {
		return errors.New("运行时类型无效")
	}
	item.Type = runtimeType
	if item.AppDetailID == "" {
		if strings.EqualFold(item.Resource, "appstore") {
			return errors.New("应用详情 ID 不能为空")
		}
		return nil
	}
	store := getAppStore()
	store.mu.RLock()
	defer store.mu.RUnlock()
	for _, app := range store.state.Catalog {
		if !appMatchesType(app, runtimeType) {
			continue
		}
		if runtimeType == "php" && !strings.EqualFold(strings.TrimSpace(app.Key), "php") {
			continue
		}
		for _, version := range app.Versions {
			if !strings.EqualFold(strings.TrimSpace(version.ID), strings.TrimSpace(item.AppDetailID)) {
				continue
			}
			if item.Version == "" {
				item.Version = version.Version
			} else if !strings.EqualFold(item.Version, version.Version) {
				return errors.New("运行时版本与应用详情不匹配")
			}
			if item.DockerCompose == "" {
				item.DockerCompose = version.DockerCompose
				if item.DockerCompose == "" && version.ComposeURL != "" {
					item.DockerCompose = fetchRemoteCompose(version.ComposeURL)
				}
			}
			if item.Image == "" {
				if repository := appRuntimeImage(app, item.DockerCompose); repository != "" {
					item.Image = repository + ":" + item.Version
				}
			}
			if item.DownloadURL == "" {
				item.DownloadURL = version.DownloadURL
			}
			item.PackageModified = version.LastModified
			if item.AppID == "" {
				item.AppID = app.ID
			}
			if item.Resource == "" {
				item.Resource = "appstore"
			}
			return nil
		}
	}
	return errors.New("应用详情不存在或不属于所选运行时类型")
}

func removeRuntimeInstallPath(path string) error {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	base, err := filepath.Abs(filepath.Join(root, "runtimes"))
	if err != nil {
		return err
	}
	target, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("运行时目录不在数据目录内")
	}
	return os.RemoveAll(target)
}

func normalizePHPExtensions(raw any) (string, error) {
	values := make([]string, 0)
	switch value := raw.(type) {
	case nil:
		return "", nil
	case string:
		values = strings.Split(value, ",")
	case []string:
		values = append(values, value...)
	case []any:
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return "", errors.New("PHP 扩展必须是字符串数组")
			}
			values = append(values, text)
		}
	default:
		return "", errors.New("PHP 扩展格式无效")
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if !phpExtensionNamePattern.MatchString(value) {
			return "", fmt.Errorf("PHP 扩展名称无效: %s", value)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return strings.Join(result, ","), nil
}

func phpExtensionTemplatesLocked(s *runtimeStore) ([]phpExtensionTemplate, error) {
	raw, exists := s.state.Settings[phpExtensionTemplatesSetting]
	if !exists {
		return append([]phpExtensionTemplate(nil), defaultPHPExtensionTemplates...), nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var templates []phpExtensionTemplate
	if err := json.Unmarshal(payload, &templates); err != nil {
		return nil, fmt.Errorf("读取 PHP 扩展模板失败: %w", err)
	}
	if templates == nil {
		templates = []phpExtensionTemplate{}
	}
	return templates, nil
}

func registerPHPExtensionTemplateRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		name := strings.TrimSpace(runtimeString(body, "name"))
		extensions, err := normalizePHPExtensions(body["extensions"])
		if name == "" || len(name) > 64 || strings.ContainsAny(name, "\r\n\x00") {
			runtimeErr(w, http.StatusBadRequest, "扩展模板名称无效")
			return
		}
		if err != nil || extensions == "" {
			if err == nil {
				err = errors.New("扩展模板不能为空")
			}
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		templates, err := phpExtensionTemplatesLocked(s)
		if err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		nextID := 1
		for _, item := range templates {
			if strings.EqualFold(item.Name, name) {
				runtimeErr(w, http.StatusConflict, "扩展模板名称已存在")
				return
			}
			if item.ID >= nextID {
				nextID = item.ID + 1
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		templates = append(templates, phpExtensionTemplate{ID: nextID, Name: name, Extensions: extensions, CreatedAt: now, UpdatedAt: now})
		s.state.Settings[phpExtensionTemplatesSetting] = templates
		if err := s.saveLocked(); err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		runtimeOK(w, templates[len(templates)-1])
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/update", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id, idErr := strconv.Atoi(runtimeString(body, "id"))
		extensions, extensionErr := normalizePHPExtensions(body["extensions"])
		if idErr != nil || id < 1 || extensionErr != nil || extensions == "" {
			if extensionErr != nil {
				runtimeErr(w, http.StatusBadRequest, extensionErr.Error())
			} else {
				runtimeErr(w, http.StatusBadRequest, "扩展模板参数无效")
			}
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		templates, err := phpExtensionTemplatesLocked(s)
		if err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		found := false
		for index := range templates {
			if templates[index].ID == id {
				templates[index].Extensions = extensions
				templates[index].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
				found = true
				break
			}
		}
		if !found {
			runtimeErr(w, http.StatusNotFound, "扩展模板不存在")
			return
		}
		s.state.Settings[phpExtensionTemplatesSetting] = templates
		if err := s.saveLocked(); err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		runtimeOK(w, nil)
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/del", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		id, idErr := strconv.Atoi(runtimeString(body, "id"))
		if idErr != nil || id < 1 {
			runtimeErr(w, http.StatusBadRequest, "扩展模板 ID 无效")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		templates, err := phpExtensionTemplatesLocked(s)
		if err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		kept := make([]phpExtensionTemplate, 0, len(templates))
		found := false
		for _, item := range templates {
			if item.ID == id {
				found = true
				continue
			}
			kept = append(kept, item)
		}
		if !found {
			runtimeErr(w, http.StatusNotFound, "扩展模板不存在")
			return
		}
		s.state.Settings[phpExtensionTemplatesSetting] = kept
		if err := s.saveLocked(); err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		runtimeOK(w, nil)
	})
}

func registerPHPExtensionOperationRoutes(mux *http.ServeMux, s *runtimeStore) {
	for _, route := range []struct {
		path    string
		install bool
	}{
		{path: "/api/v2/runtimes/php/extensions/install", install: true},
		{path: "/api/v2/runtimes/php/extensions/uninstall", install: false},
	} {
		route := route
		mux.HandleFunc("POST "+route.path, func(w http.ResponseWriter, r *http.Request) {
			body, err := runtimeBody(r)
			if err != nil {
				runtimeErr(w, http.StatusBadRequest, err.Error())
				return
			}
			id, extension := runtimeString(body, "id", "runtimeId"), strings.ToLower(runtimeString(body, "name", "extension"))
			if !phpExtensionNamePattern.MatchString(extension) {
				runtimeErr(w, http.StatusBadRequest, "PHP 扩展名称无效")
				return
			}
			item, index := runtimeByID(s, id)
			if index < 0 || normalizeRuntimeTypeFilter(item.Type) != "php" {
				runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
				return
			}
			definition := phpExtensionDefinitionForName(extension)
			extensions := updatedPHPExtensionNames(item.Extensions, definition, route.install)
			updated := item
			updated.Extensions = extensions
			updated.Params = cloneRuntimeMap(item.Params)
			updated.Params["PHP_EXTENSIONS"] = strings.Join(extensions, ",")
			taskID := runtimeString(body, "taskID", "taskId")
			if taskID == "" {
				taskID = idToken()
			}
			startMessage := "开始更新 PHP 扩展 " + extension
			ensureAppTaskLog(taskID, item.ID, item.Name, "installing", startMessage)
			run := func() error {
				if route.install {
					if err := installPHPExtensionWithOutput(s.commandExecutor(), item, definition.Name, func(stream string, data []byte) {
						appendRuntimeTaskOutput(taskID, stream, data)
					}); err != nil {
						return err
					}
					values, err := runtimeEnvironment(updated)
					if err != nil {
						return err
					}
					if strings.TrimSpace(updated.InstallPath) == "" {
						return errors.New("PHP 运行时安装目录不存在")
					}
					if err := writeRuntimeEnv(filepath.Join(updated.InstallPath, ".env"), values); err != nil {
						return fmt.Errorf("保存 PHP 扩展环境变量失败: %w", err)
					}
					return nil
				}
				return uninstallPHPExtension(s.commandExecutor(), item, updated, definition)
			}
			finish := func() error {
				s.mu.Lock()
				defer s.mu.Unlock()
				updated.Status, updated.Message, updated.Error, updated.TaskStatus, updated.UpdatedAt = "Running", "", "", "success", time.Now().UTC()
				for runtimeIndex := range s.state.Runtimes {
					if s.state.Runtimes[runtimeIndex].ID == updated.ID {
						s.state.Runtimes[runtimeIndex] = updated
						return s.saveLocked()
					}
				}
				return errors.New("PHP 运行时已被删除")
			}
			if !route.install {
				if err := run(); err != nil {
					runtimeErr(w, http.StatusBadGateway, err.Error())
					return
				}
				if err := finish(); err != nil {
					runtimeErr(w, http.StatusInternalServerError, err.Error())
					return
				}
				runtimeOK(w, map[string]any{"id": id, "extension": extension, "status": "success"})
				return
			}
			go func() {
				appendRuntimeTaskLog(taskID, startMessage)
				if err := run(); err != nil {
					updateRuntimeTask(s, item.ID, "failed", err.Error())
					ensureAppTaskLog(taskID, item.ID, item.Name, "failed", err.Error())
					appendRuntimeTaskLog(taskID, err.Error())
					appendRuntimeTaskLog(taskID, "[TASK-END]")
					return
				}
				if err := finish(); err != nil {
					updateRuntimeTask(s, item.ID, "failed", err.Error())
					ensureAppTaskLog(taskID, item.ID, item.Name, "failed", err.Error())
					appendRuntimeTaskLog(taskID, err.Error())
					appendRuntimeTaskLog(taskID, "[TASK-END]")
					return
				}
				ensureAppTaskLog(taskID, item.ID, item.Name, "running", "PHP 扩展更新完成")
				appendRuntimeTaskLog(taskID, "PHP 扩展更新完成")
				appendRuntimeTaskLog(taskID, "[TASK-END]")
			}()
			wmhttp.JSON(w, http.StatusAccepted, map[string]any{"code": 200, "data": map[string]any{"id": id, "extension": extension, "taskID": taskID, "status": "queued"}})
		})
	}
}

func phpExtensionDefinitionForName(name string) phpExtensionDefinition {
	for _, definition := range phpExtensionCatalog {
		if strings.EqualFold(definition.Name, name) {
			return definition
		}
	}
	return phpExtensionDefinition{Name: strings.ToLower(name), Check: strings.ToLower(name), File: strings.ToLower(name) + ".so"}
}

func updatedPHPExtensionNames(current []string, definition phpExtensionDefinition, install bool) []string {
	values := make(map[string]string, len(current)+1)
	for _, value := range current {
		value = strings.TrimSpace(value)
		if value != "" {
			values[strings.ToLower(value)] = value
		}
	}
	delete(values, strings.ToLower(definition.Name))
	delete(values, strings.ToLower(definition.Check))
	if install {
		values[strings.ToLower(definition.Name)] = definition.Name
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i]) < strings.ToLower(result[j]) })
	return result
}

func installPHPExtension(executor runtimeCommandExecutor, item runtimeRecord, extension string) error {
	return installPHPExtensionWithOutput(executor, item, extension, nil)
}

func installPHPExtensionWithOutput(executor runtimeCommandExecutor, item runtimeRecord, extension string, output func(string, []byte)) error {
	if strings.TrimSpace(item.Container) == "" || strings.TrimSpace(item.Image) == "" {
		return errors.New("PHP 运行时缺少容器名或目标镜像")
	}
	if _, err := runtimeCommandWithOutput(executor, item, time.Hour, output, "exec", "-i", item.Container, "install-ext", extension); err != nil {
		return fmt.Errorf("安装 PHP 扩展失败: %w", err)
	}
	if _, err := runtimeCommand(executor, item, 15*time.Minute, "commit", item.Container, item.Image); err != nil {
		return fmt.Errorf("提交 PHP 扩展镜像失败: %w", err)
	}
	if err := restartRuntimeContainer(executor, item); err != nil {
		return fmt.Errorf("重启 PHP 运行时失败: %w", err)
	}
	return nil
}

func restartRuntimeContainer(executor runtimeCommandExecutor, item runtimeRecord) error {
	if err := operateRuntimeContainer(executor, item, "down"); err != nil {
		return err
	}
	return operateRuntimeContainer(executor, item, "up")
}

func uninstallPHPExtension(executor runtimeCommandExecutor, current, updated runtimeRecord, definition phpExtensionDefinition) error {
	extensionDir, err := phpRuntimeExtensionDirectory(current)
	if err != nil {
		return err
	}
	tx, err := service.PreparePHPExtensionRemoval(service.PHPExtensionRemoval{
		RuntimeDir: current.InstallPath, ExtensionDir: extensionDir, Name: definition.Name,
		ModuleFile: definition.File, Extensions: updated.Extensions,
	})
	if err != nil {
		return fmt.Errorf("准备卸载 PHP 扩展失败: %w", err)
	}
	if err := restartRuntimeContainer(executor, current); err != nil {
		rollbackErr := tx.Rollback()
		_ = restartRuntimeContainer(executor, current)
		if rollbackErr != nil {
			return fmt.Errorf("卸载 PHP 扩展后重启失败，且文件回滚失败: %v: %w", rollbackErr, err)
		}
		return fmt.Errorf("卸载 PHP 扩展后重启失败，已恢复旧文件: %w", err)
	}
	tx.Commit()
	return nil
}

func phpRuntimeExtensionDirectory(item runtimeRecord) (string, error) {
	if strings.TrimSpace(item.InstallPath) == "" {
		return "", errors.New("PHP 运行时尚未完成安装")
	}
	if info, err := os.Stat(item.InstallPath); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("不是目录")
		}
		return "", fmt.Errorf("PHP 运行时目录不存在: %w", err)
	}
	root := filepath.Join(item.InstallPath, "extensions")
	if value := strings.TrimSpace(runtimeString(item.Params, "EXTENSION_DIR")); value != "" {
		name := filepath.Base(filepath.Clean(value))
		if strings.HasPrefix(name, "no-debug-non-zts-") {
			candidate := filepath.Join(root, name)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate, nil
			}
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("读取 PHP 扩展目录失败: %w", err)
	}
	candidates := make([]string, 0, 1)
	for _, entry := range entries {
		if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && strings.HasPrefix(entry.Name(), "no-debug-non-zts-") {
			candidates = append(candidates, filepath.Join(root, entry.Name()))
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) == 0 {
		return root, nil
	}
	return "", errors.New("PHP 扩展目录不唯一，请检查 EXTENSION_DIR")
}

func phpRuntimeConfigPath(item runtimeRecord, kind string) (string, error) {
	if normalizeRuntimeTypeFilter(item.Type) != "php" || strings.TrimSpace(item.InstallPath) == "" {
		return "", errors.New("PHP 运行时尚未完成安装")
	}
	var name string
	switch kind {
	case "php", "config":
		name = "php.ini"
	case "fpm":
		name = "php-fpm.conf"
	default:
		return "", errors.New("PHP 配置文件类型无效")
	}
	path := filepath.Join(item.InstallPath, "conf", name)
	root, _ := filepath.Abs(item.InstallPath)
	target, _ := filepath.Abs(path)
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("PHP 配置文件路径越界")
	}
	return target, nil
}

func readRuntimeConfigFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("PHP 配置文件不存在")
	}
	if info.Size() > 2<<20 {
		return nil, errors.New("PHP 配置文件超过 2 MiB 限制")
	}
	return os.ReadFile(path)
}

func parseRuntimeINI(content string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found && strings.TrimSpace(key) != "" {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"")
		}
	}
	return values
}

var phpINIKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,127}$`)

func updateRuntimeINI(content string, updates map[string]string) (string, error) {
	for key, value := range updates {
		if !phpINIKeyPattern.MatchString(key) || strings.ContainsAny(value, "\r\n\x00") {
			return "", fmt.Errorf("PHP 配置项无效: %s", key)
		}
	}
	seen := map[string]bool{}
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, found := strings.Cut(trimmed, "=")
		key = strings.TrimSpace(key)
		value, exists := updates[key]
		if found && exists {
			lines[index] = key + " = " + value
			seen[key] = true
		}
	}
	keys := make([]string, 0, len(updates))
	for key := range updates {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, key+" = "+updates[key])
	}
	return strings.Join(lines, "\n"), nil
}

func updatePHPFileAndRestart(executor runtimeCommandExecutor, item runtimeRecord, path string, content []byte) error {
	old, err := readRuntimeConfigFile(path)
	if err != nil {
		return err
	}
	if len(content) > 2<<20 || bytes.IndexByte(content, 0) >= 0 {
		return errors.New("PHP 配置内容无效或超过 2 MiB 限制")
	}
	if err := writeAtomicRuntimeFile(path, content); err != nil {
		return err
	}
	if err := operateRuntimeContainer(executor, item, "restart"); err != nil {
		_ = writeAtomicRuntimeFile(path, old)
		_ = operateRuntimeContainer(executor, item, "restart")
		return fmt.Errorf("应用 PHP 配置失败，已恢复旧文件: %w", err)
	}
	return nil
}

func registerPHPConfigurationRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("GET /api/v2/runtimes/php/config/{id}", func(w http.ResponseWriter, r *http.Request) {
		item, index := runtimeByID(s, r.PathValue("id"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		path, err := phpRuntimeConfigPath(item, "php")
		if err != nil {
			runtimeErr(w, http.StatusConflict, err.Error())
			return
		}
		content, err := readRuntimeConfigFile(path)
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		params := parseRuntimeINI(string(content))
		disabled := []string{}
		for _, value := range strings.Split(params["disable_functions"], ",") {
			if value = strings.TrimSpace(value); value != "" {
				disabled = append(disabled, value)
			}
		}
		runtimeOK(w, map[string]any{"id": item.ID, "params": params, "disableFunctions": disabled, "uploadMaxSize": params["upload_max_filesize"], "maxExecutionTime": params["max_execution_time"]})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/config", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, index := runtimeByID(s, runtimeString(body, "id", "runtimeId"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		path, err := phpRuntimeConfigPath(item, "php")
		if err != nil {
			runtimeErr(w, http.StatusConflict, err.Error())
			return
		}
		old, err := readRuntimeConfigFile(path)
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		updates := map[string]string{}
		if params, ok := body["params"].(map[string]any); ok {
			for key, value := range params {
				updates[key] = fmt.Sprint(value)
			}
		}
		if raw, exists := body["disableFunctions"]; exists {
			values, ok := raw.([]any)
			if !ok {
				runtimeErr(w, http.StatusBadRequest, "disableFunctions 必须是数组")
				return
			}
			functions := make([]string, 0, len(values))
			for _, value := range values {
				name, ok := value.(string)
				if !ok || !phpINIKeyPattern.MatchString(name) {
					runtimeErr(w, http.StatusBadRequest, "禁用函数名称无效")
					return
				}
				functions = append(functions, name)
			}
			updates["disable_functions"] = strings.Join(functions, ",")
		}
		if value := runtimeString(body, "uploadMaxSize"); value != "" {
			updates["upload_max_filesize"] = value
		}
		if value := runtimeString(body, "maxExecutionTime"); value != "" {
			updates["max_execution_time"] = value
		}
		content, err := updateRuntimeINI(string(old), updates)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := updatePHPFileAndRestart(s.commandExecutor(), item, path, []byte(content)); err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		runtimeOK(w, nil)
	})
	for _, kind := range []string{"php", "fpm"} {
		kind := kind
		mux.HandleFunc("GET /api/v2/runtimes/php/"+kind+"/file/{id}", func(w http.ResponseWriter, r *http.Request) {
			item, index := runtimeByID(s, r.PathValue("id"))
			if index < 0 {
				runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
				return
			}
			path, err := phpRuntimeConfigPath(item, kind)
			if err != nil {
				runtimeErr(w, http.StatusConflict, err.Error())
				return
			}
			content, err := readRuntimeConfigFile(path)
			if err != nil {
				runtimeErr(w, http.StatusNotFound, err.Error())
				return
			}
			runtimeOK(w, map[string]any{"name": filepath.Base(path), "path": path, "content": string(content)})
		})
	}
	mux.HandleFunc("POST /api/v2/runtimes/php/file", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, index := runtimeByID(s, runtimeString(body, "id", "runtimeId"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		path, err := phpRuntimeConfigPath(item, runtimeString(body, "type"))
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		content, err := readRuntimeConfigFile(path)
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		runtimeOK(w, map[string]any{"name": filepath.Base(path), "path": path, "content": string(content)})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/update", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, index := runtimeByID(s, runtimeString(body, "id", "runtimeId"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		path, err := phpRuntimeConfigPath(item, runtimeString(body, "type"))
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		content, ok := body["content"].(string)
		if !ok {
			runtimeErr(w, http.StatusBadRequest, "PHP 配置内容不能为空")
			return
		}
		if err := updatePHPFileAndRestart(s.commandExecutor(), item, path, []byte(content)); err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		runtimeOK(w, nil)
	})
	mux.HandleFunc("GET /api/v2/runtimes/php/fpm/config/{id}", func(w http.ResponseWriter, r *http.Request) {
		item, index := runtimeByID(s, r.PathValue("id"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		path, err := phpRuntimeConfigPath(item, "fpm")
		if err != nil {
			runtimeErr(w, http.StatusConflict, err.Error())
			return
		}
		content, err := readRuntimeConfigFile(path)
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		runtimeOK(w, map[string]any{"id": item.ID, "params": parseRuntimeINI(string(content))})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/fpm/config", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		item, index := runtimeByID(s, runtimeString(body, "id", "runtimeId"))
		if index < 0 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		params, ok := body["params"].(map[string]any)
		if !ok {
			runtimeErr(w, http.StatusBadRequest, "FPM params 必须是对象")
			return
		}
		path, err := phpRuntimeConfigPath(item, "fpm")
		if err != nil {
			runtimeErr(w, http.StatusConflict, err.Error())
			return
		}
		old, err := readRuntimeConfigFile(path)
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		updates := make(map[string]string, len(params))
		for key, value := range params {
			updates[key] = fmt.Sprint(value)
		}
		content, err := updateRuntimeINI(string(old), updates)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := updatePHPFileAndRestart(s.commandExecutor(), item, path, []byte(content)); err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		runtimeOK(w, nil)
	})
	mux.HandleFunc("GET /api/v2/runtimes/php/fpm/status/{id}", func(w http.ResponseWriter, r *http.Request) {
		item, index := runtimeByID(s, r.PathValue("id"))
		if index < 0 || normalizeRuntimeTypeFilter(item.Type) != "php" || item.Port < 1 || item.Port > 65535 {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在或尚未启动")
			return
		}
		status, err := readFastCGIStatus(net.JoinHostPort("127.0.0.1", strconv.Itoa(item.Port)), 10*time.Second)
		if err != nil {
			runtimeErr(w, http.StatusBadGateway, "读取 PHP-FPM 状态失败: "+err.Error())
			return
		}
		runtimeOK(w, status)
	})
	mux.HandleFunc("GET /api/v2/runtimes/php/container/{id}", func(w http.ResponseWriter, r *http.Request) {
		item, index := runtimeByID(s, r.PathValue("id"))
		if index < 0 || normalizeRuntimeTypeFilter(item.Type) != "php" {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		runtimeOK(w, map[string]any{"id": item.ID, "containerName": item.Container, "exposedPorts": item.ExposedPorts, "environments": item.Environments, "volumes": item.Volumes, "extraHosts": item.ExtraHosts})
	})
	mux.HandleFunc("POST /api/v2/runtimes/php/container/update", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		current, index := runtimeByID(s, runtimeString(body, "id", "runtimeId"))
		if index < 0 || normalizeRuntimeTypeFilter(current.Type) != "php" {
			runtimeErr(w, http.StatusNotFound, "PHP 运行时不存在")
			return
		}
		if name := runtimeString(body, "containerName", "container"); name != "" {
			if current.Params == nil {
				current.Params = map[string]any{}
			}
			body["params"] = cloneRuntimeMap(current.Params)
			body["params"].(map[string]any)["CONTAINER_NAME"] = name
		}
		updated, err := mergeRuntimeUpdate(current, body)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		s.mu.RLock()
		others := append([]runtimeRecord(nil), s.state.Runtimes[:index]...)
		others = append(others, s.state.Runtimes[index+1:]...)
		s.mu.RUnlock()
		if err := validateRuntimeCreateLocked(others, updated); err != nil {
			runtimeErr(w, http.StatusConflict, err.Error())
			return
		}
		if err := applyRuntimeConfiguration(s.commandExecutor(), current, updated); err != nil {
			runtimeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		updated.Status, updated.Message, updated.Error, updated.UpdatedAt = "Running", "", "", time.Now().UTC()
		s.mu.Lock()
		s.state.Runtimes[index] = updated
		saveErr := s.saveLocked()
		s.mu.Unlock()
		if saveErr != nil {
			runtimeErr(w, http.StatusServiceUnavailable, saveErr.Error())
			return
		}
		runtimeOK(w, updated)
	})
}

func registerRuntimeSubroutes(mux *http.ServeMux, s *runtimeStore) {
	registerNodeRuntimeRoutes(mux, s)
	registerPHPExtensionOperationRoutes(mux, s)
	registerPHPConfigurationRoutes(mux, s)
	registerPHPSupervisorRoutes(mux, s)
	registerPHPExtensionTemplateRoutes(mux, s)
	mux.HandleFunc("POST /api/v2/runtimes/php/extensions/search", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		name := strings.ToLower(runtimeString(body, "name", "search"))
		s.mu.RLock()
		templates, templateErr := phpExtensionTemplatesLocked(s)
		s.mu.RUnlock()
		if templateErr != nil {
			runtimeErr(w, http.StatusInternalServerError, templateErr.Error())
			return
		}
		items := make([]phpExtensionTemplate, 0, len(templates))
		for _, template := range templates {
			if name != "" && !strings.Contains(strings.ToLower(template.Name), name) {
				continue
			}
			items = append(items, template)
		}
		page, pageSize, pageErr := runtimePage(body)
		if pageErr != nil {
			runtimeErr(w, http.StatusBadRequest, pageErr.Error())
			return
		}
		total := len(items)
		start := (page - 1) * pageSize
		if start > total {
			start = total
		}
		end := start + pageSize
		if end > total {
			end = total
		}
		if all, _ := body["all"].(bool); all {
			start, end = 0, total
		}
		if items == nil {
			items = []phpExtensionTemplate{}
		}
		runtimeOK(w, map[string]any{"items": items[start:end], "total": total, "page": page, "pageSize": pageSize})
	})
	mux.HandleFunc("/api/v2/runtimes/php/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/runtimes/php/"), "/"), "/")
		if len(parts) >= 2 && parts[0] != "" {
			id := parts[0]
			s.mu.RLock()
			var rec *runtimeRecord
			for i := range s.state.Runtimes {
				if s.state.Runtimes[i].ID == id {
					copy := s.state.Runtimes[i]
					rec = &copy
					break
				}
			}
			s.mu.RUnlock()
			if rec == nil {
				runtimeErr(w, 404, "runtime not found")
				return
			}
			if parts[1] == "extensions" {
				exts := append([]string(nil), rec.Extensions...)
				if strings.TrimSpace(rec.Container) != "" {
					var err error
					exts, err = phpInstalledExtensions(s.commandExecutor(), *rec)
					if err != nil {
						runtimeErr(w, http.StatusBadGateway, "读取 PHP 扩展失败: "+err.Error())
						return
					}
				}
				installed := make(map[string]bool, len(exts))
				for _, extension := range exts {
					installed[strings.ToLower(strings.TrimSpace(extension))] = true
				}
				support := make([]map[string]any, 0, len(phpExtensionCatalog))
				for _, definition := range phpExtensionCatalog {
					support = append(support, definition.toMap(installed[strings.ToLower(definition.Check)]))
				}
				runtimeOK(w, map[string]any{"id": id, "extensions": exts, "supportExtensions": support, "total": len(exts), "status": rec.Status})
				return
			}
		}
		runtimeErr(w, 404, "运行时路径不存在")
	})
}

func phpInstalledExtensions(executor runtimeCommandExecutor, item runtimeRecord) ([]string, error) {
	result, err := runtimeCommand(executor, item, 20*time.Second, "exec", "-i", item.Container, "php", "-m")
	if err != nil {
		return nil, err
	}
	seen := map[string]string{}
	for _, line := range strings.Split(result.Stdout, "\n") {
		value := strings.TrimSpace(line)
		if value == "" || value == "[PHP Modules]" || value == "[Zend Modules]" {
			continue
		}
		seen[strings.ToLower(value)] = value
	}
	items := make([]string, 0, len(seen))
	for _, value := range seen {
		items = append(items, value)
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i]) < strings.ToLower(items[j]) })
	return items, nil
}

// registerNodeRuntimeRoutes 注册 Node.js 包脚本、模块查询和受限模块操作接口。
func registerNodeRuntimeRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("POST /api/v2/runtimes/node/package", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		dir, err := validateNodeRuntimeDirectory(runtimeString(body, "codeDir"))
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		manifest, err := readNodePackage(filepath.Join(dir, "package.json"))
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		keys := make([]string, 0, len(manifest.Scripts))
		for name := range manifest.Scripts {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		items := make([]map[string]string, 0, len(keys))
		for _, name := range keys {
			items = append(items, map[string]string{"name": name, "script": manifest.Scripts[name]})
		}
		runtimeOK(w, items)
	})
	mux.HandleFunc("POST /api/v2/runtimes/node/modules", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		record, err := findNodeRuntime(s, runtimeRequestID(body))
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		dir, err := validateNodeRuntimeDirectory(record.CodeDir)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		items, err := scanNodeModules(filepath.Join(dir, "node_modules"), 500)
		if err != nil {
			runtimeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		runtimeOK(w, items)
	})
	mux.HandleFunc("POST /api/v2/runtimes/node/modules/operate", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		record, err := findNodeRuntime(s, runtimeRequestID(body))
		if err != nil {
			runtimeErr(w, http.StatusNotFound, err.Error())
			return
		}
		operation := strings.ToLower(runtimeString(body, "operate", "operation"))
		manager := strings.ToLower(runtimeString(body, "pkgManager", "packageManager"))
		module := strings.ToLower(runtimeString(body, "module", "name"))
		if operation != "install" && operation != "uninstall" && operation != "update" {
			runtimeErr(w, http.StatusBadRequest, "Node 模块操作必须是 install、uninstall 或 update")
			return
		}
		if manager != "npm" && manager != "yarn" && manager != "pnpm" {
			runtimeErr(w, http.StatusBadRequest, "Node 包管理器只允许 npm、yarn 或 pnpm")
			return
		}
		if manager == "pnpm" && nodeRuntimeMajor(record.Version) < 18 {
			runtimeErr(w, http.StatusBadRequest, "Node 18 以下版本不支持 pnpm")
			return
		}
		if module != "" && !nodeModuleNamePattern.MatchString(module) {
			runtimeErr(w, http.StatusBadRequest, "Node 模块名称无效")
			return
		}
		if module == "" && operation != "update" {
			runtimeErr(w, http.StatusBadRequest, "安装或卸载时模块名称不能为空")
			return
		}
		if strings.TrimSpace(record.Container) == "" {
			runtimeErr(w, http.StatusConflict, "Node 运行时尚未关联容器")
			return
		}
		taskID := "node-module-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
		task := map[string]any{"id": taskID, "runtimeID": record.ID, "operation": operation, "module": module, "packageManager": manager, "status": "queued", "createdAt": time.Now().UTC()}
		s.mu.Lock()
		s.state.Settings["node-task:"+taskID] = task
		_ = s.saveLocked()
		s.mu.Unlock()
		go runNodeModuleTask(s, taskID, s.commandExecutor(), record, manager, operation, module)
		wmhttp.JSON(w, http.StatusAccepted, map[string]any{"code": 200, "data": task})
	})
	mux.HandleFunc("GET /api/v2/runtimes/node/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		task := s.state.Settings["node-task:"+r.PathValue("id")]
		s.mu.RUnlock()
		if task == nil {
			runtimeErr(w, http.StatusNotFound, "Node 模块任务不存在")
			return
		}
		runtimeOK(w, task)
	})
}

type nodePackageManifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	License     string            `json:"license"`
	Description string            `json:"description"`
	Scripts     map[string]string `json:"scripts"`
}

func runtimeRequestID(body map[string]any) string {
	if value := runtimeString(body, "id", "runtimeId", "ID"); value != "" {
		return value
	}
	for _, key := range []string{"id", "runtimeId", "ID"} {
		if value, ok := body[key].(float64); ok && value > 0 && value == float64(uint64(value)) {
			return strconv.FormatUint(uint64(value), 10)
		}
	}
	return ""
}

func findNodeRuntime(s *runtimeStore, id string) (runtimeRecord, error) {
	if id == "" {
		return runtimeRecord{}, errors.New("运行时 ID 不能为空")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.state.Runtimes {
		if item.ID == id {
			return item, nil
		}
	}
	return runtimeRecord{}, errors.New("运行时不存在")
}

func validateNodeRuntimeDirectory(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("Node 运行时工作目录不能为空")
	}
	dir, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return "", errors.New("Node 运行时工作目录无效")
	}
	if root := strings.TrimSpace(os.Getenv("WORKMESH_WORKSPACE_ROOT")); root != "" {
		rootAbs, rootErr := filepath.Abs(filepath.Clean(root))
		if rootErr != nil {
			return "", errors.New("WORKMESH_WORKSPACE_ROOT 配置无效")
		}
		relative, relErr := filepath.Rel(rootAbs, dir)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", errors.New("Node 运行时工作目录超出允许范围")
		}
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", errors.New("Node 运行时工作目录不存在")
	}
	return dir, nil
}

func readNodePackage(path string) (nodePackageManifest, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nodePackageManifest{}, errors.New("package.json 不存在")
	}
	if info.Size() > 2<<20 {
		return nodePackageManifest{}, errors.New("package.json 超过 2 MiB 限制")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nodePackageManifest{}, errors.New("读取 package.json 失败")
	}
	var manifest nodePackageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nodePackageManifest{}, errors.New("package.json 格式无效")
	}
	if manifest.Scripts == nil {
		manifest.Scripts = map[string]string{}
	}
	return manifest, nil
}

func scanNodeModules(root string, limit int) ([]nodePackageManifest, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []nodePackageManifest{}, nil
	}
	if err != nil {
		return nil, errors.New("读取 node_modules 失败")
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		base := filepath.Join(root, entry.Name())
		if strings.HasPrefix(entry.Name(), "@") {
			scoped, scopedErr := os.ReadDir(base)
			if scopedErr != nil {
				continue
			}
			for _, child := range scoped {
				if child.IsDir() {
					paths = append(paths, filepath.Join(base, child.Name(), "package.json"))
				}
			}
		} else {
			paths = append(paths, filepath.Join(base, "package.json"))
		}
	}
	sort.Strings(paths)
	items := make([]nodePackageManifest, 0, len(paths))
	for _, path := range paths {
		if len(items) >= limit {
			break
		}
		manifest, readErr := readNodePackage(path)
		if readErr == nil {
			manifest.Scripts = nil
			items = append(items, manifest)
		}
	}
	return items, nil
}

func nodeRuntimeMajor(version string) int {
	major, _ := strconv.Atoi(strings.SplitN(strings.TrimSpace(version), ".", 2)[0])
	return major
}

func runNodeModuleTask(s *runtimeStore, taskID string, executor runtimeCommandExecutor, record runtimeRecord, manager, operation, module string) {
	update := func(status, message string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		value, _ := s.state.Settings["node-task:"+taskID].(map[string]any)
		if value == nil {
			return
		}
		value["status"] = status
		value["updatedAt"] = time.Now().UTC()
		if message != "" {
			value["error"] = message
		}
		s.state.Settings["node-task:"+taskID] = value
		_ = s.saveLocked()
	}
	update("running", "")
	command := operation
	if manager == "yarn" {
		switch operation {
		case "install":
			command = "add"
		case "uninstall":
			command = "remove"
		case "update":
			command = "upgrade"
		}
	} else if manager == "pnpm" {
		switch operation {
		case "install":
			command = "add"
		case "uninstall":
			command = "remove"
		}
	}
	args := []string{command}
	if module != "" {
		args = append(args, module)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	requestArgs := append([]string{"exec", "-i", record.Container, manager}, args...)
	result, err := executor.Execute(ctx, model.CommandRequest{Program: service.DockerBinary(), Args: requestArgs, Timeout: 20 * time.Minute})
	if err != nil || result.ExitCode != 0 {
		message := strings.TrimSpace(result.Stderr)
		if message == "" {
			message = strings.TrimSpace(result.Stdout)
		}
		if len(message) > 512 {
			message = message[:512]
		}
		if message == "" && err != nil {
			message = err.Error()
		}
		update("failed", message)
		return
	}
	update("completed", "")
}

func registerTerminalRoutes(mux *http.ServeMux) {
	for _, p := range []string{"/api/v2/hosts/terminal/local", "/api/v2/hosts/terminal/container", "/api/v2/hosts/terminal/ssh"} {
		mux.HandleFunc("GET "+p, handleTerminalStream)
	}
}

func registerSSHRoutes(mux *http.ServeMux, s *runtimeStore) {
	get := func(w http.ResponseWriter, _ *http.Request) {
		var v map[string]any
		if !loadNodeSetting("ssh", &v) || v == nil {
			v = map[string]any{}
		}
		delete(v, "password")
		delete(v, "privateKey")
		delete(v, "passPhrase")
		runtimeOK(w, v)
	}
	mux.HandleFunc("GET /api/v2/settings/ssh/conn", get)
	mux.HandleFunc("POST /api/v2/settings/ssh", func(w http.ResponseWriter, r *http.Request) {
		v, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, "解析 SSH 配置失败: "+err.Error())
			return
		}
		if err := validateSSHConfig(v); err != nil {
			runtimeErr(w, 400, err.Error())
			return
		}
		if err := saveNodeSetting("ssh", v); err != nil {
			runtimeErr(w, 500, "保存 SSH 配置失败: "+err.Error())
			return
		}
		out := cloneMapRuntime(v)
		delete(out, "password")
		delete(out, "privateKey")
		delete(out, "passPhrase")
		runtimeOK(w, map[string]any{"config": out})
	})
	mux.HandleFunc("POST /api/v2/settings/ssh/default", func(w http.ResponseWriter, r *http.Request) {
		v, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, "解析默认连接配置失败: "+err.Error())
			return
		}
		if err := saveNodeSetting("ssh.default", v); err != nil {
			runtimeErr(w, 500, "保存默认连接配置失败: "+err.Error())
			return
		}
		runtimeOK(w, v)
	})
	mux.HandleFunc("POST /api/v2/settings/ssh/check/info", func(w http.ResponseWriter, r *http.Request) {
		v, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, "解析 SSH 测试参数失败: "+err.Error())
			return
		}
		runtimeSSHCheck(w, v)
	})
	mux.HandleFunc("POST /api/v2/settings/ssh/check", func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		if !loadNodeSetting("ssh", &v) {
			runtimeErr(w, 400, "尚未配置 SSH 连接")
			return
		}
		runtimeSSHCheck(w, v)
	})
}

func cloneMapRuntime(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
func validateSSHConfig(v map[string]any) error {
	host := runtimeString(v, "host", "addr", "address")
	if host == "" {
		return errors.New("SSH 主机地址不能为空")
	}
	port := runtimeIntValue(v["port"])
	if port == 0 {
		port = 22
	}
	if port < 1 || port > 65535 {
		return errors.New("SSH 端口无效")
	}
	return nil
}
func runtimeSSHCheck(w http.ResponseWriter, v map[string]any) {
	if err := validateSSHConfig(v); err != nil {
		runtimeErr(w, 400, err.Error())
		return
	}
	host := runtimeString(v, "host", "addr", "address")
	port := runtimeIntValue(v["port"])
	if port == 0 {
		port = 22
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	result := map[string]any{"host": host, "port": port, "connected": err == nil}
	if conn != nil {
		_ = conn.Close()
	}
	if err != nil {
		result["error"] = err.Error()
		runtimeErrData(w, http.StatusBadGateway, "SSH 连接失败", result)
		return
	}
	runtimeOK(w, result)
}

func runtimeIntValue(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	default:
		return 0
	}
}

func registerToolboxRoutes(mux *http.ServeMux, s *runtimeStore) {
	// 显式注册查询路由，便于契约扫描和文档准确发现每个功能。
	mux.HandleFunc("GET /api/v2/toolbox/device/users", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, toolboxGetData(s, "/api/v2/toolbox/device/users"))
	})
	mux.HandleFunc("GET /api/v2/toolbox/device/zone/options", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, toolboxGetData(s, "/api/v2/toolbox/device/zone/options"))
	})
	mux.HandleFunc("GET /api/v2/toolbox/fail2ban/base", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, toolboxGetData(s, "/api/v2/toolbox/fail2ban/base"))
	})
	mux.HandleFunc("GET /api/v2/toolbox/fail2ban/load/conf", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, toolboxGetData(s, "/api/v2/toolbox/fail2ban/load/conf"))
	})
	mux.HandleFunc("GET /api/v2/toolbox/ftp/base", func(w http.ResponseWriter, _ *http.Request) {
		runtimeOK(w, toolboxGetData(s, "/api/v2/toolbox/ftp/base"))
	})
	registerToolboxDeviceRoutes(mux, s)
	registerToolboxFail2BanRoutes(mux, s)
	registerToolboxFtpRoutes(mux, s)
	for _, p := range []string{"/api/v2/toolbox/clam", "/api/v2/toolbox/clam/base", "/api/v2/toolbox/clam/del", "/api/v2/toolbox/clam/file/search", "/api/v2/toolbox/clam/file/update", "/api/v2/toolbox/clam/handle", "/api/v2/toolbox/clam/operate", "/api/v2/toolbox/clam/record/clean", "/api/v2/toolbox/clam/record/search", "/api/v2/toolbox/clam/search", "/api/v2/toolbox/clam/status/update", "/api/v2/toolbox/clam/update", "/api/v2/toolbox/clean", "/api/v2/toolbox/scan", "/api/v2/settings/terminal/ai/search", "/api/v2/settings/terminal/ai/update"} {
		mux.HandleFunc("POST "+p, func(w http.ResponseWriter, r *http.Request) {
			v, _ := runtimeBody(r)
			runtimeOK(w, map[string]any{"status": "accepted", "config": v})
		})
	}
	_ = s
}

// registerToolboxDeviceRoutes 注册设备信息、主机配置和 DNS 探测。
func registerToolboxDeviceRoutes(mux *http.ServeMux, s *runtimeStore) {
	mux.HandleFunc("POST /api/v2/toolbox/device/base", func(w http.ResponseWriter, _ *http.Request) {
		host, _ := os.Hostname()
		runtimeOK(w, map[string]any{"hostname": host, "os": runtime.GOOS, "arch": runtime.GOARCH, "status": "ready"})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/check/dns", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, "解析 DNS 请求失败: "+err.Error())
			return
		}
		host := runtimeString(body, "host", "domain")
		if host == "" || len(host) > 253 || strings.ContainsAny(host, "/\\ ") {
			runtimeErr(w, 400, "DNS 主机名无效")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		ips, lookupErr := net.DefaultResolver.LookupHost(ctx, host)
		if lookupErr != nil {
			runtimeErr(w, http.StatusBadGateway, "DNS 查询失败: "+lookupErr.Error())
			return
		}
		runtimeOK(w, map[string]any{"host": host, "addresses": ips, "resolved": true})
	})
	mux.HandleFunc("POST /api/v2/toolbox/device/conf", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		value := s.state.Settings["device"]
		s.mu.RUnlock()
		if value == nil {
			value = map[string]any{}
		}
		runtimeOK(w, value)
	})
	for _, path := range []string{"/api/v2/toolbox/device/update/byconf", "/api/v2/toolbox/device/update/conf", "/api/v2/toolbox/device/update/host", "/api/v2/toolbox/device/update/swap"} {
		key := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v2/toolbox/device/update/"), "/")
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			body, err := runtimeBody(r)
			if err != nil {
				runtimeErr(w, 400, "解析设备配置失败: "+err.Error())
				return
			}
			if key == "host" {
				name := runtimeString(body, "hostname", "host")
				if name == "" || len(name) > 253 || strings.ContainsAny(name, " /\\") {
					runtimeErr(w, 400, "主机名无效")
					return
				}
			}
			if key == "swap" {
				if value, ok := body["size"].(float64); ok && (value < 0 || value > 1<<40) {
					runtimeErr(w, 400, "交换分区大小超出范围")
					return
				}
			}
			delete(body, "password")
			delete(body, "passwd")
			s.mu.Lock()
			s.state.Settings["device"] = body
			saveErr := s.saveLocked()
			s.mu.Unlock()
			if saveErr != nil {
				runtimeErr(w, 500, "保存设备配置失败: "+saveErr.Error())
				return
			}
			runtimeOK(w, map[string]any{"updated": true, "scope": key, "config": body})
		})
	}
	// 密码更新只确认已接收，不把敏感字段写入状态或日志。
	mux.HandleFunc("POST /api/v2/toolbox/device/update/passwd", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, "解析密码请求失败: "+err.Error())
			return
		}
		if runtimeString(body, "password", "passwd") == "" {
			runtimeErr(w, 400, "密码不能为空")
			return
		}
		runtimeOK(w, map[string]any{"updated": true, "sensitive": true})
	})
}

// registerToolboxFail2BanRoutes 注册 Fail2ban 配置读取与受限服务操作。
func registerToolboxFail2BanRoutes(mux *http.ServeMux, s *runtimeStore) {
	configPath := func() string {
		if value := strings.TrimSpace(os.Getenv("WORKMESH_FAIL2BAN_CONFIG")); value != "" {
			return filepath.Clean(value)
		}
		return filepath.Join(filepath.Dir(s.path), "fail2ban.local")
	}
	readConfig := func() (string, error) {
		value, err := os.ReadFile(configPath())
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if len(value) > 1<<20 {
			return "", errors.New("Fail2ban 配置超过 1 MiB 限制")
		}
		return string(value), nil
	}
	mux.HandleFunc("POST /api/v2/toolbox/fail2ban/search", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, err.Error())
			return
		}
		content, err := readConfig()
		if err != nil {
			runtimeErr(w, 500, "读取 Fail2ban 配置失败: "+err.Error())
			return
		}
		keyword := runtimeString(body, "keyword", "name")
		lines := make([]string, 0, 100)
		for _, line := range strings.Split(content, "\n") {
			if keyword == "" || strings.Contains(strings.ToLower(line), strings.ToLower(keyword)) {
				if strings.TrimSpace(line) != "" {
					lines = append(lines, line)
				}
				if len(lines) >= 100 {
					break
				}
			}
		}
		runtimeOK(w, map[string]any{"items": lines, "total": len(lines), "path": configPath()})
	})
	for _, path := range []string{"/api/v2/toolbox/fail2ban/update", "/api/v2/toolbox/fail2ban/update/byconf"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			body, err := runtimeBody(r)
			if err != nil {
				runtimeErr(w, 400, err.Error())
				return
			}
			content := runtimeString(body, "content", "conf")
			if content == "" || len(content) > 1<<20 {
				runtimeErr(w, 400, "Fail2ban 配置内容无效")
				return
			}
			file := configPath()
			if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
				runtimeErr(w, 500, "创建 Fail2ban 配置目录失败: "+err.Error())
				return
			}
			tmp := file + ".tmp"
			if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
				runtimeErr(w, 500, "写入 Fail2ban 配置失败: "+err.Error())
				return
			}
			if err := os.Rename(tmp, file); err != nil {
				_ = os.Remove(tmp)
				runtimeErr(w, 500, "替换 Fail2ban 配置失败: "+err.Error())
				return
			}
			runtimeOK(w, map[string]any{"updated": true, "path": file})
		})
	}
	for _, path := range []string{"/api/v2/toolbox/fail2ban/operate", "/api/v2/toolbox/fail2ban/operate/sshd"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			body, err := runtimeBody(r)
			if err != nil {
				runtimeErr(w, 400, err.Error())
				return
			}
			action := strings.ToLower(runtimeString(body, "operate", "action"))
			if action != "start" && action != "stop" && action != "restart" {
				runtimeErr(w, 400, "Fail2ban 操作必须是 start、stop 或 restart")
				return
			}
			binary, lookErr := exec.LookPath("fail2ban-client")
			if lookErr != nil {
				runtimeErr(w, http.StatusServiceUnavailable, "fail2ban-client 未安装")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, action)
			output, runErr := cmd.CombinedOutput()
			if runErr != nil {
				runtimeErr(w, http.StatusBadGateway, "执行 Fail2ban 操作失败: "+strings.TrimSpace(string(output)))
				return
			}
			runtimeOK(w, map[string]any{"operation": action, "output": strings.TrimSpace(string(output))})
		})
	}
}

// registerToolboxFtpRoutes 管理本地 FTP 连接配置，密码永不回传。
func registerToolboxFtpRoutes(mux *http.ServeMux, s *runtimeStore) {
	// FTP 日志写入共享运行时状态，限制最多保留 1000 条，避免无界增长。
	appendLog := func(action, id, detail string) {
		record := map[string]any{"id": "ftp-log-" + strconv.FormatInt(time.Now().UnixNano(), 10), "action": action, "ftpId": id, "detail": detail, "createdAt": time.Now().UTC().Format(time.RFC3339Nano)}
		value, _ := s.state.Settings["ftp.logs"].([]any)
		value = append(value, record)
		if len(value) > 1000 {
			value = value[len(value)-1000:]
		}
		s.state.Settings["ftp.logs"] = value
	}
	entries := func() []map[string]any {
		value, _ := s.state.Settings["ftp.entries"].([]any)
		result := make([]map[string]any, 0, len(value))
		for _, item := range value {
			if typed, ok := item.(map[string]any); ok {
				copy := map[string]any{}
				for key, val := range typed {
					if key != "password" {
						copy[key] = val
					}
				}
				result = append(result, copy)
			}
		}
		return result
	}
	mux.HandleFunc("POST /api/v2/toolbox/ftp/search", func(w http.ResponseWriter, r *http.Request) {
		body, _ := runtimeBody(r)
		keyword := strings.ToLower(runtimeString(body, "keyword", "name", "host"))
		s.mu.RLock()
		all := entries()
		s.mu.RUnlock()
		filtered := make([]map[string]any, 0, len(all))
		for _, item := range all {
			if keyword == "" || strings.Contains(strings.ToLower(fmt.Sprint(item["name"])+" "+fmt.Sprint(item["host"])), keyword) {
				filtered = append(filtered, item)
			}
		}
		runtimeOK(w, pageRecordsGeneric(filtered, body))
	})
	for _, path := range []string{"/api/v2/toolbox/ftp", "/api/v2/toolbox/ftp/update"} {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			body, err := runtimeBody(r)
			if err != nil {
				runtimeErr(w, 400, err.Error())
				return
			}
			host := runtimeString(body, "host", "hostname")
			user := runtimeString(body, "username", "user")
			if host == "" || user == "" || len(host) > 253 || strings.ContainsAny(host, " /\\") {
				runtimeErr(w, 400, "FTP 主机和用户名不能为空")
				return
			}
			body["host"], body["username"] = host, user
			delete(body, "password")
			s.mu.Lock()
			value, _ := s.state.Settings["ftp.entries"].([]any)
			id := runtimeString(body, "id")
			if id == "" {
				id = "ftp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
				body["id"] = id
				value = append(value, body)
			} else {
				found := false
				for i, item := range value {
					if typed, ok := item.(map[string]any); ok && fmt.Sprint(typed["id"]) == id {
						value[i] = body
						found = true
					}
				}
				if !found {
					value = append(value, body)
				}
			}
			s.state.Settings["ftp.entries"] = value
			logs, _ := s.state.Settings["ftp.logs"].([]any)
			logs = append(logs, map[string]any{"id": "ftp-log-" + strconv.FormatInt(time.Now().UnixNano(), 10), "action": path[strings.LastIndex(path, "/")+1:], "resourceId": id, "createdAt": time.Now().UTC()})
			if len(logs) > 1000 {
				logs = logs[len(logs)-1000:]
			}
			s.state.Settings["ftp.logs"] = logs
			saveErr := s.saveLocked()
			s.mu.Unlock()
			if saveErr != nil {
				runtimeErr(w, 500, "保存 FTP 配置失败: "+saveErr.Error())
				return
			}
			s.mu.Lock()
			appendLog("create_or_update", id, host)
			_ = s.saveLocked()
			s.mu.Unlock()
			runtimeOK(w, body)
		})
	}
	mux.HandleFunc("POST /api/v2/toolbox/ftp/del", func(w http.ResponseWriter, r *http.Request) {
		body, _ := runtimeBody(r)
		id := runtimeString(body, "id", "ftpId")
		if id == "" {
			runtimeErr(w, 400, "FTP 配置 ID 不能为空")
			return
		}
		s.mu.Lock()
		value, _ := s.state.Settings["ftp.entries"].([]any)
		out := make([]any, 0, len(value))
		found := false
		for _, item := range value {
			record, ok := item.(map[string]any)
			if ok && fmt.Sprint(record["id"]) == id {
				found = true
				continue
			}
			out = append(out, item)
		}
		s.state.Settings["ftp.entries"] = out
		if found {
			logs, _ := s.state.Settings["ftp.logs"].([]any)
			logs = append(logs, map[string]any{"id": "ftp-log-" + strconv.FormatInt(time.Now().UnixNano(), 10), "action": "delete", "resourceId": id, "createdAt": time.Now().UTC()})
			if len(logs) > 1000 {
				logs = logs[len(logs)-1000:]
			}
			s.state.Settings["ftp.logs"] = logs
		}
		saveErr := s.saveLocked()
		s.mu.Unlock()
		if !found {
			runtimeErr(w, 404, "FTP 配置不存在")
			return
		}
		if saveErr != nil {
			runtimeErr(w, 500, "保存 FTP 配置失败: "+saveErr.Error())
			return
		}
		s.mu.Lock()
		appendLog("delete", id, "")
		_ = s.saveLocked()
		s.mu.Unlock()
		runtimeOK(w, map[string]any{"id": id, "deleted": true})
	})
	mux.HandleFunc("POST /api/v2/toolbox/ftp/operate", func(w http.ResponseWriter, r *http.Request) {
		body, _ := runtimeBody(r)
		op := strings.ToLower(runtimeString(body, "operate", "operation"))
		if op != "connect" && op != "disconnect" && op != "test" {
			runtimeErr(w, 400, "FTP 操作无效")
			return
		}
		s.mu.Lock()
		appendLog(op, runtimeString(body, "id", "ftpId"), "client_operation")
		_ = s.saveLocked()
		s.mu.Unlock()
		runtimeOK(w, map[string]any{"operation": op, "status": "not_connected", "message": "FTP 连接需由已配置的客户端执行"})
	})
	mux.HandleFunc("POST /api/v2/toolbox/ftp/sync", func(w http.ResponseWriter, r *http.Request) {
		body, _ := runtimeBody(r)
		runtimeOK(w, map[string]any{"status": "queued", "id": runtimeString(body, "id", "ftpId")})
	})
	mux.HandleFunc("POST /api/v2/toolbox/ftp/log/search", func(w http.ResponseWriter, r *http.Request) {
		body, err := runtimeBody(r)
		if err != nil {
			runtimeErr(w, 400, "解析 FTP 日志查询失败: "+err.Error())
			return
		}
		keyword := strings.ToLower(runtimeString(body, "keyword", "action", "resourceId"))
		s.mu.RLock()
		stored, _ := s.state.Settings["ftp.logs"].([]any)
		logs := make([]map[string]any, 0, len(stored))
		for _, item := range stored {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if keyword != "" && !strings.Contains(strings.ToLower(fmt.Sprint(record["action"])+" "+fmt.Sprint(record["resourceId"])), keyword) {
				continue
			}
			logs = append(logs, cloneRuntimeMap(record))
		}
		s.mu.RUnlock()
		runtimeOK(w, pageRecordsGeneric(logs, body))
	})
}

func pageRecordsGeneric(items []map[string]any, body map[string]any) map[string]any {
	page, size := 1, 100
	if n, ok := body["page"].(float64); ok && n >= 1 {
		page = int(n)
	}
	if n, ok := body["pageSize"].(float64); ok && n >= 1 {
		size = int(n)
	}
	if size > 500 {
		size = 500
	}
	start := (page - 1) * size
	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	return map[string]any{"items": items[start:end], "total": len(items), "page": page, "pageSize": size}
}

func cloneRuntimeMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// toolboxGetData 从受限系统文件和本地状态读取工具箱信息，不执行用户输入命令。
func toolboxGetData(s *runtimeStore, path string) map[string]any {
	switch path {
	case "/api/v2/toolbox/device/users":
		items := make([]map[string]any, 0)
		if raw, err := os.ReadFile("/etc/passwd"); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				fields := strings.SplitN(line, ":", 7)
				if len(fields) < 7 || fields[0] == "" {
					continue
				}
				items = append(items, map[string]any{"name": fields[0], "uid": fields[2], "gid": fields[3], "home": fields[5], "shell": fields[6]})
				if len(items) >= 200 {
					break
				}
			}
		}
		return map[string]any{"items": items, "total": len(items), "status": "ready", "supported": len(items) > 0}
	case "/api/v2/toolbox/device/zone/options":
		zone := time.Local.String()
		if zone == "" {
			zone = "Local"
		}
		return map[string]any{"items": []map[string]any{{"name": zone, "value": zone}}, "current": zone, "status": "ready"}
	case "/api/v2/toolbox/fail2ban/base", "/api/v2/toolbox/fail2ban/load/conf":
		configPath := "/etc/fail2ban/jail.local"
		content := ""
		if raw, err := os.ReadFile(configPath); err == nil {
			content = string(raw)
			if len(content) > 1<<20 {
				content = content[:1<<20]
			}
		}
		return map[string]any{"path": configPath, "content": content, "installed": content != "", "enabled": content != "", "status": "ready"}
	case "/api/v2/toolbox/ftp/base":
		s.mu.RLock()
		value := s.state.Settings["ftp"]
		s.mu.RUnlock()
		if value == nil {
			value = map[string]any{"enabled": false, "port": 21, "status": "not_configured"}
		}
		if config, ok := value.(map[string]any); ok {
			return config
		}
		return map[string]any{"config": value, "status": "ready"}
	default:
		return map[string]any{"status": "unsupported", "path": path}
	}
}
