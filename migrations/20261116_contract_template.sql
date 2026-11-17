-- A6 合同模板库 + 变量套打（2026-11-16，P1）
-- 法大大/上上签核心收费点：模板库不限量。我方此前仅 AI 生成，补模板库管理 + {{变量}} 套打。
-- 幂等，可重复执行。

CREATE TABLE IF NOT EXISTS contract_template (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(200) NOT NULL,                              -- 模板名称
  category VARCHAR(50) NOT NULL DEFAULT '',                -- 适用合同类型（采购/销售/服务/租赁/保密等）
  content TEXT NOT NULL DEFAULT '',                        -- 模板文本，含 {{变量}} 占位符
  variables JSONB NOT NULL DEFAULT '[]',                   -- 变量清单 [{key,label,required,default}]
  remark VARCHAR(300) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_contract_template_cat ON contract_template(category);
