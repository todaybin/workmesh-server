// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"regexp"
	"time"
)

var nodeModuleNamePattern = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`)
var phpExtensionNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
var supervisorProcessNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var supervisorUserPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)

// runtimeRecord 是运行时及其扩展的最小持久化模型。
type runtimeRecord struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	// Mode controls how the runtime is managed. "host" uses the system
	// Supervisor service; empty/"docker" keeps the existing Compose behavior.
	Mode            string         `json:"mode,omitempty"`
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
