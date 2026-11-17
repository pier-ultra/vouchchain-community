-- v14 AK4.2 审计五要素补齐：来源 IP。前后值由既有 detail JSON 承载（部分动作已带 before/after）。
-- 幂等；单独文件，保证已执行过 20261115_p2p_bridge.sql 的实例也会补上这一列。
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS ip VARCHAR(64) DEFAULT '';
