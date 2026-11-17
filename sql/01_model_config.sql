-- ============================================================================
-- 模型配置表 model_config  —— Core 共用内核层
-- 统一托管各类 AI / ML 服务的接入配置，避免把密钥与端点散落到业务代码。
-- 类别(category) 取值示例：
--   llm      大模型对话/补全            provider: agnes / openai / qwen ...
--   ocr      在线 OCR 服务              provider: baidu-paddle / mineroo ...
--   rerank   重排序（检索召回后精排）    provider: cohere-rerank / aliyun-rerank ...
--   embedding 向量化模型               provider: openai-embed / bge ...
--   vector   向量库接入（pgvector）     provider: pgvector
-- ============================================================================
CREATE TABLE IF NOT EXISTS model_config (
    id            BIGSERIAL PRIMARY KEY,
    name          VARCHAR(100)  NOT NULL,                 -- 配置名，如 'agnes-llm' / 'baidu-ocr' / 'cohere-rerank'
    category      VARCHAR(50)   NOT NULL,                 -- llm / ocr / rerank / embedding / vector
    provider      VARCHAR(100)  NOT NULL,                 -- 服务商标识
    base_url      VARCHAR(512),                            -- 端点（OpenAI 兼容 / OCR 服务 / 重排服务）
    model_name    VARCHAR(200),                            -- 模型名，如 agnes-3.0-flash / pp-ocrv4
    api_key       VARCHAR(512),                            -- 密钥【★敏感：生产须加密存储或走 KMS/环境变量，勿明文落库与备份】
    extra_config  JSONB,                                   -- 服务商特有参数：temperature / top_p / timeout / dims / top_k ...
    enabled       BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMP     NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP     NOT NULL DEFAULT NOW(),
    CONSTRAINT uk_model_config_name UNIQUE (name)
);

CREATE INDEX IF NOT EXISTS idx_model_config_category ON model_config (category);
CREATE INDEX IF NOT EXISTS idx_model_config_enabled  ON model_config (enabled);

COMMENT ON TABLE  model_config IS 'AI/ML 模型接入配置表（Core 共用内核），统一托管 LLM/OCR/Rerank/Embedding/Vector 等接入参数';

-- ----------------------------------------------------------------------------
-- 向量库（pgvector）建表约定（★ 暂不建，待真正需要向量检索时再启用）
-- 启用前确保已 CREATE EXTENSION vector; 本部署脚本已执行。
-- 示例（后续启用）：
--   CREATE TABLE IF NOT EXISTS knowledge_embedding (
--       id         BIGSERIAL PRIMARY KEY,
--       doc_id     VARCHAR(64)  NOT NULL,
--       chunk_no   INT          NOT NULL,
--       content    TEXT,
--       embedding  vector(1536),                -- 维度随 embedding 模型而定
--       created_at TIMESTAMP NOT NULL DEFAULT NOW()
--   );
--   CREATE INDEX IF NOT EXISTS idx_knowledge_embedding_vec
--       ON knowledge_embedding USING hnsw (embedding vector_cosine_ops);
-- ----------------------------------------------------------------------------
