<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 文件管理接口（首批）

文件接口在节点本机执行，保留 `/api/v2/files/*` 旧路径和 JSON envelope。路径由服务进程权限控制，调用方不得使用路径拼接绕过操作系统权限。

已迁移接口：

- `POST /api/v2/files`：创建文件或目录（`path`、`isDir`）。
- `POST /api/v2/files/search`：列出目录，支持 `showHidden`。
- `POST /api/v2/files/content`、`POST /api/v2/files/save`：读取和保存文本内容。
- `POST /api/v2/files/del`：删除文件/目录，`forceDelete` 为递归删除。
- `POST /api/v2/files/rename`、`POST /api/v2/files/move`：重命名和移动。
- `POST /api/v2/files/size`：递归计算目录大小。
- `GET /api/v2/files/download?path=...`：下载文件并设置附件响应头。
- `POST /api/v2/files/share/create`：为现有文件或目录创建随机 token 分享，记录持久化到 `WORKMESH_DATA_DIR/file-shares.json`。
- `GET /api/v2/files/share/check?token=...`、`share/info`、`share/qrcode`：校验 token 和目标是否仍存在。
- `GET /api/v2/files/share/download?token=...`：仅对有效 token 提供附件下载。
- `POST /api/v2/files/share/search`、`share/del`：查询或撤销本地分享记录。

压缩、分片上传、回收站、分享和远程 wget 等长耗时能力继续沿用兼容路由，后续迁移时应异步化并增加任务状态查询。
