#!/usr/bin/env node
/**
 * VouchChain MCP 客户端（Node 版，零第三方依赖，Node 18+ 内置 fetch）
 *
 * 连接 VouchChain 内置 MCP 服务端（Streamable HTTP，/mcp，Bearer <MCP_TOKEN>），
 * 列出工具 / 调用工具 / 握手健康检查。与 mcp_client.py（Python 版）行为一致。
 *
 * 用法:
 *   node mcp_client.mjs ping
 *   node mcp_client.mjs list
 *   node mcp_client.mjs call query_contracts '{"keyword":"服务"}'
 *   node mcp_client.mjs call get_contract '{"contractId":1}'
 *
 * 配置（环境变量优先，命令行参数覆盖）:
 *   VOUCHCHAIN_MCP_ENDPOINT   MCP 端点（默认 http://localhost:8000/mcp）
 *   MCP_TOKEN                 Bearer 令牌（必填）
 *   VOUCHCHAIN_MCP_USER       操作身份（默认 mcp_agent）
 */

const DEFAULT_ENDPOINT = "http://localhost:8000/mcp";
const DEFAULT_USER = "mcp_agent";
const PROTOCOL_VERSION = "2025-06-18";

class MCPError extends Error {}

class McpClient {
  constructor(endpoint, token, user = DEFAULT_USER) {
    if (!token) {
      throw new MCPError("未提供 MCP_TOKEN：请设置环境变量 MCP_TOKEN 或传 --token");
    }
    this.endpoint = endpoint.replace(/\/+$/, "");
    this.token = token;
    this.user = user;
    this.sessionId = null;
    this._reqId = 0;
  }

  async _post(payload) {
    const headers = {
      "Content-Type": "application/json",
      Accept: "application/json, text/event-stream",
      Authorization: "Bearer " + this.token,
      "X-MCP-User": this.user,
    };
    if (this.sessionId) headers["Mcp-Session-Id"] = this.sessionId;

    let resp;
    try {
      resp = await fetch(this.endpoint, {
        method: "POST",
        headers,
        body: JSON.stringify(payload),
        signal: AbortSignal.timeout(30000),
      });
    } catch (e) {
      throw new MCPError(`连接失败（${this.endpoint}）：${e.message}`);
    }

    const sid = resp.headers.get("Mcp-Session-Id");
    if (sid) this.sessionId = sid;

    const ctype = resp.headers.get("Content-Type") || "";
    const text = await resp.text();

    if (!resp.ok) {
      throw new MCPError(`HTTP ${resp.status}: ${text.slice(0, 400)}`);
    }
    return this._parseBody(text, ctype);
  }

  _parseBody(text, ctype) {
    if (ctype.includes("text/event-stream")) {
      const lines = text.split("\n").map((l) => l.replace(/\r$/, ""));
      const data = lines.filter((l) => l.startsWith("data:")).map((l) => l.slice(5).trim());
      if (!data.length) return null;
      return JSON.parse(data[data.length - 1]);
    }
    if (!text.trim()) return null;
    return JSON.parse(text);
  }

  async _rpc(method, params, isNotification = false) {
    this._reqId += 1;
    const payload = { jsonrpc: "2.0", method };
    if (params !== undefined) payload.params = params;
    if (!isNotification) payload.id = this._reqId;
    const resp = await this._post(payload);
    if (isNotification || resp === null) return null;
    if (resp.error) {
      throw new MCPError(`服务端错误 [${resp.error.code}]: ${resp.error.message}`);
    }
    return resp.result;
  }

  async initialize() {
    const result = await this._rpc("initialize", {
      protocolVersion: PROTOCOL_VERSION,
      capabilities: {},
      clientInfo: { name: "vouchchain-mcp-client", version: "1.0.0" },
    });
    await this._rpc("notifications/initialized", undefined, true);
    return result;
  }

  async toolsList() {
    const result = await this._rpc("tools/list");
    return result?.tools ?? [];
  }

  async toolsCall(name, arguments_ = {}) {
    const result = await this._rpc("tools/call", { name, arguments: arguments_ });
    if (result === null) return "";
    if (result.isError) {
      const texts = (result.content || []).filter((c) => c.type === "text").map((c) => c.text);
      throw new MCPError("工具执行失败：" + texts.join(""));
    }
    return (result.content || []).filter((c) => c.type === "text").map((c) => c.text).join("");
  }
}

function fmtTool(t) {
  const lines = [`## ${t.name}`, `   ${t.description || ""}`];
  const schema = t.inputSchema || {};
  const props = schema.properties || {};
  const req = new Set(schema.required || []);
  const keys = Object.keys(props);
  if (keys.length) {
    lines.push("   参数:");
    for (const pname of keys) {
      const p = props[pname] || {};
      const star = req.has(pname) ? "*" : "";
      lines.push(`     - ${pname}${star} (${p.type || "?"}) ${p.description || ""}`);
    }
  }
  return lines.join("\n");
}

async function main() {
  const args = process.argv.slice(2);
  const env = process.env;
  let endpoint = env.VOUCHCHAIN_MCP_ENDPOINT || DEFAULT_ENDPOINT;
  let token = env.MCP_TOKEN || "";
  let user = env.VOUCHCHAIN_MCP_USER || DEFAULT_USER;
  const rest = [];

  // 简易参数解析：--endpoint / --token / --user
  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--endpoint") endpoint = args[++i];
    else if (args[i] === "--token") token = args[++i];
    else if (args[i] === "--user") user = args[++i];
    else rest.push(args[i]);
  }
  const [cmd, tool, argsJson] = rest;

  let client;
  try {
    client = new McpClient(endpoint, token, user);
  } catch (e) {
    console.error(`[配置错误] ${e.message}`);
    process.exit(2);
  }

  try {
    if (cmd === "ping") {
      const info = (await client.initialize()) || {};
      const si = info.serverInfo || {};
      console.log(`[OK] 握手成功：${si.name || "?"} v${si.version || "?"} (session=${client.sessionId || "无状态"})`);
      return 0;
    }
    if (cmd === "list") {
      await client.initialize();
      const tools = await client.toolsList();
      console.log(`[OK] 共 ${tools.length} 个工具：\n`);
      console.log(tools.map(fmtTool).join("\n"));
      return 0;
    }
    if (cmd === "call") {
      if (!tool) {
        console.error("[参数错误] call 需要工具名，如 call query_contracts");
        return 2;
      }
      await client.initialize();
      let parsed = {};
      try {
        parsed = argsJson && argsJson.trim() ? JSON.parse(argsJson) : {};
      } catch (e) {
        console.error(`[参数错误] args 不是合法 JSON：${e.message}`);
        return 2;
      }
      if (typeof parsed !== "object" || Array.isArray(parsed)) {
        console.error('[参数错误] args 必须是 JSON 对象，如 {"keyword":"服务"}');
        return 2;
      }
      const out = await client.toolsCall(tool, parsed);
      console.log(out);
      return 0;
    }
    console.error("用法: node mcp_client.mjs <ping|list|call> [args]");
    return 2;
  } catch (e) {
    if (e instanceof MCPError) {
      console.error(`[MCP错误] ${e.message}`);
      return 1;
    }
    console.error(`[错误] ${e.message || e}`);
    return 1;
  }
}

main().then((code) => process.exit(code));
