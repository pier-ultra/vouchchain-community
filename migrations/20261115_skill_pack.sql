-- 切片凭证联动 + Agent Skill 包（用户 11-15）
ALTER TABLE journal_draft ADD COLUMN IF NOT EXISTS split_id BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS skill_pack (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL UNIQUE,          -- 目录名（zip 内 SKILL.md frontmatter 的 name）
  display_name VARCHAR(200) NOT NULL DEFAULT '',
  description VARCHAR(500) NOT NULL DEFAULT '',
  triggers TEXT NOT NULL DEFAULT '',          -- 逗号分隔触发词
  instructions TEXT NOT NULL DEFAULT '',      -- 注入对话的技能指令（SKILL.md 正文）
  version VARCHAR(20) NOT NULL DEFAULT '1.0',
  path VARCHAR(300) NOT NULL DEFAULT '',      -- 磁盘权威源：skills/{name}/
  file_id BIGINT NOT NULL DEFAULT 0,          -- 文件中心备份登记（file_entry.id）
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  creator VARCHAR(100) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
