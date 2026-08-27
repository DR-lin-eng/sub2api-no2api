CREATE TABLE IF NOT EXISTS act_campaigns (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(200) NOT NULL,
    subtitle VARCHAR(500) NOT NULL DEFAULT '',
    banner_url VARCHAR(500) NOT NULL DEFAULT '',
    type VARCHAR(32) NOT NULL DEFAULT 'custom',
    ref_id VARCHAR(200) NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    starts_at TIMESTAMPTZ DEFAULT NULL,
    ends_at TIMESTAMPTZ DEFAULT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    content TEXT NOT NULL DEFAULT '',
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL,
    CONSTRAINT chk_act_campaigns_type CHECK (type IN ('announcement', 'redeem', 'external_link', 'custom')),
    CONSTRAINT chk_act_campaigns_status CHECK (status IN ('draft', 'active', 'archived')),
    CONSTRAINT chk_act_campaigns_sort_order CHECK (sort_order >= 0 AND sort_order <= 10000),
    CONSTRAINT chk_act_campaigns_time_range CHECK (starts_at IS NULL OR ends_at IS NULL OR starts_at < ends_at)
);

CREATE INDEX IF NOT EXISTS idx_act_campaigns_visible
    ON act_campaigns (status, sort_order, starts_at, ends_at, id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_act_campaigns_status
    ON act_campaigns (status)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_act_campaigns_type
    ON act_campaigns (type)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_act_campaigns_created_at
    ON act_campaigns (created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

COMMENT ON TABLE act_campaigns IS '活动中心活动配置';
COMMENT ON COLUMN act_campaigns.type IS '活动类型: announcement, redeem, external_link, custom';
COMMENT ON COLUMN act_campaigns.ref_id IS '关联资源标识，例如外链 URL、公告 ID 或后续兑换/抽奖配置 ID';
COMMENT ON COLUMN act_campaigns.status IS '状态: draft, active, archived';
