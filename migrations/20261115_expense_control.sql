-- 费控三件套（用户 11-15 拍板）：报销切片 / 费控预算 / 差旅标准
-- 1) 报销切片：一张发票拆多条报销记录（多方承担/多用途），合计不得超过发票价税合计
CREATE TABLE IF NOT EXISTS expense_split (
  id BIGSERIAL PRIMARY KEY,
  invoice_id BIGINT NOT NULL,
  slice_no INT NOT NULL DEFAULT 1,
  amount NUMERIC(14,2) NOT NULL,
  expense_type VARCHAR(50) NOT NULL DEFAULT '',   -- 用途（对齐发票分类：差旅交通/餐饮招待/...）
  dept_id BIGINT NOT NULL DEFAULT 0,              -- 归属部门（对齐 org_node）
  bearer VARCHAR(20) NOT NULL DEFAULT 'company',  -- company=公司承担 / personal=个人垫付
  remark VARCHAR(300) NOT NULL DEFAULT '',
  creator VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_expense_split_invoice ON expense_split (invoice_id);
CREATE INDEX IF NOT EXISTS idx_expense_split_dept ON expense_split (dept_id);

-- 2) 费控预算：部门+月份+分类 的上限；已用按切片金额聚合
CREATE TABLE IF NOT EXISTS budget (
  id BIGSERIAL PRIMARY KEY,
  dept_id BIGINT NOT NULL DEFAULT 0,
  period VARCHAR(7) NOT NULL,                     -- YYYY-MM
  category VARCHAR(50) NOT NULL,                  -- 对齐发票分类
  amount_limit NUMERIC(14,2) NOT NULL,
  remark VARCHAR(200) NOT NULL DEFAULT '',
  creator VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  UNIQUE (dept_id, period, category)
);

-- 3) 差旅标准：分类内单笔上限（住宿/交通/...），发票或切片金额超限即预警
CREATE TABLE IF NOT EXISTS travel_standard (
  id BIGSERIAL PRIMARY KEY,
  category VARCHAR(50) NOT NULL,                  -- 差旅交通/餐饮招待/住宿...
  limit_amount NUMERIC(14,2) NOT NULL,            -- 单笔上限
  note VARCHAR(200) NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  creator VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
