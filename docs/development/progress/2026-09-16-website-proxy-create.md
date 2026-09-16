<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 反向站点代理配置修复

状态：`[x] 已完成`

## 结果

- `type=proxy` 创建后生成 `nginx/proxy/root.conf`，`site.conf` 仅引用 `nginx/proxy/*.conf`，反向代理菜单可立即看到默认配置。
- `/api/v2/websites/proxies/update` 按 `<name>.conf` 独立创建/编辑；状态接口原子切换 `.conf/.bak`，删除同步清理文件、include 和 SQLite 配置。
- 历史反向站点在服务启动和代理列表首次读取时补齐 `root.conf`。
- 代理目标、匹配路径、请求头仍复用既有白名单校验；OpenResty 检查或持久化失败会恢复文件和内存状态。

## 验证

- `GOWORK=off go test ./node/service -run 'TestWebsite' -count=1`、`GOWORK=off go test ./node/api -run 'TestWebsite' -count=1` 通过；完整包测试在当前沙盒因 `httptest` IPv6 监听权限受阻。
- `GOWORK=off go vet ./node/service ./node/api` 通过。
- `git diff --check` 通过。

## 正式部署

- 已通过 `deploy/install/activate-release.sh` 原子部署到 `/opt/workmesh-server`。
- `workmesh-server.service` 已重启，MainPID 为 `3951658`，9999 端口归属检查通过。
- `/health` 返回 `code=200,status=ok`，`/ready` 返回 `code=200,status=ready`。
- 运行二进制 SHA-256：`70943fc3fec6b7e17ba1cbceebbb4a710f16580dbfba547e21cab3899a0ab597`。
- 旧版本备份：`/opt/workmesh-server/bin/workmesh-server.bak.20260916164810-3951537`。
- 真实 OpenResty 配置 reload、反向代理上游访问和浏览器页面验收仍需在生产站点执行。
