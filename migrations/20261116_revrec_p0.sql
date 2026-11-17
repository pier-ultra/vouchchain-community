-- v23 线A 收入确认（乙方轻量版）P0 补格（2026-11-16）
-- 本轮：R-A5 确认分录补成本结转（借主营业务成本/贷合同履约成本）+ 科目映射复用 account_mapping。
-- 铁律不变：AI/规则只出建议（status=draft），会计人审确认才落账（confirmed）；不做 SSP 自动分析/多币种/SOX/复杂变更追溯调整。
-- 幂等，可重复执行；服务启动迁移器按文件名顺序自动应用（20261115_revrec_seal.sql 之后）。

-- R-A5：收入确认记录补「本期确认成本」（成本结转分录的金额来源）
ALTER TABLE revrec_confirm ADD COLUMN IF NOT EXISTS cost NUMERIC(14,2) NOT NULL DEFAULT 0;

-- R-A5：收入确认科目映射（复用 account_mapping，规则/分录生成只从这张表取科目，不硬编码科目码）
-- 科目编码对照：合同结算 1131 / 主营业务收入 6001 / 主营业务成本 6401 / 合同履约成本 1471 / 应收账款 1122 / 销项税额 22210105
INSERT INTO account_mapping(category, subject_code, subject_name, direction) VALUES
  ('收入确认-合同结算',       '1131',     '合同结算',                                  'credit'),
  ('收入确认-主营业务收入',   '6001',     '主营业务收入',                              'credit'),
  ('收入确认-主营业务成本',   '6401',     '主营业务成本',                              'debit'),
  ('收入确认-合同履约成本',   '1471',     '合同履约成本',                              'credit'),
  ('收入确认-应收账款',       '1122',     '应收账款',                                  'debit'),
  ('收入确认-销项税额',       '22210105', '应交税费—应交增值税（销项税额）',            'credit')
ON CONFLICT (category, direction) DO NOTHING;
