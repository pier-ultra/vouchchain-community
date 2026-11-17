-- A 收入确认（乙方轻量版）+ B 用印台账（v23 R-A/R-B，2026-11-15）
-- A1: 义务挂确认方式与分摊
ALTER TABLE obligation ADD COLUMN IF NOT EXISTS confirm_mode VARCHAR(10) NOT NULL DEFAULT '';      -- point(时点法,默认)/time(时段法投入法)
ALTER TABLE obligation ADD COLUMN IF NOT EXISTS share_ratio NUMERIC(6,2) NOT NULL DEFAULT 0;      -- 分摊比例%（多义务按单独售价比例，手工维护）
ALTER TABLE obligation ADD COLUMN IF NOT EXISTS planned_cost NUMERIC(14,2) NOT NULL DEFAULT 0;    -- 时段法：预计总成本
ALTER TABLE obligation ADD COLUMN IF NOT EXISTS accum_cost NUMERIC(14,2) NOT NULL DEFAULT 0;      -- 时段法：累计已发生成本
ALTER TABLE obligation ADD COLUMN IF NOT EXISTS tax_rate NUMERIC(5,2) NOT NULL DEFAULT 13;        -- 销项税率%（货物13/服务6，可改）

-- A2: 收入确认记录（建议→人审确认铁律）
CREATE TABLE IF NOT EXISTS revrec_confirm (
  id BIGSERIAL PRIMARY KEY,
  obligation_id BIGINT NOT NULL,
  period VARCHAR(7) NOT NULL,                     -- 确认期间 YYYY-MM
  amount NUMERIC(14,2) NOT NULL,                  -- 本期确认收入（不含税）
  tax NUMERIC(14,2) NOT NULL DEFAULT 0,           -- 销项税额
  total NUMERIC(14,2) NOT NULL DEFAULT 0,         -- 价税合计
  progress NUMERIC(6,2) NOT NULL DEFAULT 100,     -- 时段法履约进度%
  entries TEXT NOT NULL DEFAULT '[]',             -- 分录 JSON（借应收/合同结算 贷收入+销项）
  source VARCHAR(10) NOT NULL DEFAULT 'rule',     -- rule/ai
  status VARCHAR(12) NOT NULL DEFAULT 'draft',    -- draft→confirmed（人审确认铁律）
  draft_id BIGINT NOT NULL DEFAULT 0,             -- 关联 journal_draft
  creator VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  confirmed_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_revrec_obl ON revrec_confirm (obligation_id);

-- A3: 回款登记（乙方收款，现金流日历数据源）
CREATE TABLE IF NOT EXISTS revrec_receipt (
  id BIGSERIAL PRIMARY KEY,
  contract_file_id BIGINT NOT NULL DEFAULT 0,
  obligation_id BIGINT NOT NULL DEFAULT 0,
  amount NUMERIC(14,2) NOT NULL,
  received_at DATE NOT NULL,
  remark VARCHAR(200) NOT NULL DEFAULT '',
  creator VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- B1: 印章台账
CREATE TABLE IF NOT EXISTS seal (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,                     -- 印章名称
  type VARCHAR(20) NOT NULL DEFAULT 'contract',   -- contract合同章/official公章/finance财务章/legal法人章/other
  keeper VARCHAR(100) NOT NULL DEFAULT '',        -- 保管人
  status VARCHAR(12) NOT NULL DEFAULT 'active',   -- active在用/disabled停用/destroyed销毁
  remark VARCHAR(200) NOT NULL DEFAULT '',
  creator VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- B2: 用印申请与登记（审批简化：legal/admin 批；七字段对齐商业口径）
CREATE TABLE IF NOT EXISTS seal_apply (
  id BIGSERIAL PRIMARY KEY,
  seal_id BIGINT NOT NULL,
  biz_type VARCHAR(30) NOT NULL DEFAULT 'contract', -- contract/other
  biz_id BIGINT NOT NULL DEFAULT 0,                 -- 关联合同文件 id 等
  file_id BIGINT NOT NULL DEFAULT 0,                -- file_entry（被盖章文件，哈希防篡改）
  copies INT NOT NULL DEFAULT 1,
  reason VARCHAR(300) NOT NULL DEFAULT '',
  status VARCHAR(12) NOT NULL DEFAULT 'pending',    -- pending待批/approved已批/stamped已用印/rejected驳回
  approver VARCHAR(100) NOT NULL DEFAULT '',
  approved_at TIMESTAMP,
  operator VARCHAR(100) NOT NULL DEFAULT '',        -- 用印操作人（七字段）
  stamped_at TIMESTAMP,                             -- 用印时间（七字段）
  device VARCHAR(120) NOT NULL DEFAULT '',          -- 来源设备/IP（七字段）
  file_hash VARCHAR(64) NOT NULL DEFAULT '',        -- 文件哈希（七字段）
  creator VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_seal_apply_seal ON seal_apply (seal_id);
