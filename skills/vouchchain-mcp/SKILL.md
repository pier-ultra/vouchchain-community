---
name: vouchchain-mcp
display_name: MCP 客户端
description: VouchChain MCP 客户端技能：连接内置 /mcp 服务端（Bearer token 鉴权），列出并调用合同/发票/审批/法律知识库等 9 个工具。提供 python（mcp_client.py）与 node（mcp_client.mjs）两种零依赖客户端。
triggers: mcp,MCP客户端,MCP 客户端,客户端工具,工具调用,工具列表,mcp_client,调用工具
version: 1.0.0
---

# VouchChain MCP 客户端技能

通过内置 MCP 服务端（Streamable HTTP，端点 `/mcp`）调用 VouchChain 工具。客户端位于 `skills/vouchchain-mcp/`，两种实现任选，行为一致。

## 前提检查
1. VouchChain 服务已启动且监听（默认 `http://localhost:8000`）。
2. 服务端已设置 `MCP_TOKEN` 环境变量（未设置时 `/mcp` 返回 503）。
3. 调用时带上同一 `MCP_TOKEN` 作为 Bearer 令牌；可选 `X-MCP-User` 指定操作身份（默认 `mcp_agent`，审计按此留痕）。

## 客户端用法
```
# Python（零依赖，Python 3）
python skills/vouchchain-mcp/mcp_client.py --endpoint http://localhost:8000/mcp --token <MCP_TOKEN> ping
python skills/vouchchain-mcp/mcp_client.py ... list
python skills/vouchchain-mcp/mcp_client.py ... call query_contracts '{"keyword":"服务"}'

# Node（零依赖，Node 18+）
node skills/vouchchain-mcp/mcp_client.mjs --token <MCP_TOKEN> list
node skills/vouchchain-mcp/mcp_client.mjs call search_legal_kb '{"query":"逾期付款违约金","topK":3}'
```
环境变量 `MCP_TOKEN` / `VOUCHCHAIN_MCP_ENDPOINT` / `VOUCHCHAIN_MCP_USER` 可免去重复传参。

## 工具速查（共 9 个）
| 工具 | 必填参数 | 说明 |
|---|---|---|
| `query_contracts` | —（keyword/counterParty/status 可空） | 按关键词/相对方/状态查合同列表 |
| `get_contract` | `contractId` | 查合同详情（含文件与审批状态） |
| `get_party_profile` | `name` | 相对方风险画像（仅内部合同库） |
| `query_invoices` | —（keyword/verifyStatus 可空） | 查发票列表与验真状态 |
| `query_obligations` | —（keyword/status 可空） | 查合同义务（类型/到期/状态） |
| `query_approvals` | —（status/mine 可空） | 查审批实例列表 |
| `start_contract_approval` | `contractFileId` | 对合同文件发起审批（写操作） |
| `act_approval` | `instanceId`,`action`(approve/reject) | 审批同意/驳回（写操作，须当前节点审批人） |
| `search_legal_kb` | `query` | 法律条款知识库检索（民法典合同编为主） |

## 调用要点
- 查询类工具返回真实库数据；查不到就如实说明，不要编造 id。
- 写操作（`start_contract_approval`/`act_approval`）以 `--user` 身份落库并审计留痕，执行者必须是当前节点审批人否则失败。
- 工具返回错误时讲清原因，并给出下一步建议（例如无模板先到审批中心建模板）。
- 回复用简体中文，简洁分条，关键数字保留。

## 边界
- 相对方画像只基于内部合同库，不臆测工商/司法数据。
- 向量服务未启用时法律检索自动降级为关键词检索（如实说明，不谎报语义检索）。
