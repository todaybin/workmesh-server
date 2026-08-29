<!-- SPDX-License-Identifier: LicenseRef-WorkMesh-Pending -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 主次节点角色边界

角色切换由本机多机管理负责，不由 Gateway 直接选主。所有写操作必须携带并校验 `roleEpoch`；切换流程必须经过健康检查、任务锁定、旧主 fencing、双向确认和审计。

TODO：实现基于数据库事务的 `Store`、节点锁和恢复扫描；自动故障转移需要额外见证节点，当前不在本适配层承诺。
