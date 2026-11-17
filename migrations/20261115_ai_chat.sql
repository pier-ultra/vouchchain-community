-- AI 对话助手（对话即操作）：会话与消息留痕（本文件幂等，可重复执行）
CREATE TABLE IF NOT EXISTS chat_session (
  id BIGSERIAL PRIMARY KEY,
  username VARCHAR(100) NOT NULL DEFAULT '',
  title VARCHAR(200) NOT NULL DEFAULT '新对话',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS chat_message (
  id BIGSERIAL PRIMARY KEY,
  session_id BIGINT NOT NULL,
  role VARCHAR(20) NOT NULL, -- user / assistant
  content TEXT NOT NULL DEFAULT '',
  tool_trace JSONB NOT NULL DEFAULT '[]', -- 本轮工具执行步骤 [{tool,args,summary,ok}]
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_chat_message_session ON chat_message(session_id);
CREATE INDEX IF NOT EXISTS idx_chat_session_user ON chat_session(username);
