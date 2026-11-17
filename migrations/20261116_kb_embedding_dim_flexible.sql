-- 向量列维度可配置化：从固定 vector(1024) 放宽为无维度约束 vector，
-- 使不同维度模型（硅基流动 bge-m3=1024 / 讯飞 xop3qwen8bembedding=768 / 本地模型）都能入库。
-- 幂等：可重复执行。
--
-- 背景：接入讯飞 MaaS 后 embedding 维度由模型决定，硬编码 1024 会让 768 维向量写入失败。
-- 代码侧 pkg/kb/kb.go 已改为动态维度；本迁移只负责放宽数据库列约束。
-- 注意：放宽列类型不丢已有数据（1024 维向量保持不变）。
--
-- 关键修正（2026-11-16 端到端实测）：pgvector 的 ivfflat 索引会记住建索引时的维度，
-- 即使列已放宽为无约束 vector，只要基于旧维度建的 ivfflat 索引还在，
-- 写入新维度向量仍会报 "expected 1024 dimensions, not 768"。
-- 且 ivfflat 官方要求 >=1000 行才有意义，本项目法律条款种子仅十几条，本就不该建 ivfflat。
-- 因此：放宽列后无条件 DROP 该索引，且不再重建（少量数据全表扫描即可）。

DO $$
BEGIN
    -- 仅当列当前是 vector(1024) 这类带维度约束的类型时才放宽；已是 vector 则跳过
    IF EXISTS (
        SELECT 1 FROM pg_attribute a
        JOIN pg_class c ON c.oid = a.attrelid
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relname = 'kb_clause'
          AND a.attname = 'embedding'
          AND n.nspname = current_schema()
          AND format_type(a.atttypid, a.atttypmod) LIKE 'vector(%)'
    ) THEN
        ALTER TABLE kb_clause ALTER COLUMN embedding TYPE vector;
    END IF;
END $$;

-- 删除任何基于 embedding 的旧索引：它可能绑定了 1024 维，会拒绝新维度写入。
-- 数据量小无需向量索引；若将来条款上千条，再用新维度重建 ivfflat/hnsw。
DROP INDEX IF EXISTS idx_kb_clause_embedding;
