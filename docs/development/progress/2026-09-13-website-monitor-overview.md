<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 网站监控概览与菜单层级（2026-09-13）

状态：`[>]` 前端实现和本地构建完成，浏览器像素验收与生产激活受环境阻断

## 已完成

- 网站监控顶部导航固定为概览、访问统计、趋势统计、请求日志、网站列表、设置六项。
- WAF 保留概览、攻击报表、拦截记录、封锁记录、黑白名单、网站设置、全局设置七项顶部导航。
- 网站监控和 WAF 的子路由加入 `hideInSidebar`，高级侧栏只显示网站监控、WAF、多机管理三个二级入口；父入口分别重定向到概览。
- 网站监控概览按 `docs/img/999.png` 重做：提示条、网站选择器、8 项今日状态、30 日访客地图、地域数量表、实时请求数/流量和访客趋势。
- 概览数据通过 WorkMesh 自有网站列表、监控统计、地域统计、QPS 和配置接口加载；开关保存使用现有监控配置接口，保留空数据与加载状态。
- 生产前端构建、Go 发布构建和发布校验文件已更新。

## 验证

- `npm run type-check`：通过。
- `npm run build:pro`：通过。
- `make build-release`：通过；发布二进制 SHA-256 为
  `f9d078a750e3049934c3c7886d5c6047861143742eca5a9f280a85a84de5a92f`。
- `git diff --check`：通过。
- `node test/contract/frontend-menu-inventory.mjs`：通过，生成 13 个主菜单、116 个路由清单。
- `STRICT=1 deploy/acceptance/preflight.sh`：未通过，只读预检发现 Docker socket、旧 WAF 镜像、域名解析和浏览器依赖阻断。

## 未完成

- 当前环境没有浏览器自动化依赖，未执行 `999.png` 尺寸下的桌面像素截图和移动端溢出检查。
- 当前环境不能访问 `/opt` 写入目录或 systemd 总线，未执行 `activate-release.sh`；没有发生生产二进制半替换。
- 生产健康接口、网站监管/WAF 页面入口和真实访问日志需要在可访问的运行主机上验证。
