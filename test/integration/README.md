<!-- SPDX-License-Identifier: LicenseRef-WorkMesh-Pending -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 集成测试

集成测试只调用新服务的真实运行时契约，不连接生产主机，也不修改旧项目。测试重点是跨模块行为：角色 epoch 防并发写、取消上下文和 Gateway 节点注册字段。

运行：

```powershell
go test ./test/integration -count=1
```

需要访问真实 Gateway 或 Docker 的测试应在后续增加显式环境开关（例如 `WORKMESH_TEST_GATEWAY_URL`），默认不允许隐式访问外部系统。
