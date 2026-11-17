-- A5 长期保存/国密合规封装层（2026-11-16，P2，2028-01-01 财会 9 号硬窗口）
-- 登记长期保存存证记录：SM3 摘要 + SM4 加密件路径 + 时间戳 + 版本控制。
-- 幂等，可重复执行。

CREATE TABLE IF NOT EXISTS longterm_archive (
  id BIGSERIAL PRIMARY KEY,
  source_type VARCHAR(30) NOT NULL,                    -- invoice / contract / voucher ...
  source_id BIGINT NOT NULL DEFAULT 0,                 -- 源对象 id
  version INT NOT NULL DEFAULT 1,                      -- 版本号（同源件重复归档自动 +1，保留全部历史）
  digest_alg VARCHAR(10) NOT NULL DEFAULT 'sm3',       -- 摘要算法
  digest VARCHAR(64) NOT NULL DEFAULT '',              -- SM3 摘要
  encrypt_alg VARCHAR(10) NOT NULL DEFAULT 'sm4',      -- 加密算法
  enc_path VARCHAR(512) NOT NULL DEFAULT '',           -- 加密件路径
  ts_token VARCHAR(64) NOT NULL DEFAULT '',            -- 时间戳存证 token（摘要前缀）
  ts_time VARCHAR(20) NOT NULL DEFAULT '',             -- 登记可信时间
  remark VARCHAR(300) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_longterm_source ON longterm_archive(source_type, source_id, version);
