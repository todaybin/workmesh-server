<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 部署与旧版本切换

安装脚本默认只执行预检。只有完成安装包校验、配置备份、Gateway 注册信息确认和健康检查后，才允许使用 `--apply` 写入 systemd。

脚本不会自动删除旧版本。旧 `workmesh-node-core`、`workmesh-node-agent` 服务和 `/opt/workmesh` 数据目录必须在新服务双节点验收完成、备份可恢复且取得人工确认后单独处理。

推荐顺序：

1. 旧版本只读盘点和备份。
2. 新版本在 `/opt/workmesh-server` 预检安装。
3. 停止旧版本并启动新服务。
4. 验证 `/health`、`/ready`、Gateway 注册、主次节点和全部功能。
5. 确认回滚窗口关闭后，再人工卸载旧服务；默认保留旧数据目录。
