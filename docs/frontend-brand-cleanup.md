# 前端品牌清理清单

前端兼容审计以只读参考 `/www/apps/1Panel/frontend/src` 为来源，WorkMesh 的工作副本位于 `apps/workmesh-server/web/src`。迁移阶段不删除任何业务页面、API 模块、路由、语言包或组件。以下清单记录后续必须逐项处理的旧产品标识，避免把品牌清理误认为功能删减。

## 已处理

- `package.json` 与 `package-lock.json` 的包名改为 `@workmesh/server-web`，描述改为 WorkMesh Server。
- `.env` 的 `VITE_GLOB_APP_TITLE` 改为 `WorkMesh Server`。
- 开发代理默认目标改为 `http://localhost:9999/`，不绑定旧节点地址。
- `vite.config.ts` 构建输出改为独立 `web/dist`，不再写入旧 Core 目录。
- `index.html` 标题改为 `WorkMesh Server`。

## 待清理标识

以下内容仍保留在完整功能快照中，必须由品牌迁移阶段按语义审核后替换，不得简单全局替换：

- 语言包中的旧产品名称、旧产品安装目录、服务名、镜像前缀和升级提示：`src/lang/modules/*`。
- 旧产品域名和升级/页脚检测：`src/components/footer-navigation/model.ts`。
- 旧产品管理块、SSH 别名和脚本输出：`src/components/vscode-open/index.vue`。
- 旧产品特有文件名/目录保护规则：`src/components/file-list/index.vue`。
- 多机管理页面中的旧节点服务发现、安装包名称和服务名：`src/views/advanced/multi-node/*`。
- 旧产品兼容错误码和能力开关：`src/enums/http-enum.ts`、`src/typings/global.d.ts`、`src/api/index.ts`。
- 资源文件名或字体元数据中包含的旧品牌：`src/assets/*`。

## 清理规则

1. 产品文案、域名、服务名、环境变量和镜像标签统一改为 WorkMesh 语义。
2. 旧路径只在兼容协议或迁移检测确实需要时保留，并在 API 文档注明。
3. 许可证、版权和第三方 NOTICE 不删除，来源记录放在 `docs/legal/`。
4. 每次清理后运行品牌关键字扫描并更新本清单。
