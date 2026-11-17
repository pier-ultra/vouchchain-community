#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
VouchChain MCP 客户端（零第三方依赖，Python 3 标准库 only）

连接 VouchChain 内置 MCP 服务端（Streamable HTTP，/mcp 端点，鉴权 Bearer <MCP_TOKEN>），
列出工具 / 调用工具 / 握手健康检查。

用法:
  python mcp_client.py ping
  python mcp_client.py list
  python mcp_client.py call query_contracts '{"keyword":"服务"}'
  python mcp_client.py call get_contract '{"contractId":1}'
  python mcp_client.py call search_legal_kb '{"query":"逾期付款违约金","topK":4}'

配置（环境变量优先，命令行参数覆盖）:
  VOUCHCHAIN_MCP_ENDPOINT   MCP 端点 URL（默认 http://localhost:8000/mcp）
  MCP_TOKEN                 Bearer 令牌（必填；服务端未设置 MCP_TOKEN 时返回 503）
  VOUCHCHAIN_MCP_USER       操作身份（默认 mcp_agent，审计 username 按此留痕）

说明:
  - 协议：MCP Streamable HTTP（JSON-RPC 2.0 over HTTP POST）
  - 响应兼容 application/json 与 text/event-stream（SSE）两种格式
  - 自动维护 Mcp-Session-Id（服务端返回时后续请求携带，无状态亦可工作）
  - 写操作（start_contract_approval / act_approval）会以 --user 身份落库并审计留痕
"""

import json
import os
import sys
import urllib.error
import urllib.request

DEFAULT_ENDPOINT = "http://localhost:8000/mcp"
DEFAULT_USER = "mcp_agent"
PROTOCOL_VERSION = "2025-06-18"  # mcp-go v1.5.0 LATEST


class MCPError(Exception):
    """MCP 调用错误（服务端返回 error 或 HTTP 异常）"""


class McpClient:
    """MCP Streamable HTTP 客户端（零依赖）"""

    def __init__(self, endpoint, token, user=DEFAULT_USER):
        if not token:
            raise MCPError("未提供 MCP_TOKEN：请设置环境变量 MCP_TOKEN 或传 --token")
        self.endpoint = endpoint.rstrip("/")
        self.token = token
        self.user = user
        self.session_id = None
        self._req_id = 0

    # ---------- 底层 HTTP ----------
    def _post(self, payload):
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
            "Authorization": "Bearer " + self.token,
            "X-MCP-User": self.user,
        }
        if self.session_id:
            headers["Mcp-Session-Id"] = self.session_id
        req = urllib.request.Request(self.endpoint, data=body, headers=headers, method="POST")
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                raw = resp.read()
                # 捕获服务端下发的 session id（首次 initialize 通常带）
                sid = resp.headers.get("Mcp-Session-Id")
                if sid:
                    self.session_id = sid
                ctype = resp.headers.get("Content-Type", "")
                return self._parse_body(raw, ctype)
        except urllib.error.HTTPError as e:
            detail = e.read().decode("utf-8", "replace")
            raise MCPError(f"HTTP {e.code}: {detail[:400]}")
        except urllib.error.URLError as e:
            raise MCPError(f"连接失败（{self.endpoint}）：{e.reason}")

    @staticmethod
    def _parse_body(raw, ctype):
        text = raw.decode("utf-8", "replace")
        # SSE：text/event-stream，body 形如 "event: message\ndata: {...}\n\n"
        if "text/event-stream" in ctype.lower():
            data_lines = []
            for line in text.splitlines():
                line = line.rstrip("\r")
                if line.startswith("data:"):
                    data_lines.append(line[len("data:"):].strip())
            if not data_lines:
                return None
            return json.loads(data_lines[-1])
        # 普通 JSON 响应
        if not text.strip():
            return None
        return json.loads(text)

    # ---------- JSON-RPC ----------
    def _rpc(self, method, params=None, is_notification=False):
        self._req_id += 1
        payload = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            payload["params"] = params
        if not is_notification:
            payload["id"] = self._req_id
        resp = self._post(payload)
        if is_notification:
            return None
        if resp is None:
            return None
        if "error" in resp and resp["error"]:
            err = resp["error"]
            raise MCPError(f"服务端错误 [{err.get('code')}]: {err.get('message')}")
        return resp.get("result")

    # ---------- 公开操作 ----------
    def initialize(self):
        """握手：返回服务端 serverInfo / capabilities，并捕获 session"""
        result = self._rpc("initialize", {
            "protocolVersion": PROTOCOL_VERSION,
            "capabilities": {},
            "clientInfo": {"name": "vouchchain-mcp-client", "version": "1.0.0"},
        })
        # 通知已初始化（不期待响应）
        self._rpc("notifications/initialized", is_notification=True)
        return result

    def tools_list(self):
        """列出全部工具（含参数 schema）"""
        result = self._rpc("tools/list")
        return result.get("tools", []) if result else []

    def tools_call(self, name, arguments=None):
        """调用工具；arguments 为 dict，如 {"keyword": "服务"}"""
        result = self._rpc("tools/call", {"name": name, "arguments": arguments or {}})
        if result is None:
            return ""
        # mcp-go 返回 {content: [{type:"text", text:"..."}]}；isError 表示业务失败但 HTTP 200
        if result.get("isError"):
            texts = [c.get("text", "") for c in result.get("content", []) if c.get("type") == "text"]
            raise MCPError("工具执行失败：" + "".join(texts))
        texts = [c.get("text", "") for c in result.get("content", []) if c.get("type") == "text"]
        return "".join(texts)


def _fmt_tool(t):
    """工具格式化输出：名称 + 描述 + 参数"""
    name = t.get("name", "")
    desc = t.get("description", "")
    lines = [f"## {name}", f"   {desc}"]
    schema = t.get("inputSchema", {}) or {}
    props = schema.get("properties", {}) or {}
    req = set(schema.get("required", []) or [])
    if props:
        lines.append("   参数:")
        for pname, p in props.items():
            ptype = p.get("type", "?")
            pdesc = p.get("description", "")
            star = "*" if pname in req else ""
            lines.append(f"     - {pname}{star} ({ptype}) {pdesc}")
    return "\n".join(lines)


def main():
    import argparse

    parser = argparse.ArgumentParser(description="VouchChain MCP 客户端（零依赖）")
    parser.add_argument("--endpoint", default=os.environ.get("VOUCHCHAIN_MCP_ENDPOINT", DEFAULT_ENDPOINT),
                        help=f"MCP 端点（默认 {DEFAULT_ENDPOINT}）")
    parser.add_argument("--token", default=os.environ.get("MCP_TOKEN", ""), help="Bearer 令牌（或环境变量 MCP_TOKEN）")
    parser.add_argument("--user", default=os.environ.get("VOUCHCHAIN_MCP_USER", DEFAULT_USER),
                        help=f"操作身份（默认 {DEFAULT_USER}）")
    sub = parser.add_subparsers(dest="cmd", required=True)

    sub.add_parser("ping", help="握手健康检查")
    sub.add_parser("list", help="列出全部工具")
    p_call = sub.add_parser("call", help="调用工具")
    p_call.add_argument("tool", help="工具名，如 query_contracts")
    p_call.add_argument("args", nargs="?", default="{}", help='JSON 参数，如 \'{"keyword":"服务"}\'')

    args = parser.parse_args()

    try:
        client = McpClient(args.endpoint, args.token, args.user)
    except MCPError as e:
        print(f"[配置错误] {e}")
        return 2

    try:
        if args.cmd == "ping":
            info = client.initialize() or {}
            si = info.get("serverInfo", {})
            print(f"[OK] 握手成功：{si.get('name', '?')} v{si.get('version', '?')} "
                  f"(session={client.session_id or '无状态'})")
            return 0

        if args.cmd == "list":
            client.initialize()
            tools = client.tools_list()
            print(f"[OK] 共 {len(tools)} 个工具：\n")
            print("\n".join(_fmt_tool(t) for t in tools))
            return 0

        if args.cmd == "call":
            client.initialize()
            try:
                parsed = json.loads(args.args or "{}")
            except json.JSONDecodeError as e:
                print(f"[参数错误] args 不是合法 JSON：{e}")
                return 2
            if not isinstance(parsed, dict):
                print("[参数错误] args 必须是 JSON 对象，如 {\"keyword\":\"服务\"}")
                return 2
            out = client.tools_call(args.tool, parsed)
            print(out)
            return 0
    except MCPError as e:
        print(f"[MCP错误] {e}")
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
