-- 合同台账 contract（产品① 合同线核心实体）
CREATE TABLE IF NOT EXISTS contract (
    id            BIGSERIAL PRIMARY KEY,
    title         VARCHAR(200) NOT NULL,
    counter_party VARCHAR(200),
    amount        NUMERIC(18,2),
    currency      VARCHAR(10)  DEFAULT 'CNY',
    sign_date     VARCHAR(20),
    status        VARCHAR(30)  DEFAULT 'draft',
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP NOT NULL DEFAULT NOW()
);
COMMENT ON TABLE contract IS '合同台账（产品① 合同线核心实体）';
CREATE INDEX IF NOT EXISTS idx_contract_status ON contract (status);
