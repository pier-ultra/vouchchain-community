-- MCP Client：外部 MCP server 连接配置（系统作为客户端去连接外部 MCP server）
-- 用途：把外部 MCP server 暴露的工具动态注册进 VouchChain AI 对话（ChatTools 统一工具表）
-- 字段：
--   endpoint    外部 MCP server 端点（Streamable HTTP / SSE，如 https://host/mcp）
--   token       鉴权 Bearer token（存 enc:v1: 密文 AES-256-GCM，需 APP_AES_KEY；可空）
--   extra_headers 额外请求头 JSON（如 {"X-Org":"xxx"}，可空）
--   enabled     启停开关（启动时/变更时按此连接并拉取工具注册）
CREATE TABLE IF NOT EXISTS mcp_server (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL DEFAULT '',
  endpoint VARCHAR(500) NOT NULL DEFAULT '',
  token TEXT NOT NULL DEFAULT '',                    -- 敏感：存 enc:v1: 密文，需 APP_AES_KEY
  extra_headers JSONB NOT NULL DEFAULT '{}',
  description VARCHAR(500) NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  tool_count INTEGER NOT NULL DEFAULT 0,             -- 最近一次拉取的工具数（展示用）
  last_status VARCHAR(20) NOT NULL DEFAULT 'idle',   -- idle / ok / error
  last_error VARCHAR(500) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_mcp_server_enabled ON mcp_server(enabled);
