<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 2026-09-03 站点类型切换与运行时引用

状态：`[x] 已完成`

- 运行时 Compose 生命周期统一注入 `PANEL_WEBSITE_DIR` 与 `TZ=Asia/Shanghai`，并在 stop/start/restart 前执行 `docker compose config` 校验。
- 网站模型、SQLite 关系表、创建/更新/搜索请求统一使用字符串 `runtimeID`；历史数字引用转换为文本，非运行时站点自动清空引用。
- 建站表单切换类型时清理运行时字段，提交请求按类型裁剪字段，运行时 ID 强制转为非空字符串；隐藏字段不再参与校验。
- `/api/v2/settings/website/dir` 返回真实字符串路径；缺失容器同步状态记为 `Stopped` 而不使接口整体失败。

验证：`GOWORK=off GOCACHE=.cache/go-build go test -run '^$' ./...`、定向 Go 回归测试、`go vet`、`npm run type-check`、`npm run build:pro` 均通过。完整 Go 测试仍受沙箱禁止绑定本机临时端口影响。
