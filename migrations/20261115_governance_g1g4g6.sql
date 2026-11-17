-- 产品② 企业治理底座：G1 RBAC 配置化 / G4 合规策略引擎 / G6 MDM 实体解析去重
-- 全部为开源社区版能力，无企业版分层、无 license 开关。幂等可重复执行。

-- ---------- G1 RBAC（AG1.1–AG1.6） ----------
CREATE TABLE IF NOT EXISTS rbac_role (
  id BIGSERIAL PRIMARY KEY,
  code VARCHAR(50) NOT NULL,
  name VARCHAR(100) NOT NULL DEFAULT '',
  description VARCHAR(300) NOT NULL DEFAULT '',
  is_builtin BOOLEAN NOT NULL DEFAULT FALSE,
  permissions TEXT NOT NULL DEFAULT '[]',       -- JSON 数组：资源模式 "POST /api/xxx"（内置角色为空=走静态规则兜底）
  data_scope VARCHAR(20) NOT NULL DEFAULT 'all', -- all / entity / dept / self
  field_mask TEXT NOT NULL DEFAULT '[]',       -- JSON 数组：不可见字段（AG1.2 字段级权限）
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_rbac_role_code ON rbac_role(code);

CREATE TABLE IF NOT EXISTS rbac_user_role (
  id BIGSERIAL PRIMARY KEY,
  username VARCHAR(100) NOT NULL,
  role_code VARCHAR(50) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_rbac_user_role ON rbac_user_role(username, role_code);

-- 预置角色（AG1.5）：内置角色不可删，权限沿用静态规则表，避免升级后越权/失权
INSERT INTO rbac_role(code, name, description, is_builtin, permissions, data_scope, field_mask) VALUES
  ('admin',    '系统管理员', '全部功能与数据权限',        TRUE, '[]', 'all', '[]'),
  ('legal',    '法务',       '合同/条款/归档/财务复核',   TRUE, '[]', 'all', '[]'),
  ('business', '业务',       '合同发起/查询/审批/发票',   TRUE, '[]', 'all', '[]'),
  ('viewer',   '只读',       '仅查看，不可写入',          TRUE, '[]', 'all', '[]')
ON CONFLICT (code) DO NOTHING;

-- ---------- G4 合规策略引擎（AG4.1–AG4.6） ----------
CREATE TABLE IF NOT EXISTS compliance_rule (
  id BIGSERIAL PRIMARY KEY,
  code VARCHAR(60) NOT NULL,
  name VARCHAR(150) NOT NULL DEFAULT '',
  description VARCHAR(400) NOT NULL DEFAULT '',
  rule_type VARCHAR(20) NOT NULL DEFAULT 'required',   -- required / format / relation / threshold
  object_type VARCHAR(40) NOT NULL DEFAULT 'contract',  -- contract / invoice / voucher / master_data
  field VARCHAR(80) NOT NULL DEFAULT '',
  op VARCHAR(20) NOT NULL DEFAULT '',                  -- threshold: gt/gte/lt/lte/eq；format: regex；relation: exists
  rule_value VARCHAR(300) NOT NULL DEFAULT '',         -- 阈值 / 正则 / 被关联字段
  condition_expr VARCHAR(200) NOT NULL DEFAULT '',    -- AG4.3 绑定状态/子类型：key=value，空=不限
  severity VARCHAR(20) NOT NULL DEFAULT 'warn',       -- block=拦截 / warn=告警留痕
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  version INT NOT NULL DEFAULT 1,
  doc VARCHAR(1000) NOT NULL DEFAULT '',              -- 说明文档（制度条款出处）
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_compliance_rule_code ON compliance_rule(code);

-- 规则版本历史（AG4.6）：每次编辑留一份快照
CREATE TABLE IF NOT EXISTS compliance_rule_history (
  id BIGSERIAL PRIMARY KEY,
  rule_id BIGINT NOT NULL,
  version INT NOT NULL,
  snapshot TEXT NOT NULL DEFAULT '{}',
  changed_by VARCHAR(100) NOT NULL DEFAULT '',
  changed_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_crh_rule ON compliance_rule_history(rule_id, version);

-- 违规记录（AG4.5）
CREATE TABLE IF NOT EXISTS compliance_violation (
  id BIGSERIAL PRIMARY KEY,
  rule_id BIGINT NOT NULL DEFAULT 0,
  rule_code VARCHAR(60) NOT NULL DEFAULT '',
  rule_name VARCHAR(150) NOT NULL DEFAULT '',
  object_type VARCHAR(40) NOT NULL DEFAULT '',
  object_id BIGINT NOT NULL DEFAULT 0,
  object_label VARCHAR(200) NOT NULL DEFAULT '',
  field VARCHAR(80) NOT NULL DEFAULT '',
  message VARCHAR(500) NOT NULL DEFAULT '',
  severity VARCHAR(20) NOT NULL DEFAULT 'warn',
  status VARCHAR(20) NOT NULL DEFAULT 'open',         -- open / resolved / waived
  owner VARCHAR(100) NOT NULL DEFAULT '',
  handled_by VARCHAR(100) NOT NULL DEFAULT '',
  handled_note VARCHAR(500) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  handled_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_cv_status ON compliance_violation(status, created_at DESC);

-- 预置规则库：制度条款落到可执行规则（默认全部启用；block 只用于明确不能放过的必填项）
INSERT INTO compliance_rule(code, name, description, rule_type, object_type, field, op, rule_value, condition_expr, severity, doc) VALUES
  ('CR-CONTRACT-PARTY', '合同相对方必填', '合同必须登记相对方，否则无法做履约与相对方风险归集',
   'required', 'contract', 'counterParty', '', '', '', 'block', 'v13 AC1.2 / 合同台账最低登记要求'),
  ('CR-CONTRACT-AMOUNT', '合同金额上限', '单份合同金额超过阈值需走专项审批，不允许直接登记',
   'threshold', 'contract', 'amount', 'lte', '1000000', '', 'block', '示例制度：超 100 万合同需专项审批'),
  ('CR-INVOICE-NO', '发票号格式', '数电票发票号应为 8–20 位数字',
   'format', 'invoice', 'invoiceNo', 'regex', '^[0-9]{8,20}$', 'verifyStatus=verified', 'warn', '电子发票基础字段校验（验真动作触发）'),
  ('CR-VOUCHER-AMOUNT', '凭证金额必须大于零', '零金额或负金额凭证不得入账',
   'threshold', 'voucher', 'amount', 'gt', '0', '', 'block', '会计基础规范：凭证金额有效性'),
  ('CR-MD-TAXNO', '供应商税号格式', '统一社会信用代码为 15–20 位大写字母或数字',
   'format', 'master_data', 'taxNo', 'regex', '^[A-Z0-9]{15,20}$', 'mdType=supplier', 'warn', '主数据质量规则（仅供应商类型生效）')
ON CONFLICT (code) DO NOTHING;
ALTER TABLE compliance_rule ADD COLUMN IF NOT EXISTS condition_expr VARCHAR(200) NOT NULL DEFAULT '';
UPDATE compliance_rule SET condition_expr = 'mdType=supplier' WHERE code = 'CR-MD-TAXNO' AND condition_expr = '';
UPDATE compliance_rule SET condition_expr = 'verifyStatus=verified' WHERE code = 'CR-INVOICE-NO' AND condition_expr = '';

-- ---------- G6 MDM 实体解析与去重（AG6.1–AG6.5） ----------
CREATE TABLE IF NOT EXISTS md_match_rule (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(150) NOT NULL DEFAULT '',
  md_type VARCHAR(50) NOT NULL DEFAULT 'supplier',
  fields TEXT NOT NULL DEFAULT '["name","taxNo"]',   -- JSON 数组：参与匹配的字段
  threshold NUMERIC(5,2) NOT NULL DEFAULT 70.00,    -- 相似度阈值
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
INSERT INTO md_match_rule(name, md_type, fields, threshold)
SELECT '供应商默认匹配规则', 'supplier', '["name","taxNo","address","phone"]', 68.00
WHERE NOT EXISTS (SELECT 1 FROM md_match_rule WHERE name = '供应商默认匹配规则');

CREATE TABLE IF NOT EXISTS md_duplicate_pair (
  id BIGSERIAL PRIMARY KEY,
  match_rule_id BIGINT NOT NULL DEFAULT 0,
  left_id BIGINT NOT NULL,
  right_id BIGINT NOT NULL,
  left_uid VARCHAR(80) NOT NULL DEFAULT '',
  right_uid VARCHAR(80) NOT NULL DEFAULT '',
  left_name VARCHAR(200) NOT NULL DEFAULT '',
  right_name VARCHAR(200) NOT NULL DEFAULT '',
  score NUMERIC(5,2) NOT NULL DEFAULT 0,
  reasons TEXT NOT NULL DEFAULT '[]',
  suggestion TEXT NOT NULL DEFAULT '',
  status VARCHAR(20) NOT NULL DEFAULT 'pending',     -- pending / merged / ignored
  handled_by VARCHAR(100) NOT NULL DEFAULT '',
  handled_note VARCHAR(500) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  handled_at TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_md_pair ON md_duplicate_pair(left_id, right_id);
CREATE INDEX IF NOT EXISTS idx_md_pair_status ON md_duplicate_pair(status, score DESC);

-- 权威版本与历史 ID 映射（AG6.3）：合并后被合并方稳定 ID 仍可反查
CREATE TABLE IF NOT EXISTS md_id_mapping (
  id BIGSERIAL PRIMARY KEY,
  old_id BIGINT NOT NULL,
  old_uid VARCHAR(80) NOT NULL DEFAULT '',
  old_name VARCHAR(200) NOT NULL DEFAULT '',
  survivor_id BIGINT NOT NULL,
  survivor_uid VARCHAR(80) NOT NULL DEFAULT '',
  pair_id BIGINT NOT NULL DEFAULT 0,
  merged_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_md_mapping_old ON md_id_mapping(old_id);

-- G1 数据权限（AG1.2/AG1.4）：业务表补创建人，供 self / dept 可见范围真实生效
ALTER TABLE contract ADD COLUMN IF NOT EXISTS owner VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE invoice ADD COLUMN IF NOT EXISTS owner VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE voucher ADD COLUMN IF NOT EXISTS owner VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE master_data ADD COLUMN IF NOT EXISTS owner VARCHAR(100) NOT NULL DEFAULT '';
