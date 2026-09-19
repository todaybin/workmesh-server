<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# znmp 应用目录隔离

- [>] 进行中：停止为网站创建 `config/basic` 兼容磁盘目录；基础设置 API 与页面功能保持不变。
- [ ] 待验证：网站 API 定向测试、完整 Go 测试和 `go vet ./...`。
- [ ] 待部署：重新构建并激活 `/opt/workmesh-server` 公共管理面板，确认不改变其独立运行目录和职责。
- [ ] 待生产验收：通过 SQLite 权威状态切换 znmp 静态根和路径级反代，归档并注销 `sp.sopvip.com`。

本任务不将 WorkMesh Server 移入任何站点应用目录，也不引入 Gateway 私有代码依赖。生产操作必须先备份 SQLite、二进制和 OpenResty 配置，并保留回滚材料。

