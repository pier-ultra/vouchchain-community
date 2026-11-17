-- v23 线B 用印 P2 增量（2026-11-16）
-- R-B11 电子签平台盖章记录回填：电子真章不做（CA 资质墙），
-- 但接了外部电子签平台的客户，盖章记录要能回填用印台账形成闭环。
-- 红线：未配置外部平台时明确失败，不谎报"已同步"。凭据只存库，不出接口、不进导出。
-- 幂等，可重复执行。

-- 外部电子签平台配置（单行，app 级）
CREATE TABLE IF NOT EXISTS seal_es_config (
  id BIGSERIAL PRIMARY KEY,
  platform VARCHAR(50) NOT NULL DEFAULT '',        -- 平台标识（如 esign-ali / esign-teng）
  endpoint VARCHAR(500) NOT NULL DEFAULT '',       -- 盖章记录查询地址（空=未配置 → 回填明确失败）
  token VARCHAR(200) NOT NULL DEFAULT '',          -- 凭据（不出接口/不进导出）
  enabled VARCHAR(10) NOT NULL DEFAULT 'off',      -- on/off
  remark VARCHAR(300) NOT NULL DEFAULT '',
  updated_by VARCHAR(100) NOT NULL DEFAULT '',
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
INSERT INTO seal_es_config (id, enabled) VALUES (1, 'off') ON CONFLICT (id) DO NOTHING;

-- 用印台账回填列（电子章四要素）
ALTER TABLE seal_apply ADD COLUMN IF NOT EXISTS es_platform VARCHAR(50) NOT NULL DEFAULT '';
ALTER TABLE seal_apply ADD COLUMN IF NOT EXISTS es_file_no VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE seal_apply ADD COLUMN IF NOT EXISTS es_signer VARCHAR(200) NOT NULL DEFAULT '';
ALTER TABLE seal_apply ADD COLUMN IF NOT EXISTS es_signed_at TIMESTAMP;

-- 回填记录（每次同步留痕，可追溯来源平台与外部流水号）
CREATE TABLE IF NOT EXISTS seal_es_record (
  id BIGSERIAL PRIMARY KEY,
  apply_id BIGINT NOT NULL,
  platform VARCHAR(50) NOT NULL DEFAULT '',
  ext_ref VARCHAR(200) NOT NULL DEFAULT '',        -- 外部流水号
  es_file_no VARCHAR(100) NOT NULL DEFAULT '',      -- 电子章/文件编号
  signer VARCHAR(200) NOT NULL DEFAULT '',
  signed_at TIMESTAMP,
  evidence_url VARCHAR(500) NOT NULL DEFAULT '',
  raw TEXT NOT NULL DEFAULT '',                    -- 平台原始返回（不含凭据）
  creator VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_seal_es_apply ON seal_es_record (apply_id);