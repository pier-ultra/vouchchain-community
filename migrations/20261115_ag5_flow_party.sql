-- 产品② AG5 强隔离 / US-G3 审批增强 / 产品① US-C8 相对方画像
-- 开源社区版能力，无企业版分层、无 license 开关。幂等可重复执行。

-- ---------- AG5 多维度数据隔离策略（AG5.1/AG5.2/AG5.5） ----------
CREATE TABLE IF NOT EXISTS isolation_policy (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(150) NOT NULL DEFAULT '',
  dimension VARCHAR(20) NOT NULL DEFAULT 'entity',   -- entity / dept / project
  subject_type VARCHAR(20) NOT NULL DEFAULT 'role',  -- user / role / all
  subject VARCHAR(100) NOT NULL DEFAULT '',          -- 用户名 / 角色码 / all 时留空
  allowed_values TEXT NOT NULL DEFAULT '[]',         -- JSON 数组：主体 ID / 组织节点 ID / 项目编码；"0" 代表未归属历史数据
  include_children BOOLEAN NOT NULL DEFAULT FALSE,   -- 法人/部门维度：允许值沿父子树递归展开
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  priority INT NOT NULL DEFAULT 0,
  created_by VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_iso_policy_subject ON isolation_policy(subject_type, subject, enabled);

-- AG5.4 越权访问尝试：拦截 + 告警留痕（可人工处理结案）
CREATE TABLE IF NOT EXISTS isolation_violation (
  id BIGSERIAL PRIMARY KEY,
  username VARCHAR(100) NOT NULL DEFAULT '',
  role VARCHAR(50) NOT NULL DEFAULT '',
  method VARCHAR(10) NOT NULL DEFAULT '',
  path VARCHAR(300) NOT NULL DEFAULT '',
  resource VARCHAR(60) NOT NULL DEFAULT '',
  object_id BIGINT NOT NULL DEFAULT 0,
  dimension VARCHAR(20) NOT NULL DEFAULT '',
  attempted_value VARCHAR(100) NOT NULL DEFAULT '',
  policy_ids TEXT NOT NULL DEFAULT '[]',
  status VARCHAR(20) NOT NULL DEFAULT 'open',        -- open / resolved / waived
  handled_by VARCHAR(100) NOT NULL DEFAULT '',
  handled_note VARCHAR(500) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  handled_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_iso_violation_status ON isolation_violation(status, created_at DESC);

-- 预置示例（默认停用，避免升级后立刻收紧演示数据；管理员在页面启用）
INSERT INTO isolation_policy(name, dimension, subject_type, subject, allowed_values, include_children, enabled, priority)
SELECT '示例：业务角色仅华东主体（含下级）', 'entity', 'role', 'business', '[1]', TRUE, FALSE, 10
WHERE NOT EXISTS (SELECT 1 FROM isolation_policy WHERE name = '示例：业务角色仅华东主体（含下级）');
INSERT INTO isolation_policy(name, dimension, subject_type, subject, allowed_values, include_children, enabled, priority)
SELECT '示例：法务按项目隔离', 'project', 'user', 'legal_wang', '["PRJ-A"]', FALSE, FALSE, 20
WHERE NOT EXISTS (SELECT 1 FROM isolation_policy WHERE name = '示例：法务按项目隔离');

-- 业务表补 AG5 隔离维度归属列（K3 已有 legal_entity_id，这里补部门/项目）
ALTER TABLE contract ADD COLUMN IF NOT EXISTS dept_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE contract ADD COLUMN IF NOT EXISTS project_code VARCHAR(80) NOT NULL DEFAULT '';
ALTER TABLE contract_file ADD COLUMN IF NOT EXISTS dept_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE contract_file ADD COLUMN IF NOT EXISTS project_code VARCHAR(80) NOT NULL DEFAULT '';
ALTER TABLE invoice ADD COLUMN IF NOT EXISTS dept_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE invoice ADD COLUMN IF NOT EXISTS project_code VARCHAR(80) NOT NULL DEFAULT '';
ALTER TABLE voucher ADD COLUMN IF NOT EXISTS dept_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE voucher ADD COLUMN IF NOT EXISTS project_code VARCHAR(80) NOT NULL DEFAULT '';
ALTER TABLE payment_plan ADD COLUMN IF NOT EXISTS dept_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE payment_plan ADD COLUMN IF NOT EXISTS project_code VARCHAR(80) NOT NULL DEFAULT '';
ALTER TABLE journal_draft ADD COLUMN IF NOT EXISTS dept_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE journal_draft ADD COLUMN IF NOT EXISTS project_code VARCHAR(80) NOT NULL DEFAULT '';
ALTER TABLE master_data ADD COLUMN IF NOT EXISTS dept_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE master_data ADD COLUMN IF NOT EXISTS project_code VARCHAR(80) NOT NULL DEFAULT '';
ALTER TABLE bank_receipt ADD COLUMN IF NOT EXISTS dept_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE bank_receipt ADD COLUMN IF NOT EXISTS project_code VARCHAR(80) NOT NULL DEFAULT '';

-- ---------- US-C8 合同文件与相对方关联（存量文件可人工补绑） ----------
ALTER TABLE contract_file ADD COLUMN IF NOT EXISTS counter_party VARCHAR(200) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_contract_file_party ON contract_file(counter_party);

-- ---------- US-G3 审批增强：条件分支上下文 / 加签来源 / 全程留痕 ----------
ALTER TABLE approval_instance ADD COLUMN IF NOT EXISTS context JSONB NOT NULL DEFAULT '{}';
ALTER TABLE approval_instance ADD COLUMN IF NOT EXISTS revoked_by VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE approval_instance ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMP;
ALTER TABLE approval_node ADD COLUMN IF NOT EXISTS origin VARCHAR(20) NOT NULL DEFAULT 'template'; -- template / addsign / transfer
ALTER TABLE approval_node ADD COLUMN IF NOT EXISTS when_expr VARCHAR(300) NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS approval_event (
  id BIGSERIAL PRIMARY KEY,
  instance_id BIGINT NOT NULL,
  seq INT NOT NULL DEFAULT 0,
  actor VARCHAR(100) NOT NULL DEFAULT '',
  action VARCHAR(30) NOT NULL DEFAULT '',       -- create / approve / reject / skip / transfer / addsign / revoke
  from_user VARCHAR(100) NOT NULL DEFAULT '',
  to_user VARCHAR(100) NOT NULL DEFAULT '',
  comment TEXT NOT NULL DEFAULT '',
  detail TEXT NOT NULL DEFAULT '{}',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_approval_event_inst ON approval_event(instance_id, id);
