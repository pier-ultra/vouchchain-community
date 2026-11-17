-- v23 线A 收入确认（乙方轻量版）P1 补格（2026-11-16）
-- R-A6 增强：结算登记（出结算单）——三条线口径：确认（权责发生）/ 结算（出单）/ 收款（实收）。
-- R-A6 差额口径：差额 = 已确认 − 已结算；正 = 合同资产（干得多结算得少），负 = 合同负债（先结算/干得少）。
-- R-A7 应收实收差异、R-A11 月度视图（确认/结算/收款三条线按月）。
-- 幂等，可重复执行；服务启动迁移器按文件名顺序自动应用（20261116_revrec_p0.sql 之后）。

CREATE TABLE IF NOT EXISTS revrec_settlement (
  id BIGSERIAL PRIMARY KEY,
  contract_file_id BIGINT NOT NULL DEFAULT 0,
  obligation_id BIGINT NOT NULL DEFAULT 0,
  amount NUMERIC(14,2) NOT NULL,
  settled_at DATE NOT NULL,
  remark VARCHAR(200) NOT NULL DEFAULT '',
  creator VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_revrec_settle_cf ON revrec_settlement (contract_file_id);
