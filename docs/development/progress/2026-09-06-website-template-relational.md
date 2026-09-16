<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 网站模板关系化与真实预览（2026-09-06）

## 目标

把网站模板、模板产物从网站扩展状态 JSON BLOB 中移到统一 SQLite 关系表，并让上传、预览、
产物生成和删除产生真实文件副作用，作为网站应用部署上线基础的一部分。

## 已完成

- 新增 `website_templates`、`website_template_outputs`、`website_template_migrations` 表定义。
- 新增启动迁移 `0015-website-template-relational`，迁移只创建缺失结构，不覆盖用户数据。
- 旧 `website_extension_state` 中的模板和产物只做一次性导入，使用完成标记保证幂等。
- 模板 CRUD 改为 SQLite 查询和写入。
- ZIP 上传使用临时文件、大小/条目/路径校验和原子 rename。
- 预览读取真实单文件内容或 ZIP 内的 `index.html`/`index.htm`，变量替换后返回 `html`。
- 模板产物生成到 `WORKMESH_DATA_DIR/templates/outputs/<id>`，失败时清理数据库记录和半成品目录。
- 删除操作限制在模板受控目录，不允许路径穿越。
- `apps/1Panel` 和 `apps/1Panel/frontend` 未修改、未编译、未写入测试结果。

## 验证要求

本文件新增迁移代码后，必须由唯一集成测试负责人重新执行 Go 全量、race、vet、Node 契约、
路由扫描和网站定向测试。前端没有变更，不重新执行前端构建。

## 尚未完成

- 已在门禁通过后完成后端二进制原子替换并重启：候选制品 SHA256 为
  `4d1b0b81ba2df32892830e9413cb070fbe6c50220f5b713374ff6e5fb245c30f`，备份目录为
  `/opt/workmesh-server/backups/deploy-20260906T090558Z-website-template`。
- 生产只读核对已确认 `0014`、`0015` 均进入迁移账本，三个模板关系表已创建且当前记录数为 0；
  `/health`、`/ready`、`znmp.sopvip.com` HTTP/HTTPS 和 OpenResty `nginx -t` 均正常。
- `website_extension_state` 中代理、认证、扩展日志等历史字段仍需后续按领域关系化，不能将本次模板迁移视为全部扩展迁移完成。
