-- v23 线D 政务公文/督办小件（2026-11-16）
-- 范围收窄：O2OA 已覆盖通用公文（GB/T 9704 收发文/套红/归档），我们只做与合同/义务咬合的「督办」小件。
-- R-D1 督办任务类型：会议决议/领导批示/其他 → 任务 → 期限 → 催办；R-D2 督办看板过滤。
-- 幂等，可重复执行。

CREATE TABLE IF NOT EXISTS duban_task (
  id BIGSERIAL PRIMARY KEY,
  source_type VARCHAR(20) NOT NULL DEFAULT 'meeting',  -- meeting 会议决议 / directive 领导批示 / other 其他
  title VARCHAR(200) NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  owner VARCHAR(100) NOT NULL DEFAULT '',               -- 责任人
  creator VARCHAR(100) NOT NULL DEFAULT '',
  deadline DATE,                                        -- 督办期限
  status VARCHAR(12) NOT NULL DEFAULT 'todo',           -- todo 进行中 / done 已完成（逾期为派生态，不落库）
  priority VARCHAR(8) NOT NULL DEFAULT 'mid',           -- high / mid / low
  remind_cnt INT NOT NULL DEFAULT 0,                    -- 催办次数
  last_remind_at TIMESTAMP,
  contract_file_id BIGINT NOT NULL DEFAULT 0,           -- 可选：关联合同文件（与合同线咬合）
  remark VARCHAR(300) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_duban_status_deadline ON duban_task (status, deadline);
CREATE INDEX IF NOT EXISTS idx_duban_source ON duban_task (source_type);

CREATE TABLE IF NOT EXISTS duban_log (
  id BIGSERIAL PRIMARY KEY,
  task_id BIGINT NOT NULL,
  action VARCHAR(20) NOT NULL DEFAULT '',              -- create / remind / done / update
  actor VARCHAR(100) NOT NULL DEFAULT '',
  note VARCHAR(300) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_duban_log_task ON duban_log (task_id, id);
