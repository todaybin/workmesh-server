<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 网站、OpenResty 与 WAF 接口

WorkMesh Server 将网站元数据、OpenResty 控制面和网站 WAF 配置保存在
`WORKMESH_DATA_DIR` 下的 JSON 文件中，写入使用临时文件原子替换，适用于低资源节点。
服务启动时自动加载已有数据，不依赖常驻数据库连接。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET/POST | `/api/v2/websites`、`/api/v2/websites/list`、`/api/v2/websites/search` | 网站列表、创建和搜索 |
| GET | `/api/v2/websites/{id}` | 网站详情 |
| POST | `/api/v2/websites/update`、`/api/v2/websites/del` | 更新或删除网站 |
| GET/POST | `/api/v2/websites/waf/access-lists` | 读取或更新 WAF 黑白名单，支持 IP 和 CIDR |
| GET | `/api/v2/websites/waf/sites` | 列出网站 WAF 配置 |
| POST | `/api/v2/websites/waf/sites` | 更新网站 WAF 开关和 observe/block 模式 |
| GET | `/api/v2/websites/waf/sites/{id}/rules` | 列出网站规则 |
| POST | `/api/v2/websites/waf/rules`、`/rules/delete` | 新增、更新或删除网站规则 |
| GET/POST | `/api/v2/websites/waf/global`、`/waf/status` | 读取或更新节点级 WAF 配置和状态 |
| GET | `/api/v2/websites/waf/standard-rules` | 获取内置规则清单 |
| GET | `/api/v2/openresty/status`、`/modules`、`/https` | 查询 OpenResty 运行、模块和默认 HTTPS 状态 |
| POST | `/api/v2/openresty/update`、`/file`、`/scope`、`/build`、`/modules/update`、`/https` | 更新 OpenResty 配置摘要或默认 HTTPS 开关 |

所有成功响应使用数字 `code: 200`；参数错误返回 `ERR` 和 HTTP 400，资源不存在返回 HTTP 404。
OpenResty 的 `build`、模块和配置文件接口只更新本地控制面状态，不在 HTTP 请求中执行特权命令；实际运行时变更应由受控异步任务完成。
