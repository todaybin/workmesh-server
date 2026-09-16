<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 节点透传辅助逻辑质量整改（2026-09-06）

状态：`[x]` 本批次完成，待下一轮完整集成门禁覆盖。

## 本批次变更

- 将 `node/api/node_relay.go` 中的目标节点解析、URL 校验、Header 过滤、签名比较、nonce 和错误 envelope 辅助函数移至 `node_relay_helpers.go`。
- 主转发流程、WebSocket 隧道、节点签名、role epoch、nonce 防重放和响应字段保持不变。
- 主文件由 529 行降至 450 行，辅助文件 91 行；没有修改公开路由、请求参数、鉴权策略或节点地址来源。

## 验证

```text
gofmt -w node/api/node_relay.go node/api/node_relay_helpers.go
GOWORK=off go test ./node/api -run 'Test.*Relay|Test.*Forward|TestNode' -count=1
git diff --check -- node/api/node_relay.go node/api/node_relay_helpers.go
```

结果：定向测试和差异检查通过。全量 Go、Race、Vet、Node 合约、路由扫描及生产只读核对由唯一集成测试负责人在源码冻结后执行。

## 后续风险

- 真实跨节点 HTTP/WS 透传仍需次节点凭据和可访问网络；无凭据时保持 `blocked/not-run`。
- `nodes.json` 的兼容读取路径仍需与 SQLite 节点清单迁移在真实重启场景中复核。
