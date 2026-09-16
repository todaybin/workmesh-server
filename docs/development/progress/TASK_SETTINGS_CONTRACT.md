<!-- SPDX-License-Identifier: GPL-3.0-only -->
# V2 设置参数契约轮换任务

只检查 `/api/v2` 设置、日志、主机和计划任务写接口的参数契约，不修改 apps/1Panel、前端、生产数据或网站运行时文件。

工作：从现有路由和前端调用提取 scope/operate/必填字段、类型和范围；为缺失边界补定向测试，保持未知字段兼容和原有响应 envelope。不得用固定成功或 JSON fixture。

验收：运行设置域定向测试和 `node test/contract/security-contract-scan.mjs --strict`；把路由、方法、状态码、错误码和测试结果追加到本文件。
