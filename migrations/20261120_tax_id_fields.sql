-- 补齐 tax_id 字段：提升三方匹配税号维度准确度
-- 幂等：全部 IF NOT EXISTS，重复执行无害

-- invoice 表：销售方税号（从 OCR 解析或手工录入）
ALTER TABLE invoice ADD COLUMN IF NOT EXISTS seller_tax_id VARCHAR(50) NOT NULL DEFAULT '';
ALTER TABLE invoice ADD COLUMN IF NOT EXISTS buyer_tax_id VARCHAR(50) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_invoice_seller_tax_id ON invoice(seller_tax_id);
CREATE INDEX IF NOT EXISTS idx_invoice_buyer_tax_id ON invoice(buyer_tax_id);

-- contract 表：对方税号
ALTER TABLE contract ADD COLUMN IF NOT EXISTS counter_party_tax_id VARCHAR(50) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_contract_counter_party_tax_id ON contract(counter_party_tax_id);

-- payment_plan 表：收款方税号
ALTER TABLE payment_plan ADD COLUMN IF NOT EXISTS payee_tax_id VARCHAR(50) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_payment_plan_payee_tax_id ON payment_plan(payee_tax_id);

-- voucher 表：摘要/对方税号（可选，便于凭证关联时辅助匹配）
ALTER TABLE voucher ADD COLUMN IF NOT EXISTS counter_party_tax_id VARCHAR(50) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_voucher_counter_party_tax_id ON voucher(counter_party_tax_id);

-- bank_receipt 表：付款方/收款方税号（可选，便于回单匹配时辅助匹配）
ALTER TABLE bank_receipt ADD COLUMN IF NOT EXISTS payer_tax_id VARCHAR(50) NOT NULL DEFAULT '';
ALTER TABLE bank_receipt ADD COLUMN IF NOT EXISTS payee_tax_id VARCHAR(50) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_receipt_payer_tax_id ON bank_receipt(payer_tax_id);
CREATE INDEX IF NOT EXISTS idx_receipt_payee_tax_id ON bank_receipt(payee_tax_id);