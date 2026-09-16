<!-- SPDX-License-Identifier: GPL-3.0-only -->
<!-- Copyright (c) 2026 WorkMesh contributors -->

# 前端 HTTP/WS 真实合约执行器

状态：`[>]` 已建立安全执行入口，完整登录后业务验收仍在进行。

## 执行器

脚本：[`../../../test/contract/frontend-contract-executor.mjs`](../../../test/contract/frontend-contract-executor.mjs)

输入：[`../../inventory/frontend-http-ws-contract-matrix.json`](../../inventory/frontend-http-ws-contract-matrix.json)

脚本不会修改矩阵状态，只在 `docs/inventory/` 生成独立的带时间戳证据文件。

## 安全策略

- `WORKMESH_BASE_URL`：服务地址；未设置时使用 `http://127.0.0.1:9999`。
- `WORKMESH_TOKEN`：可选 Bearer Token，只从环境变量读取。
- `WORKMESH_COOKIE`：可选完整 Cookie，只从环境变量读取。
- `WORKMESH_SCOPE`：默认为 `boundary`。
  - `boundary`：只执行 6 条 HTTP 健康/认证边界和 1 条 WS 未授权握手；不执行 389 条业务接口。
  - `read`：增加没有动态路径的 GET/HEAD，以及 WS 握手。
  - `fixtures`：增加由 `WORKMESH_REQUEST_FIXTURES` 提供真实请求体的案例。
  - `all`：除真实 fixture 外，还要求 `WORKMESH_CONFIRM_DESTRUCTIVE=YES`；脚本不会自行生成业务数据。
- `WORKMESH_REQUEST_FIXTURES`：可选 JSON 文件，按矩阵案例 ID 提供真实的 `path`、`method`、`headers`、`body`、`expectedStatus`。
- `--dry-run`：只检查执行选择，不发送请求。

示例：

```bash
# 无凭据：只执行七条 HTTP/WS 公开/未授权边界，结果写入指定证据文件
node test/contract/frontend-contract-executor.mjs \
  --out docs/inventory/frontend-contract-evidence-$(date +%Y%m%dT%H%M%S).json

# 登录会话下执行静态安全读取和 WS 握手
WORKMESH_BASE_URL='http://127.0.0.1:9999' \
WORKMESH_COOKIE='SecurityEntrance=...; session=...' \
WORKMESH_SCOPE=read \
node test/contract/frontend-contract-executor.mjs

# 使用外部真实 fixture 执行指定业务案例；fixture 不得写入密码等明文敏感值
WORKMESH_TOKEN='...' \
WORKMESH_SCOPE=fixtures \
WORKMESH_REQUEST_FIXTURES=/secure/workmesh/fixtures/frontend-contract.json \
node test/contract/frontend-contract-executor.mjs
```

fixture 只允许描述真实测试输入，例如：

```json
{
  "frontend-contract-123": {
    "path": "/api/v2/example/read",
    "method": "POST",
    "body": { "真实字段": "由验收人员准备的值" },
    "expectedStatus": [200]
  }
}
```

请求体不会出现在证据文件中，只记录 body 的字段名；响应只保留脱敏摘要。动态路径必须由 fixture 显式提供，执行器不会用 `1`、`test` 或空字符串替换路径参数。

## WS 验证范围

WS 使用原始 HTTP Upgrade 握手发送 Cookie/Bearer 认证头，记录 `101 Switching Protocols` 或实际失败状态；成功后发送正常关闭帧并销毁连接。当前只验证握手和关闭释放，不发送终端、脚本或容器业务消息，后续需要专门的 PTY/消息序列测试。

## 四级执行范围限制

| 范围 | 当前实际行为 | 业务接口状态 |
| --- | --- | --- |
| `boundary` | 无凭据也可执行 6 条 HTTP 健康/认证边界和 1 条 WS 未授权握手；有凭据时仍只执行这 7 条 | 389 条保持 `not-run` |
| `read` | 必须提供 Token 或 Cookie；执行无动态路径的 GET/HEAD 和 WS 握手，动态路径、POST、上传下载没有 fixture 时不执行 | 未实际请求项保持 `not-run`，缺少参数项记录 `blocked` |
| `fixtures` | 必须提供 `WORKMESH_REQUEST_FIXTURES`；只执行 fixture 明确提供的真实路径、方法和请求体 | 未提供 fixture 的项保持 `not-run`/`blocked` |
| `all` | 必须提供真实 fixture，并设置 `WORKMESH_CONFIRM_DESTRUCTIVE=YES`；仍不会自动生成业务数据 | 只有实际执行并有响应证据的独立报告才可判定 |

`fixtures` 和 `all` 的 fixture 不允许写入仓库，也不应包含明文密码、Token、Cookie 或私钥。执行器只在证据中记录字段名和脱敏摘要。

## 已执行结果

2026-09-05 在无凭据环境执行了七条边界请求，其中包括一次真实 WS 未授权握手：

```text
证据：docs/inventory/frontend-contract-evidence-20260905T142131654Z.json
HTTP：健康/就绪/v2 健康/认证设置均 200；当前用户和概览均 401
WS：/api/v2/hosts/terminal/local -> HTTP/1.1 401 Unauthorized
结果：pass 7
```

该证据只证明公开边界和未授权握手行为，不代表登录后的 389 条业务接口可用；`frontend-http-ws-contract-matrix.json` 的 389 条案例仍全部为 `not-run`。

另以 `--dry-run` 验证了无凭据模式不会发送请求，并验证矩阵 SHA-256 在执行前后保持不变。

## Node 测试

```bash
node --test test/contract/frontend-contract-executor.test.mjs
```

测试覆盖：无凭据 dry-run、七条 HTTP/WS 边界选择、矩阵不可变，以及没有 fixture 时不执行需要业务请求体的接口。
