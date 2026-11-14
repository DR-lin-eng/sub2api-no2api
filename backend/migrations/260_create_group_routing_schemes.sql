-- Group routing schemes: admin-defined, platform-scoped scheduling templates.
-- A key opts into one scheme with api_keys.routing_scheme_id; the scheme items
-- never replace the legacy ordered api_keys.group_bindings path.

CREATE TABLE IF NOT EXISTS group_routing_schemes (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    platform    VARCHAR(50)  NOT NULL DEFAULT 'anthropic',
    status      VARCHAR(20)  NOT NULL DEFAULT 'active',
    sort_order  INT          NOT NULL DEFAULT 0,
    sort_mode   VARCHAR(20)  NOT NULL DEFAULT 'score',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_group_routing_schemes_platform_status
    ON group_routing_schemes (platform, status);
CREATE INDEX IF NOT EXISTS idx_group_routing_schemes_deleted_at
    ON group_routing_schemes (deleted_at);

CREATE TABLE IF NOT EXISTS group_routing_scheme_items (
    id         BIGSERIAL PRIMARY KEY,
    scheme_id  BIGINT      NOT NULL REFERENCES group_routing_schemes(id) ON DELETE CASCADE,
    group_id   BIGINT      NOT NULL,
    weight     INT         NOT NULL DEFAULT 1,
    position   INT         NOT NULL DEFAULT 0,
    rules      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_group_routing_scheme_items_scheme_group
    ON group_routing_scheme_items (scheme_id, group_id);
CREATE INDEX IF NOT EXISTS idx_group_routing_scheme_items_scheme_position
    ON group_routing_scheme_items (scheme_id, position);
CREATE INDEX IF NOT EXISTS idx_group_routing_scheme_items_group
    ON group_routing_scheme_items (group_id);

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS routing_scheme_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_api_keys_routing_scheme_id
    ON api_keys (routing_scheme_id);
