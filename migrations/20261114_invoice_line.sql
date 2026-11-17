-- 发票线迁移（US-I1~I8；幂等可重复执行）
CREATE TABLE IF NOT EXISTS invoice (
  id BIGSERIAL PRIMARY KEY,
  invoice_no VARCHAR(50) DEFAULT '',
  issue_date VARCHAR(20) DEFAULT '',
  buyer_name VARCHAR(200) DEFAULT '',
  seller_name VARCHAR(200) DEFAULT '',
  amount NUMERIC(14,2) DEFAULT 0,
  tax NUMERIC(14,2) DEFAULT 0,
  total NUMERIC(14,2) DEFAULT 0,
  invoice_type VARCHAR(50) DEFAULT '',
  items JSONB DEFAULT '[]',
  file_path VARCHAR(512) DEFAULT '',
  file_name VARCHAR(255) DEFAULT '',
  file_hash VARCHAR(64) DEFAULT '',
  verify_status VARCHAR(20) DEFAULT 'unverified',
  verify_remark TEXT DEFAULT '',
  integrity_report JSONB DEFAULT '[]',
  archive_status VARCHAR(20) DEFAULT 'none',
  archive_path VARCHAR(512) DEFAULT '',
  voucher_id BIGINT DEFAULT 0,
  remark TEXT DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_invoice_no ON invoice(invoice_no) WHERE invoice_no <> '';
CREATE INDEX IF NOT EXISTS idx_invoice_hash ON invoice(file_hash);
CREATE TABLE IF NOT EXISTS voucher (
  id BIGSERIAL PRIMARY KEY,
  voucher_no VARCHAR(50) DEFAULT '',
  summary TEXT DEFAULT '',
  amount NUMERIC(14,2) DEFAULT 0,
  voucher_date VARCHAR(20) DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS bank_receipt (
  id BIGSERIAL PRIMARY KEY,
  payer VARCHAR(200) DEFAULT '',
  payee VARCHAR(200) DEFAULT '',
  amount NUMERIC(14,2) DEFAULT 0,
  receipt_date VARCHAR(20) DEFAULT '',
  matched_invoice_id BIGINT DEFAULT 0,
  match_rule VARCHAR(100) DEFAULT '',
  remark TEXT DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_receipt_invoice ON bank_receipt(matched_invoice_id);
