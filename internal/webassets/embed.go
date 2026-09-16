// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

// Package webassets 提供编译进 WorkMesh Server 的前端资源。
package webassets

import (
	"embed"
	"io/fs"
)

// Embedded 保存由前端构建生成的发布资源。
//
// dist 目录在开发环境只保留占位文件；正式构建由 web/package.json 先生成
// 完整资源，再由 go build 将其编译进 workmesh-server 二进制。
//
//go:embed all:dist
var Embedded embed.FS

// Dist 是前端发布目录的根文件系统。
var Dist fs.FS = mustSub(Embedded, "dist")

func mustSub(source fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(source, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
