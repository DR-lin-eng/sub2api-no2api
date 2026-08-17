ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS egress_mode VARCHAR(20) NOT NULL DEFAULT 'inherit';

CREATE TABLE IF NOT EXISTS ipv6_egress_pools (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    cidr VARCHAR(64) NOT NULL UNIQUE,
    node_id VARCHAR(128),
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    allocation_version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ipv6_egress_pools_status_check CHECK (status IN ('active', 'disabled')),
    CONSTRAINT ipv6_egress_pools_allocation_version_check CHECK (allocation_version > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS ipv6_egress_pools_one_default_idx
    ON ipv6_egress_pools (is_default)
    WHERE is_default = TRUE;

CREATE TABLE IF NOT EXISTS account_egress_bindings (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
    pool_id BIGINT NOT NULL REFERENCES ipv6_egress_pools(id) ON DELETE RESTRICT,
    source_ipv6 VARCHAR(45) NOT NULL UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    version BIGINT NOT NULL DEFAULT 1,
    rotated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_egress_bindings_status_check CHECK (status IN ('active', 'disabled')),
    CONSTRAINT account_egress_bindings_version_check CHECK (version > 0)
);

CREATE INDEX IF NOT EXISTS account_egress_bindings_pool_id_idx
    ON account_egress_bindings (pool_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'accounts_egress_mode_check'
    ) THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_egress_mode_check
            CHECK (egress_mode IN ('inherit', 'direct', 'external_proxy', 'ipv6_pool'));
    END IF;
END
$$;
