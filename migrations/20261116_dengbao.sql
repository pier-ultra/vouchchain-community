-- v23 线C 等保证据链（企业侧）P0+P1（2026-11-16）
-- 定位：被测企业的日常合规证据管理（平时攒证据、测评时拿得出来）。
-- 明确不做：测评执行（JSecProbe 的领域）、安全设备纳管（SIEM 的领域）、密评密码改造。
-- 不建新审计表——复用既有 audit_log（AC6.5/AK4.x 五要素），只在上面补「留存策略 + 到期归档 + 按等保口径导出」。
-- 幂等，可重复执行；启动迁移器按文件名顺序自动应用。

-- R-C2：留存策略（单行配置，app 级一份）
-- 等保 2.0（GB/T 22239-2019 安全审计）要求审计日志留存不少于 6 个月 → 下限 180 天。
CREATE TABLE IF NOT EXISTS dengbao_policy (
  id BIGSERIAL PRIMARY KEY,
  retention_days INTEGER NOT NULL DEFAULT 180,        -- 审计日志留存期限（天），<180 由代码层拒绝
  level VARCHAR(10) NOT NULL DEFAULT '2',             -- 拟保测评级 1/2/3（自查口径，不替代测评）
  updated_by VARCHAR(100) NOT NULL DEFAULT '',
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
INSERT INTO dengbao_policy (id, retention_days, level) VALUES (1, 180, '2')
  ON CONFLICT (id) DO NOTHING;

-- R-C2：到期审计日志归档表（与 audit_log 同构 + archived_at；归档=搬走不是删除，总留存期不变）
CREATE TABLE IF NOT EXISTS audit_log_archive (
  id BIGINT NOT NULL,                 -- 原 audit_log.id（查重用）
  username VARCHAR(200) DEFAULT '',
  action VARCHAR(200) DEFAULT '',
  target_type VARCHAR(100) DEFAULT '',
  target_id BIGINT DEFAULT 0,
  detail TEXT DEFAULT '',
  ip VARCHAR(64) DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),        -- 原日志时间（证据链连续性靠它算）
  archived_at TIMESTAMP NOT NULL DEFAULT NOW()       -- 归档动作时间（本身也是留痕）
);
CREATE UNIQUE INDEX IF NOT EXISTS uniq_audit_archive_id ON audit_log_archive(id);
CREATE INDEX IF NOT EXISTS idx_audit_archive_created ON audit_log_archive(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_archive_action ON audit_log_archive(action);

-- R-C1：等保资产台账（对标 JSecProbe 的系统构成录入，企业侧简化版；只登记不纳管）
CREATE TABLE IF NOT EXISTS dengbao_asset (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(200) NOT NULL,                          -- 系统/设备/应用名称
  category VARCHAR(20) NOT NULL DEFAULT 'application', -- application应用/system系统/device设备
  level VARCHAR(10) NOT NULL DEFAULT '2',              -- 拟保级别 1/2/3
  critical VARCHAR(20) NOT NULL DEFAULT 'normal',      -- critical重要/normal一般
  owner VARCHAR(100) DEFAULT '',                       -- 责任人（运维/业务接口人）
  department VARCHAR(100) DEFAULT '',
  addr VARCHAR(128) DEFAULT '',                        -- 部署地址/网段（仅登记，不用于探测纳管）
  remark TEXT DEFAULT '',
  status VARCHAR(20) NOT NULL DEFAULT 'active',        -- active在用/retired下线（下线即证据链终点）
  creator VARCHAR(100) DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dengbao_asset_status ON dengbao_asset(status);

-- R-C5：整改跟踪（测评问题清单 → 整改任务 → 复测留痕）
CREATE TABLE IF NOT EXISTS dengbao_finding (
  id BIGSERIAL PRIMARY KEY,
  item VARCHAR(200) NOT NULL,                          -- 对应控制点/测评项（如「安全审计-8.1.4.1」）
  description TEXT DEFAULT '',                          -- 问题描述
  level VARCHAR(10) NOT NULL DEFAULT 'medium',         -- high/medium/low（测评常见分级）
  status VARCHAR(20) NOT NULL DEFAULT 'open',          -- open待整改/fixing整改中/verified复测通过/closed已关闭
  owner VARCHAR(100) DEFAULT '',                       -- 整改责任人
  due_date TIMESTAMP,                                   -- 整改期限
  asset_id BIGINT DEFAULT 0,                           -- 关联 dengbao_asset.id（0=未关联）
  verifier VARCHAR(100) DEFAULT '',                    -- 复测人（≠整改人时留痕更硬）
  verify_note TEXT DEFAULT '',                         -- 复测结论
  verified_at TIMESTAMP,                               -- 复测时间
  creator VARCHAR(100) DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dengbao_finding_status ON dengbao_finding(status);

-- R-C5：整改/复测动作时间线（谁在何时把状态改成什么，全部留痕）
CREATE TABLE IF NOT EXISTS dengbao_finding_log (
  id BIGSERIAL PRIMARY KEY,
  finding_id BIGINT NOT NULL,
  action VARCHAR(40) NOT NULL,             -- create/update/verify/reopen/close
  actor VARCHAR(100) DEFAULT '',
  note TEXT DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dengbao_finding_log ON dengbao_finding_log(finding_id, id);

-- R-C8：备份记录证据（DB 备份脚本输出登记；有校验和就记，没有不强求）
CREATE TABLE IF NOT EXISTS dengbao_backup_record (
  id BIGSERIAL PRIMARY KEY,
  source VARCHAR(64) NOT NULL DEFAULT 'database',   -- database/app/file
  tool VARCHAR(64) DEFAULT '',                      -- pg_dump/cron/自研脚本
  file_name VARCHAR(500) DEFAULT '',
  size_bytes BIGINT DEFAULT 0,
  checksum VARCHAR(128) DEFAULT '',                 -- sha256
  started_at TIMESTAMP,
  finished_at TIMESTAMP,
  status VARCHAR(20) NOT NULL DEFAULT 'success',    -- success/failed/partial
  note TEXT DEFAULT '',
  creator VARCHAR(100) DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dengbao_backup_status ON dengbao_backup_record(status);
