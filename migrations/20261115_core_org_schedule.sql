-- Core 共用内核：K2 组织树 / K3 多组织法人主体 / K5 主数据雏形
-- ③ 发票线 P1：US-I9 定期自动归档调度（AI9.1–AI9.4）
-- 全部为开源社区版能力，无企业版分层、无 license 开关。幂等可重复执行。

-- ---------- K3 法人主体（AK3.1） ----------
CREATE TABLE IF NOT EXISTS legal_entity (
  id BIGSERIAL PRIMARY KEY,
  code VARCHAR(50) NOT NULL DEFAULT '',
  name VARCHAR(200) NOT NULL DEFAULT '',
  entity_type VARCHAR(30) NOT NULL DEFAULT 'subsidiary', -- headquarters / subsidiary / branch
  parent_id BIGINT NOT NULL DEFAULT 0,
  status VARCHAR(20) NOT NULL DEFAULT 'active',          -- active / disabled
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_le_code ON legal_entity(code) WHERE code <> '';
INSERT INTO legal_entity(code, name, entity_type, parent_id)
SELECT 'DEFAULT', '默认主体（总公司）', 'headquarters', 0
WHERE NOT EXISTS (SELECT 1 FROM legal_entity WHERE code = 'DEFAULT');

-- 当前主体上下文（AK3.2）：按用户名记录，前端也可用 X-Legal-Entity-Id 头无状态覆盖
CREATE TABLE IF NOT EXISTS user_context (
  username VARCHAR(100) PRIMARY KEY,
  legal_entity_id BIGINT NOT NULL DEFAULT 0,             -- 0 = 全部主体（未切换）
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ---------- K2 组织树（AK2.1–AK2.5） ----------
CREATE TABLE IF NOT EXISTS org_node (
  id BIGSERIAL PRIMARY KEY,
  legal_entity_id BIGINT NOT NULL DEFAULT 0,
  parent_id BIGINT NOT NULL DEFAULT 0,
  code VARCHAR(50) NOT NULL DEFAULT '',
  name VARCHAR(200) NOT NULL DEFAULT '',
  node_type VARCHAR(20) NOT NULL DEFAULT 'dept',         -- dept / position
  sort_order INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_org_code ON org_node(legal_entity_id, code) WHERE code <> '';
CREATE INDEX IF NOT EXISTS idx_org_parent ON org_node(legal_entity_id, parent_id);

CREATE TABLE IF NOT EXISTS org_member (
  id BIGSERIAL PRIMARY KEY,
  node_id BIGINT NOT NULL,
  legal_entity_id BIGINT NOT NULL DEFAULT 0,
  username VARCHAR(100) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_org_member ON org_member(node_id, username);
CREATE INDEX IF NOT EXISTS idx_org_member_entity ON org_member(legal_entity_id, username);

-- ---------- K5 主数据雏形（AK5.1–AK5.5） ----------
CREATE TABLE IF NOT EXISTS master_data_type (
  code VARCHAR(50) PRIMARY KEY,
  name VARCHAR(100) NOT NULL DEFAULT '',
  description VARCHAR(300) NOT NULL DEFAULT '',
  sort_order INT NOT NULL DEFAULT 0
);
INSERT INTO master_data_type(code, name, description, sort_order) VALUES
  ('customer', '客户', '合同相对方 / 发票购方', 1),
  ('supplier', '供应商', '合同相对方 / 发票销方 / 收款方', 2),
  ('material', '物料', '采购标的 / 发票明细行', 3),
  ('organization', '组织', '内部组织与法人主体引用', 4),
  ('person', '人员', '经办人 / 审批人 / 会计', 5)
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS master_data (
  id BIGSERIAL PRIMARY KEY,
  md_uid VARCHAR(80) NOT NULL,                          -- AK5.3 稳定唯一 ID，供 ①②③ 引用
  md_type VARCHAR(50) NOT NULL,
  code VARCHAR(100) NOT NULL DEFAULT '',
  name VARCHAR(200) NOT NULL DEFAULT '',
  legal_entity_id BIGINT NOT NULL DEFAULT 0,
  attributes JSONB NOT NULL DEFAULT '{}',
  status VARCHAR(20) NOT NULL DEFAULT 'active',         -- active / disabled
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_md_uid ON master_data(md_uid);
CREATE UNIQUE INDEX IF NOT EXISTS uk_md_type_code ON master_data(md_type, code, legal_entity_id) WHERE code <> '';
CREATE INDEX IF NOT EXISTS idx_md_type ON master_data(md_type, status);

CREATE TABLE IF NOT EXISTS master_data_link (
  id BIGSERIAL PRIMARY KEY,
  from_id BIGINT NOT NULL,
  to_id BIGINT NOT NULL,
  relation VARCHAR(50) NOT NULL DEFAULT 'belongs_to',   -- belongs_to / contact_of / supplies
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_md_link ON master_data_link(from_id, to_id, relation);

-- ---------- US-I9 归档调度（AI9.1–AI9.4） ----------
CREATE TABLE IF NOT EXISTS archive_schedule (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(200) NOT NULL DEFAULT '',
  period_type VARCHAR(20) NOT NULL DEFAULT 'monthly',   -- weekly / monthly / quarterly（账期）
  due_days INT NOT NULL DEFAULT 5,                      -- 账期结束后第 N 天执行
  scope_type VARCHAR(20) NOT NULL DEFAULT 'all',        -- all / legal_entity（归档范围）
  scope_id BIGINT NOT NULL DEFAULT 0,
  require_verified BOOLEAN NOT NULL DEFAULT TRUE,
  require_voucher BOOLEAN NOT NULL DEFAULT TRUE,
  require_paid BOOLEAN NOT NULL DEFAULT FALSE,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  next_run_at TIMESTAMP,
  last_run_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS archive_schedule_run (
  id BIGSERIAL PRIMARY KEY,
  schedule_id BIGINT NOT NULL,
  trigger VARCHAR(20) NOT NULL DEFAULT 'auto',          -- auto / manual
  status VARCHAR(20) NOT NULL DEFAULT 'running',        -- running / done / failed
  total INT NOT NULL DEFAULT 0,
  success INT NOT NULL DEFAULT 0,
  failed INT NOT NULL DEFAULT 0,
  skipped INT NOT NULL DEFAULT 0,
  cutoff_date VARCHAR(20) NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_asr_schedule ON archive_schedule_run(schedule_id, id DESC);

CREATE TABLE IF NOT EXISTS archive_schedule_item (
  id BIGSERIAL PRIMARY KEY,
  run_id BIGINT NOT NULL,
  invoice_id BIGINT NOT NULL,
  invoice_no VARCHAR(50) NOT NULL DEFAULT '',
  result VARCHAR(20) NOT NULL DEFAULT 'ok',             -- ok / failed / skipped
  message TEXT NOT NULL DEFAULT '',
  archive_path VARCHAR(512) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_asi_run ON archive_schedule_item(run_id);

-- AI9.4 失败项进入人工处理队列
CREATE TABLE IF NOT EXISTS archive_exception (
  id BIGSERIAL PRIMARY KEY,
  run_id BIGINT NOT NULL DEFAULT 0,
  invoice_id BIGINT NOT NULL,
  invoice_no VARCHAR(50) NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  status VARCHAR(20) NOT NULL DEFAULT 'pending',        -- pending / resolved / ignored
  handled_by VARCHAR(100) NOT NULL DEFAULT '',
  handled_note TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  handled_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ae_status ON archive_exception(status, id DESC);

-- ---------- AK3.4 底层归属标记（作为后续 AG5 强隔离的基础） ----------
-- 历史数据保持 0（未归属），在「全部主体」上下文可见；切换到具体主体后按该主体严格过滤。
ALTER TABLE contract ADD COLUMN IF NOT EXISTS legal_entity_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE contract_file ADD COLUMN IF NOT EXISTS legal_entity_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE invoice ADD COLUMN IF NOT EXISTS legal_entity_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE voucher ADD COLUMN IF NOT EXISTS legal_entity_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE payment_plan ADD COLUMN IF NOT EXISTS legal_entity_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE journal_draft ADD COLUMN IF NOT EXISTS legal_entity_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE bank_receipt ADD COLUMN IF NOT EXISTS legal_entity_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE bridge_log ADD COLUMN IF NOT EXISTS legal_entity_id BIGINT NOT NULL DEFAULT 0;
