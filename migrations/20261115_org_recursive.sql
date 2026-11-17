-- 组织树 / 法人主体父子递归：主体上下文增加「含下级主体」开关。
-- 默认 FALSE，保持原口径（切到某主体 = 只看该主体），开启后才按父子树展开可见范围。
-- 幂等可重复执行，纯开源社区版能力。

ALTER TABLE user_context ADD COLUMN IF NOT EXISTS include_subtree BOOLEAN NOT NULL DEFAULT FALSE;

-- 递归展开下级主体时按 parent_id 取子树，补索引
CREATE INDEX IF NOT EXISTS idx_le_parent ON legal_entity(parent_id);
