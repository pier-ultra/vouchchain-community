-- 法律条款知识库（RAG 最小闭环）：向量列 + 种子条款（本文件幂等，可重复执行）
-- pgvector：instabox PG 已启用扩展；bge-m3 输出 1024 维
CREATE EXTENSION IF NOT EXISTS vector;
CREATE TABLE IF NOT EXISTS kb_clause (
  id BIGSERIAL PRIMARY KEY,
  title VARCHAR(200) NOT NULL,
  content TEXT NOT NULL,
  source VARCHAR(200) NOT NULL DEFAULT '',
  category VARCHAR(50) NOT NULL DEFAULT '',
  embedding vector(1024),
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_kb_clause_embedding ON kb_clause USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
