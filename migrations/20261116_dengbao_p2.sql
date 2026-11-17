-- v23 线C 等保 P2 增量（2026-11-16）
-- R-C7 合规自查清单：GB/T 22239-2019 主要控制点自评（是/否/部分适用 + 证据链接）。
-- 口径：自查是「自评」，不替代测评机构现场测评；结果只作整改线索。
-- 幂等，可重复执行。

CREATE TABLE IF NOT EXISTS dengbao_selfcheck (
  id BIGSERIAL PRIMARY KEY,
  code VARCHAR(20) NOT NULL DEFAULT '',             -- 控制点编号（8.1.4 等）
  item VARCHAR(300) NOT NULL,                       -- 控制点名称
  domain VARCHAR(50) NOT NULL DEFAULT '',           -- 所属安全控制族（安全审计/身份鉴别/…）
  level VARCHAR(10) NOT NULL DEFAULT '2',           -- 适用级别 1/2/3（自查口径）
  result VARCHAR(12) NOT NULL DEFAULT 'todo',       -- todo/y/no/partial/na
  evidence TEXT NOT NULL DEFAULT '',                -- 证据链接或证据说明（导出时一并带出）
  note TEXT NOT NULL DEFAULT '',
  checked_by VARCHAR(100) NOT NULL DEFAULT '',
  checked_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_selfcheck_domain ON dengbao_selfcheck (domain);
CREATE INDEX IF NOT EXISTS idx_selfcheck_result ON dengbao_selfcheck (result);