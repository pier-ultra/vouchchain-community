-- P2P 闭环补格：应付台账 / AI 分录草稿 / 科目映射 / 总账桥接留痕
-- 全部为社区版能力，开源免费，无企业版分层、无 license 开关。
-- 幂等，可重复执行；由服务启动时自动迁移器按文件名顺序执行。

-- ③ 付款计划 / 应付台账：义务→发票→打款钉死
CREATE TABLE IF NOT EXISTS payment_plan (
  id BIGSERIAL PRIMARY KEY,
  invoice_id BIGINT NOT NULL DEFAULT 0,
  obligation_id BIGINT NOT NULL DEFAULT 0,
  payee VARCHAR(200) DEFAULT '',
  amount NUMERIC(14,2) DEFAULT 0,
  due_date VARCHAR(20) DEFAULT '',
  status VARCHAR(20) DEFAULT 'planned',       -- planned / paid / cancelled
  paid_date VARCHAR(20) DEFAULT '',
  receipt_id BIGINT DEFAULT 0,
  remark TEXT DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_payplan_invoice ON payment_plan(invoice_id) WHERE invoice_id > 0;
CREATE INDEX IF NOT EXISTS idx_payplan_status ON payment_plan(status);

-- ④ AI 分录草稿：AI 只出建议，必须会计确认才写正式凭证（人审闭环，不碰总账）
CREATE TABLE IF NOT EXISTS journal_draft (
  id BIGSERIAL PRIMARY KEY,
  invoice_id BIGINT NOT NULL,
  category VARCHAR(100) DEFAULT '',
  entries JSONB NOT NULL DEFAULT '[]',
  source VARCHAR(30) DEFAULT 'rule',          -- llm / rule / rule_fallback
  reason TEXT DEFAULT '',
  status VARCHAR(20) DEFAULT 'draft',         -- draft / confirmed / rejected
  voucher_id BIGINT DEFAULT 0,
  confirmed_by VARCHAR(100) DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_journal_invoice ON journal_draft(invoice_id);

-- 科目映射：费用类型 ↔ 会计科目（可编辑，AI 只能从这张表里选科目，不许编造）
CREATE TABLE IF NOT EXISTS account_mapping (
  id BIGSERIAL PRIMARY KEY,
  category VARCHAR(100) NOT NULL,
  subject_code VARCHAR(50) DEFAULT '',
  subject_name VARCHAR(100) DEFAULT '',
  direction VARCHAR(10) DEFAULT 'debit',      -- debit / credit
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_acct_category ON account_mapping(category, direction);
INSERT INTO account_mapping(category, subject_code, subject_name, direction) VALUES
  ('货物采购', '1401', '原材料/库存商品', 'debit'),
  ('服务采购', '6602', '管理费用', 'debit'),
  ('进项税额', '22210101', '应交税费—应交增值税（进项税额）', 'debit'),
  ('应付账款', '2202', '应付账款', 'credit')
ON CONFLICT (category, direction) DO NOTHING;

-- 总账桥接留痕（导出 / 推送，含失败原因，绝不谎报成功）
-- 对标：费控王把 ERP 连接器放企业版收费，我们开源版直接内嵌。
CREATE TABLE IF NOT EXISTS bridge_log (
  id BIGSERIAL PRIMARY KEY,
  mode VARCHAR(20) DEFAULT '',                -- export / push
  target VARCHAR(50) DEFAULT '',              -- kingdee_k3 / yonyou_u8 / kingdee_cloud
  voucher_id BIGINT DEFAULT 0,
  status VARCHAR(20) DEFAULT '',             -- ok / failed
  detail TEXT DEFAULT '',
  file_path VARCHAR(512) DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
