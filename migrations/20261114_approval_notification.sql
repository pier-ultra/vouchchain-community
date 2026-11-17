-- 审批流 + 消息中心迁移（instabox PG；本文件幂等，可重复执行）
CREATE TABLE IF NOT EXISTS approval_template (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  nodes JSONB NOT NULL DEFAULT '[]',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS approval_instance (
  id BIGSERIAL PRIMARY KEY,
  target_type VARCHAR(50) DEFAULT 'contract_file',
  target_id BIGINT NOT NULL,
  template_id BIGINT DEFAULT 0,
  status VARCHAR(20) NOT NULL DEFAULT 'running',
  current_seq INT NOT NULL DEFAULT 1,
  created_by VARCHAR(100) DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS approval_node (
  id BIGSERIAL PRIMARY KEY,
  instance_id BIGINT NOT NULL,
  seq INT NOT NULL,
  name VARCHAR(100) DEFAULT '',
  approver VARCHAR(100) DEFAULT '',
  mode VARCHAR(10) DEFAULT 'or',
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  comment TEXT DEFAULT '',
  acted_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_approval_node_inst ON approval_node(instance_id);
CREATE INDEX IF NOT EXISTS idx_approval_inst_target ON approval_instance(target_id);
ALTER TABLE contract_file ADD COLUMN IF NOT EXISTS approval_state VARCHAR(20) DEFAULT 'none';
CREATE TABLE IF NOT EXISTS notification (
  id BIGSERIAL PRIMARY KEY,
  username VARCHAR(100) DEFAULT '',
  type VARCHAR(30) DEFAULT '',
  title VARCHAR(200) DEFAULT '',
  content TEXT DEFAULT '',
  is_read BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notification_user ON notification(username);
