-- 文件中心：统一文件表（双后端 local/s3，用户 11-15 拍板兼容模式）
CREATE TABLE IF NOT EXISTS file_entry (
  id BIGSERIAL PRIMARY KEY,
  biz_type VARCHAR(50) NOT NULL DEFAULT 'other',   -- contract/invoice/receipt/kb/other
  biz_id BIGINT NOT NULL DEFAULT 0,                -- 关联业务 id（0=未挂接）
  filename VARCHAR(300) NOT NULL,
  content_type VARCHAR(100) NOT NULL DEFAULT 'application/octet-stream',
  size_bytes BIGINT NOT NULL DEFAULT 0,
  sha256 CHAR(64) NOT NULL DEFAULT '',             -- 去重
  storage VARCHAR(10) NOT NULL DEFAULT 'local',    -- local/s3
  storage_key VARCHAR(400) NOT NULL,
  uploader VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_file_entry_biz ON file_entry (biz_type, biz_id);
CREATE INDEX IF NOT EXISTS idx_file_entry_sha ON file_entry (sha256);
