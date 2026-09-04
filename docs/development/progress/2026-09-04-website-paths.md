<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 网站域名目录与独立配置

状态：`[>] 进行中`

本轮将新建站点目录统一为 `/www/wwwroot/{domain}`，入口为 `app`，Nginx、rewrite、日志、SSL、WAF 和运行缓存按域名目录分离。新增目录扫描、三级深度限制、路径越界校验、默认 `www/www` 展示和权限更新接口；配置与 rewrite 更新会写入实际站点文件并在持久化失败时恢复旧内容。WAF 站点状态同步保存到域名目录 `waf/site.json`，全局 WAF 可写入独立全局目录。

补充验证：新增 `TestZNMPStaticWebsiteAllInterfaces`，在临时站点根中覆盖 `znmp.sopvip.com` 的创建、列表/搜索、目录、Nginx、默认文档、rewrite、CORS、LBS、redirect、leech、stream、HTTPS、日志、跨站、域名、认证、批量操作、代理、模板、监控和 WAF 全链路，并确认删除后 WAF 文件清理。该测试及网站服务测试通过；`go vet`、前端类型检查和生产构建通过。全量 `go test ./...` 仍仅复现既有 Supervisor 测试的 `记录已存在` 冲突。

2026-09-04 追加修复：默认文档和代理接口只保留 `/api/v2`，直接读取 `/www/wwwroot/{domain}/nginx/site.conf` 与 `nginx/proxy`，不再返回旧扩展的空内容或更新时间占位；运行目录权限默认显示 `www/www`，根目录显示 `/`。OpenResty 容器探测已排除 `1panel/openresty` 旧实例，仅识别显式 WorkMesh/WAF 容器。源码和定向测试已通过。

2026-09-04 11:25 CST 已部署 WorkMesh Server 修复制品并重启 `workmesh-server.service`；随后构建并启动 `workmesh/openresty-waf:20260904`，容器名 `workmesh-openresty-waf`，独立配置目录 `/opt/workmesh-server/openresty-waf`，已健康运行并监听 80/443。生产二进制 SHA256=`22ad6dcc9a08637b9bcbd1a7c25f0f322fe78afb7b21941716a4d1f6254f1d43`，备份目录为 `/opt/workmesh-server/backups/deploy-20260904T112526+0800-website-v2`；`/health`、`/ready` 均返回 200。本次未修改 `/opt/1panel`。

2026-09-04 12:20 CST 修复跨文件系统分片上传：`/api/v2/files/chunkupload` 在 `EXDEV` 时改为目标文件系统临时复制后原子替换；新增站点 OpenResty 配置读取和空重定向 `data:null` 契约。`znmp.sopvip.com` 的 `app` 当前为空，已移除误放的面板 dist 文件并保存在 `/opt/workmesh-server/backups/znmp-panel-dist-20260904T120000+0800`，域名仅作为普通站点服务。
