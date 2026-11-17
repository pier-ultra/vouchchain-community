-- A2 发票红冲预警（2026-11-16，P1 规则层）
-- 红冲检测：红字发票（发票类型含"红"或价税合计为负）+ 蓝字被红冲（存在指向它的红字发票）。
-- 幂等，可重复执行。

ALTER TABLE invoice ADD COLUMN IF NOT EXISTS original_invoice_no VARCHAR(50) DEFAULT '';  -- 红字发票指向的原蓝字发票号码
ALTER TABLE invoice ADD COLUMN IF NOT EXISTS red_flush_status VARCHAR(20) DEFAULT '';    -- '' 正常 / red 红字发票 / reversed 被红冲
CREATE INDEX IF NOT EXISTS idx_invoice_redflush ON invoice(red_flush_status);
