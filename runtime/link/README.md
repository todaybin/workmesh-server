<!-- SPDX-License-Identifier: LicenseRef-WorkMesh-Pending -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 节点通信边界

节点之间只同步控制面声明的数据和版本游标，不覆盖对端 Docker、数据库、文件、进程和正在执行的本地任务。连接必须使用 TLS、节点凭据、请求签名、时间戳、nonce 和幂等键。

TODO：实现 HTTPS transport、证书轮换、断线重试、冲突报告和同步审计；不得在协议层传输私钥明文。
