<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 主次节点角色边界

角色切换由本机多机管理负责，不由 Gateway 直接选主。所有写操作必须携带并校验 `roleEpoch`；切换流程必须经过健康检查、任务锁定、旧主 fencing、双向确认和审计。配置 `WORKMESH_DATA_DIR` 时，角色和 epoch 原子保存到 `role-state.json`，服务重启后继续拒绝旧 epoch 写入。

自动故障转移需要额外见证节点，当前不在本适配层承诺；跨节点自动提交前仍需实现外部协调锁和恢复扫描。
