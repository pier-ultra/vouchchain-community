-- 发票台账：三方匹配 + 四性归档（P0 免费版核心）
-- 幂等：全部 IF NOT EXISTS，重复执行无害

-- ---------- 三方匹配日志：发票 ↔ 合同 ↔ 付款 ----------
CREATE TABLE IF NOT EXISTS invoice_match_logs (
  id BIGSERIAL PRIMARY KEY,
  invoice_id BIGINT NOT NULL,
  contract_id BIGINT NOT NULL DEFAULT 0,
  payment_plan_id BIGINT NOT NULL DEFAULT 0,
  -- 匹配维度得分（0-100）
  score_tax_id INT NOT NULL DEFAULT 0,      -- 税号一致性
  score_name INT NOT NULL DEFAULT 0,        -- 名称相似度
  score_amount INT NOT NULL DEFAULT 0,      -- 金额一致性
  score_date INT NOT NULL DEFAULT 0,        -- 日期区间合理性
  score_item INT NOT NULL DEFAULT 0,        -- 品目/项目编码一致性
  total_score INT NOT NULL DEFAULT 0,       -- 加权总分
  match_status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending / matched / mismatch / manual
  match_rule VARCHAR(100) NOT NULL DEFAULT '',           -- 命中规则名
  matched_by VARCHAR(50) NOT NULL DEFAULT 'auto',        -- auto / manual
  remark TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_match_invoice ON invoice_match_logs(invoice_id);
CREATE INDEX IF NOT EXISTS idx_match_contract ON invoice_match_logs(contract_id);
CREATE INDEX IF NOT EXISTS idx_match_payment ON invoice_match_logs(payment_plan_id);
CREATE INDEX IF NOT EXISTS idx_match_status ON invoice_match_logs(match_status);

-- ---------- 四性归档审计日志：真实性/完整性/合规性/关联性 ----------
CREATE TABLE IF NOT EXISTS archive_audit_logs (
  id BIGSERIAL PRIMARY KEY,
  invoice_id BIGINT NOT NULL,
  audit_type VARCHAR(20) NOT NULL, -- authenticity / integrity / compliance / association
  field_name VARCHAR(100) NOT NULL DEFAULT '',
  expected_hash VARCHAR(64) NOT NULL DEFAULT '',
  actual_hash VARCHAR(64) NOT NULL DEFAULT '',
  check_pass BOOLEAN NOT NULL DEFAULT FALSE,
  check_detail JSONB NOT NULL DEFAULT '{}',
  operator VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_audit_invoice ON archive_audit_logs(invoice_id);
CREATE INDEX IF NOT EXISTS idx_audit_type ON archive_audit_logs(audit_type);

-- ---------- 匹配规则配置表（可扩展权重，免费版内置默认规则） ----------
CREATE TABLE IF NOT EXISTS invoice_match_rule (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  weight_tax_id INT NOT NULL DEFAULT 30,
  weight_name INT NOT NULL DEFAULT 20,
  weight_amount INT NOT NULL DEFAULT 30,
  weight_date INT NOT NULL DEFAULT 10,
  weight_item INT NOT NULL DEFAULT 10,
  threshold_matched INT NOT NULL DEFAULT 80,    -- >= 即 matched
  threshold_mismatch INT NOT NULL DEFAULT 40,   -- <= 即 mismatch
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 默认规则（免费版内置，不可删只可改权重）
INSERT INTO invoice_match_rule (name, weight_tax_id, weight_name, weight_amount, weight_date, weight_item, threshold_matched, threshold_mismatch, enabled, is_default)
VALUES ('default', 30, 20, 30, 10, 10, 80, 40, TRUE, TRUE)
ON CONFLICT DO NOTHING;