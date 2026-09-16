<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 网站模板与模板产物

网站模板接口属于网站应用部署能力，运行时数据由统一 SQLite `workmesh.db` 提供，模板 ZIP
和生成产物由 `WORKMESH_DATA_DIR/templates/` 下的受控文件目录提供。`apps/1Panel` 仅用于只读
核对请求字段和响应字段，不参与运行时读写。

## 路由

| 方法 | 路径 | 数据来源/副作用 |
| --- | --- | --- |
| POST | `/api/v2/websites/templates/search` | SQLite 分页查询模板 |
| POST | `/api/v2/websites/templates` | SQLite 新增单文件或 ZIP 模板 |
| POST | `/api/v2/websites/templates/update` | SQLite 更新模板元数据 |
| POST | `/api/v2/websites/templates/get` | SQLite 查询单条模板 |
| POST | `/api/v2/websites/templates/del` | 删除 SQLite 记录并清理受控文件/产物 |
| POST | `/api/v2/websites/templates/upload` | 校验并原子保存真实 ZIP 文件 |
| POST | `/api/v2/websites/templates/preview` | 读取真实模板并渲染 `html` |
| POST | `/api/v2/websites/templates/outputs/search` | SQLite 分页查询模板产物 |
| POST | `/api/v2/websites/templates/outputs` | 渲染真实文件并写入产物记录 |
| POST | `/api/v2/websites/templates/outputs/get` | SQLite 查询单条产物 |
| POST | `/api/v2/websites/templates/outputs/del` | 删除 SQLite 记录和受控产物目录 |

## 数据库迁移

`0015-website-template-relational` 创建：

- `website_templates`
- `website_template_outputs`
- `website_template_migrations`

首次启动会把旧 `website_extension_state` 中的模板和产物一次性导入关系表，并写入
`legacy-json-v1` 完成标记。迁移幂等，失败会阻止服务就绪并保留重试机会。

## 真实性与安全边界

- 单文件模板渲染为真实 `index.html`；ZIP 模板只读取真实 ZIP 内的 `index.html`/`index.htm`。
- 产物目录位于 `templates/outputs/<id>`，不接受目录外路径。
- ZIP 条目拒绝符号链接和路径穿越，并限制单文件、总文本和条目数量。
- 模板删除不会删除模板根目录之外的文件。
- 未找到模板、ZIP 损坏、文件写入失败和数据库不可用均返回错误，不返回固定成功结果。
