-- Benchmark schema for CockroachDB Developer Benchmarks
-- Used by all workloads across all agents

CREATE DATABASE IF NOT EXISTS benchmark;
USE benchmark;

-- CRUD + Retry Storm + Mixed R/W + Follower Reads workloads
CREATE TABLE IF NOT EXISTS bench_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    balance DECIMAL(15, 2) NOT NULL DEFAULT 1000.00,
    name STRING NOT NULL,
    region STRING NOT NULL DEFAULT 'us-east',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Mixed R/W workload
CREATE TABLE IF NOT EXISTS bench_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES bench_accounts(id) ON DELETE CASCADE,
    amount DECIMAL(15, 2) NOT NULL,
    status STRING NOT NULL DEFAULT 'pending',
    ts TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orders_account ON bench_orders(account_id);
CREATE INDEX IF NOT EXISTS idx_orders_ts ON bench_orders(ts DESC);

-- Vector Search workload
CREATE TABLE IF NOT EXISTS bench_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    content STRING NOT NULL,
    category STRING NOT NULL DEFAULT 'general',
    embedding VECTOR(1536),
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE VECTOR INDEX IF NOT EXISTS idx_bench_docs_embedding ON bench_documents (embedding vector_cosine_ops);

-- CDC workload tracking
CREATE TABLE IF NOT EXISTS bench_cdc_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_table STRING NOT NULL,
    operation STRING NOT NULL,
    payload JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lag_ms INT NOT NULL DEFAULT 0
);

-- Seed data for retry storm (hot rows)
INSERT INTO bench_accounts (id, balance, name, region)
SELECT
    gen_random_uuid(),
    1000.00 + (random() * 9000)::DECIMAL(15,2),
    'account-' || generate_series::STRING,
    CASE (generate_series % 3)
        WHEN 0 THEN 'us-east'
        WHEN 1 THEN 'us-west'
        ELSE 'eu-west'
    END
FROM generate_series(1, 100);
