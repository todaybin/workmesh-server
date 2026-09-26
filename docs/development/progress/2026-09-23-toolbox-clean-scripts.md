# 2026-09-23 缓存清理与脚本库

- [x] 扫描根改为 `uploads`、`backups`、`logs/tasks`、`websites/*/logs`，并合并 `WORKMESH_PANEL_DIR` 或 `/opt/1panel` 下的任务日志和应用下载缓存。
- [x] Docker 清理改为解析 `docker system df` 的字节数，类型是 `images`、`containers`、`volumes`、`build_cache`。
- [x] 脚本搜索返回与 1Panel 相同的 9 条系统脚本；`id` 为数字，空分组为 `null`，系统脚本拒绝修改和删除。
- [x] 定向测试通过。2026-09-23 17:35 已切换生产进程，SHA256 `4a7da796e6b121103f9168ace8bff63251907a4731633398a966ce4da4aa1180`，MainPID `4104426`，首页脚本 `/assets/js/index-CPIjHZ5y.js`。
