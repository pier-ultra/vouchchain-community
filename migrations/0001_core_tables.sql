-- 核心表补齐（冷启动必需）
--
-- 背景：项目的核心表早期在真实库中手工演进，从未进入 migrations/。
-- 任何全新数据库（PostgreSQL 或 SQLite）冷启动时都会失败：
--   1) contract / contract_file / obligation / contract_clause / sign_task / model_config / audit_log
--      —— 从未被任何迁移 CREATE，20261114_approval_notification.sql 的
--         `ALTER TABLE contract_file ADD COLUMN ...` 会因表不存在而中断整个启动。
--   2) payment_plan / journal_draft / master_data / bridge_log
--      —— 建表定义在 20261115_p2p_bridge.sql / 20261115_core_org_schedule.sql，
--         但 20261115_ag5_flow_party.sql 的 ALTER 排在它们之前，冷启动跑到 ALTER 时表还不存在。
-- 本文件按文件名排序最先执行，补齐上述表定义，使冷启动真正可用。
-- 幂等：全部 CREATE ... IF NOT EXISTS，重复执行无害（已存在即跳过，不影响既有库结构）。
-- 方言：BIGSERIAL 在 SQLite 下由 migrate.go 自动收敛为 INTEGER（rowid 别名即自增）；
--       JSONB 收敛为 TEXT；ALTER ADD COLUMN 的 IF NOT EXISTS 由执行层做列存在性检查补齐。
-- 列口径以 internal/entity/entity.go 的结构体为准，含各迁移后续 ALTER 追加的列（不重复添加）。

-- ---------- 合同台账 ----------
CREATE TABLE IF NOT EXISTS contract (
  id BIGSERIAL PRIMARY KEY,
  title VARCHAR(200) NOT NULL DEFAULT '',
  counter_party VARCHAR(200) NOT NULL DEFAULT '',
  amount NUMERIC NOT NULL DEFAULT 0,
  currency VARCHAR(10) NOT NULL DEFAULT 'CNY',
  sign_date VARCHAR(20) NOT NULL DEFAULT '',
  status VARCHAR(30) NOT NULL DEFAULT 'draft',
  legal_entity_id BIGINT NOT NULL DEFAULT 0,
  dept_id BIGINT NOT NULL DEFAULT 0,
  project_code VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_contract_cp ON contract(counter_party);
CREATE INDEX IF NOT EXISTS idx_contract_status ON contract(status);

-- ---------- 合同文件 + AI 抽取流水线 ----------
CREATE TABLE IF NOT EXISTS contract_file (
  id BIGSERIAL PRIMARY KEY,
  filename VARCHAR(300) NOT NULL DEFAULT '',
  name VARCHAR(300) NOT NULL DEFAULT '',
  path VARCHAR(500) NOT NULL DEFAULT '',
  file_hash VARCHAR(128) NOT NULL DEFAULT '',
  file_id BIGINT NOT NULL DEFAULT 0,
  status VARCHAR(30) NOT NULL DEFAULT 'pending',   -- pending / extracting / done / failed
  error_msg TEXT NOT NULL DEFAULT '',
  stage VARCHAR(30) NOT NULL DEFAULT '',
  progress INT NOT NULL DEFAULT 0,                   -- 总进度 0-100
  pipeline JSONB NOT NULL DEFAULT '{}',              -- 各阶段明细
  review_state VARCHAR(30) NOT NULL DEFAULT '',       -- in_progress / complete（复核完成门槛）
  template VARCHAR(50) NOT NULL DEFAULT '',          -- general / purchase / sales / lease
  group_id BIGINT NOT NULL DEFAULT 0,                -- 版本组：组内第一个文件的 id（0=自身即组首）
  version_no INT NOT NULL DEFAULT 1,
  approval_state VARCHAR(20) NOT NULL DEFAULT 'none', -- none / running / approved / rejected
  provider VARCHAR(50) NOT NULL DEFAULT '',
  text_content TEXT NOT NULL DEFAULT '',
  counter_party VARCHAR(200) NOT NULL DEFAULT '',
  legal_entity_id BIGINT NOT NULL DEFAULT 0,
  dept_id BIGINT NOT NULL DEFAULT 0,
  project_code VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_contract_file_cp ON contract_file(counter_party);
CREATE INDEX IF NOT EXISTS idx_contract_file_status ON contract_file(status);
CREATE INDEX IF NOT EXISTS idx_contract_file_group ON contract_file(group_id);

-- ---------- 抽取出的条款条目 ----------
CREATE TABLE IF NOT EXISTS contract_clause (
  id BIGSERIAL PRIMARY KEY,
  file_id BIGINT NOT NULL DEFAULT 0,
  type VARCHAR(50) NOT NULL DEFAULT '',               -- 主体/权利义务/金额/日期/违约/保密/争议解决/其他
  summary TEXT NOT NULL DEFAULT '',
  quote TEXT NOT NULL DEFAULT '',
  risk VARCHAR(10) NOT NULL DEFAULT 'none',           -- high / medium / low / none
  review_status VARCHAR(20) NOT NULL DEFAULT 'unconfirmed', -- unconfirmed / confirmed / doubt
  review_remark TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_clause_file ON contract_clause(file_id);

-- ---------- 义务清单 ----------
CREATE TABLE IF NOT EXISTS obligation (
  id BIGSERIAL PRIMARY KEY,
  file_id BIGINT NOT NULL DEFAULT 0,
  type VARCHAR(50) NOT NULL DEFAULT '',               -- 付款 / 交付 / 报告 / 续约 / 其他
  description TEXT NOT NULL DEFAULT '',
  owner VARCHAR(100) NOT NULL DEFAULT '',
  due_date VARCHAR(20) NOT NULL DEFAULT '',           -- YYYY-MM-DD
  source_quote TEXT NOT NULL DEFAULT '',
  status VARCHAR(20) NOT NULL DEFAULT 'pending',      -- pending / done
  remark TEXT NOT NULL DEFAULT '',
  evidence_name VARCHAR(300) NOT NULL DEFAULT '',
  evidence_path VARCHAR(500) NOT NULL DEFAULT '',
  completed_at TIMESTAMP,
  confirm_mode VARCHAR(20) NOT NULL DEFAULT '',
  share_ratio NUMERIC NOT NULL DEFAULT 0,
  tax_rate NUMERIC NOT NULL DEFAULT 0,
  planned_cost NUMERIC NOT NULL DEFAULT 0,
  accum_cost NUMERIC NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_obligation_file ON obligation(file_id);
CREATE INDEX IF NOT EXISTS idx_obligation_due ON obligation(due_date);
CREATE INDEX IF NOT EXISTS idx_obligation_status ON obligation(status);

-- ---------- 电子签任务 ----------
CREATE TABLE IF NOT EXISTS sign_task (
  id BIGSERIAL PRIMARY KEY,
  file_id BIGINT NOT NULL DEFAULT 0,
  provider VARCHAR(50) NOT NULL DEFAULT 'mock',        -- mock / docuseal / 商业CA
  status VARCHAR(20) NOT NULL DEFAULT 'created',      -- created / sent / signed / failed
  external_id VARCHAR(200) NOT NULL DEFAULT '',
  fail_msg TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sign_task_file ON sign_task(file_id);

-- ---------- AI/ML 接入配置 ----------
CREATE TABLE IF NOT EXISTS model_config (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL DEFAULT '',
  category VARCHAR(30) NOT NULL DEFAULT '',           -- llm / ocr / rerank / embedding / vector / esign
  provider VARCHAR(100) NOT NULL DEFAULT '',
  base_url VARCHAR(500) NOT NULL DEFAULT '',
  model_name VARCHAR(200) NOT NULL DEFAULT '',
  api_key TEXT NOT NULL DEFAULT '',                    -- 敏感：存 enc:v1: 密文（AES-256-GCM），需 APP_AES_KEY
  extra_config JSONB NOT NULL DEFAULT '{}',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_model_config_cat ON model_config(category);

-- ---------- 审计日志（五要素 + 来源 IP）----------
CREATE TABLE IF NOT EXISTS audit_log (
  id BIGSERIAL PRIMARY KEY,
  username VARCHAR(100) NOT NULL DEFAULT '',
  action VARCHAR(100) NOT NULL DEFAULT '',
  target_type VARCHAR(50) NOT NULL DEFAULT '',
  target_id BIGINT NOT NULL DEFAULT 0,
  detail JSONB NOT NULL DEFAULT '{}',
  ip VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_audit_log_user ON audit_log(username);
CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_log_target ON audit_log(target_type, target_id);

-- ---------- 应付付款计划（建表原在 20261115_p2p_bridge.sql，此处提前以解决顺序依赖）----------
CREATE TABLE IF NOT EXISTS payment_plan (
  id BIGSERIAL PRIMARY KEY,
  invoice_id BIGINT NOT NULL DEFAULT 0,
  obligation_id BIGINT NOT NULL DEFAULT 0,
  payee VARCHAR(200) NOT NULL DEFAULT '',
  amount NUMERIC NOT NULL DEFAULT 0,
  due_date VARCHAR(20) NOT NULL DEFAULT '',
  status VARCHAR(20) NOT NULL DEFAULT 'planned',      -- planned / paid / cancelled
  paid_date VARCHAR(20) NOT NULL DEFAULT '',
  receipt_id BIGINT NOT NULL DEFAULT 0,
  remark TEXT NOT NULL DEFAULT '',
  dept_id BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_payplan_status ON payment_plan(status);

-- ---------- AI 分录草稿（人审闭环，不碰总账；建表原在 20261115_p2p_bridge.sql）----------
CREATE TABLE IF NOT EXISTS journal_draft (
  id BIGSERIAL PRIMARY KEY,
  invoice_id BIGINT NOT NULL DEFAULT 0,
  category VARCHAR(100) NOT NULL DEFAULT '',
  entries JSONB NOT NULL DEFAULT '[]',
  source VARCHAR(30) NOT NULL DEFAULT 'rule',         -- llm / rule / rule_fallback
  reason TEXT NOT NULL DEFAULT '',
  status VARCHAR(20) NOT NULL DEFAULT 'draft',        -- draft / confirmed / rejected
  voucher_id BIGINT NOT NULL DEFAULT 0,
  confirmed_by VARCHAR(100) NOT NULL DEFAULT '',
  dept_id BIGINT NOT NULL DEFAULT 0,
  legal_entity_id BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_journal_invoice ON journal_draft(invoice_id);

-- ---------- 主数据（建表原在 20261115_core_org_schedule.sql）----------
CREATE TABLE IF NOT EXISTS master_data (
  id BIGSERIAL PRIMARY KEY,
  md_uid VARCHAR(80) NOT NULL,                        -- 稳定唯一 ID，供各产品引用
  md_type VARCHAR(50) NOT NULL,
  code VARCHAR(100) NOT NULL DEFAULT '',
  name VARCHAR(200) NOT NULL DEFAULT '',
  legal_entity_id BIGINT NOT NULL DEFAULT 0,
  attributes JSONB NOT NULL DEFAULT '{}',
  status VARCHAR(20) NOT NULL DEFAULT 'active',       -- active / disabled
  dept_id BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_md_uid ON master_data(md_uid);
CREATE INDEX IF NOT EXISTS idx_md_type ON master_data(md_type, status);

-- ---------- 总账桥接日志（建表原在 20261115_p2p_bridge.sql）----------
CREATE TABLE IF NOT EXISTS bridge_log (
  id BIGSERIAL PRIMARY KEY,
  mode VARCHAR(20) NOT NULL DEFAULT '',              -- export / push
  target VARCHAR(50) NOT NULL DEFAULT '',            -- kingdee_k3 / yonyou_u8 / kingdee_cloud
  voucher_id BIGINT NOT NULL DEFAULT 0,
  status VARCHAR(20) NOT NULL DEFAULT '',            -- ok / failed
  detail TEXT NOT NULL DEFAULT '',
  file_path VARCHAR(512) NOT NULL DEFAULT '',
  legal_entity_id BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
