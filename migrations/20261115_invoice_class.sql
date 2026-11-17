-- 发票自动分类（用户 11-15 需求：规则配置 + AI 兜底 + 分类查询）
ALTER TABLE invoice ADD COLUMN IF NOT EXISTS category VARCHAR(50) NOT NULL DEFAULT '';
ALTER TABLE invoice ADD COLUMN IF NOT EXISTS category_source VARCHAR(10) NOT NULL DEFAULT ''; -- rule/ai/manual，空=未分类
CREATE TABLE IF NOT EXISTS invoice_class_rule (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,              -- 规则名（展示用）
  category VARCHAR(50) NOT NULL,           -- 命中后的分类
  field VARCHAR(20) NOT NULL DEFAULT 'any', -- any=扫描全部字段 / seller=开票方 / items=商品名
  keywords TEXT NOT NULL DEFAULT '',        -- 逗号分隔关键词
  priority INT NOT NULL DEFAULT 100,        -- 小者先匹配
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
