<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# Apps 测试结构整改（2026-09-07）

状态：`[x]` 本批次完成

## 本批次变更

- `node/api/apps_test.go` 从 522 行降至 360 行，保留安装、容器状态、生命周期、更新检查和版本比较测试。
- 新增 `node/api/apps_catalog_test.go`（177 行），承载远程应用目录、PHP 运行时字段、类型隔离和兼容路由测试。
- 测试函数、断言、真实 HTTP/文件/Docker 探测行为未修改；仅调整测试文件职责和 import。

## 验证

```text
gofmt -w node/api/apps_test.go node/api/apps_catalog_test.go
go test ./node/api -run 'RemoteAppCatalog|PHPVersionApp|RuntimeCatalog|LegacyCompatibilityDoesNotOverrideAppRoutes|AppInstall|AppInstalled|OpenRestyInstalled|ApplyAppContainer|AppOperations|AppOperation|AppDerived|AppCheckUpdate|CompareAppVersion' -count=1
git diff --check -- node/api/apps_test.go node/api/apps_catalog_test.go
```

结果：应用相关定向测试通过，差异检查通过。未执行全量门禁。
