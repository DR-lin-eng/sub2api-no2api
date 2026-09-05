-- PR #31: opt-in activity center. Existing application tables and migration checksums are unchanged.

-- 047_add_activity_center_campaigns.sql

-- 建表（如果不存在）
CREATE TABLE IF NOT EXISTS act_campaigns (
                                             id BIGSERIAL PRIMARY KEY,
                                             title VARCHAR(200) NOT NULL,
                                             subtitle VARCHAR(500) NOT NULL DEFAULT '',
                                             banner_url VARCHAR(500) NOT NULL DEFAULT '',
                                             banner_html TEXT NOT NULL DEFAULT '',
                                             type VARCHAR(32) NOT NULL DEFAULT 'custom',
                                             ref_id VARCHAR(200) NOT NULL DEFAULT '',
                                             config_json TEXT NOT NULL DEFAULT '{}',
                                             status VARCHAR(20) NOT NULL DEFAULT 'draft',
                                             starts_at TIMESTAMPTZ DEFAULT NULL,
                                             ends_at TIMESTAMPTZ DEFAULT NULL,
                                             sort_order INT NOT NULL DEFAULT 0,
                                             content TEXT NOT NULL DEFAULT '',
                                             created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
                                             created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                             updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                             deleted_at TIMESTAMPTZ DEFAULT NULL,
                                             CONSTRAINT chk_act_campaigns_type CHECK (type IN ('lottery', 'redeem', 'custom')),
                                             CONSTRAINT chk_act_campaigns_status CHECK (status IN ('draft', 'active', 'archived')),
                                             CONSTRAINT chk_act_campaigns_sort_order CHECK (sort_order >= 0 AND sort_order <= 10000),
                                             CONSTRAINT chk_act_campaigns_time_range CHECK (starts_at IS NULL OR ends_at IS NULL OR starts_at < ends_at)
);

-- 兼容旧表：确保 banner_html 列存在
ALTER TABLE act_campaigns ADD COLUMN IF NOT EXISTS banner_html TEXT NOT NULL DEFAULT '';

-- 索引
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

-- 注释
COMMENT ON TABLE act_campaigns IS '活动中心活动配置';
COMMENT ON COLUMN act_campaigns.banner_html IS '活动横幅 HTML，存在时优先于 banner_url 展示';
COMMENT ON COLUMN act_campaigns.type IS '活动类型: lottery, redeem, custom';
COMMENT ON COLUMN act_campaigns.ref_id IS '关联项目内配置标识，可选';
COMMENT ON COLUMN act_campaigns.config_json IS '活动类型专属配置 JSON，例如抽奖奖池、奖品与库存码';
COMMENT ON COLUMN act_campaigns.status IS '状态: draft, active, archived';

CREATE TABLE IF NOT EXISTS act_participation_records (
    id BIGSERIAL PRIMARY KEY,
    campaign_id BIGINT NOT NULL REFERENCES act_campaigns(id) ON DELETE CASCADE,
    campaign_title VARCHAR(200) NOT NULL DEFAULT '',
    campaign_type VARCHAR(32) NOT NULL DEFAULT 'custom',
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pool_id VARCHAR(200) NOT NULL DEFAULT '',
    pool_name VARCHAR(200) NOT NULL DEFAULT '',
    prize_id VARCHAR(200) NOT NULL DEFAULT '',
    prize_label VARCHAR(200) NOT NULL DEFAULT '',
    prize_type VARCHAR(32) NOT NULL DEFAULT '',
    prize_color VARCHAR(32) NOT NULL DEFAULT '',
    result_status VARCHAR(32) NOT NULL DEFAULT 'recorded',
    reward_status VARCHAR(32) NOT NULL DEFAULT 'none',
    reward_payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_act_participation_records_campaign_type CHECK (campaign_type IN ('lottery', 'redeem', 'custom')),
    CONSTRAINT chk_act_participation_records_result_status CHECK (result_status IN ('recorded', 'won', 'none')),
    CONSTRAINT chk_act_participation_records_reward_status CHECK (reward_status IN ('none', 'pending', 'granted', 'failed')),
    CONSTRAINT chk_act_participation_records_reward_payload_json CHECK (jsonb_typeof(reward_payload_json) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_act_participation_records_user_created
    ON act_participation_records (user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_act_participation_records_campaign_created
    ON act_participation_records (campaign_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_act_participation_records_prize
    ON act_participation_records (campaign_id, prize_id)
    WHERE prize_id <> '';

CREATE INDEX IF NOT EXISTS idx_act_participation_records_type_created
    ON act_participation_records (campaign_type, created_at DESC, id DESC);

COMMENT ON TABLE act_participation_records IS '活动中心参与与抽奖记录';
COMMENT ON COLUMN act_participation_records.reward_payload_json IS '中奖奖励快照，仅管理端与中奖用户本人可见';

-- Allow the activity center to distinguish code inflation from legacy redeem campaigns.
ALTER TABLE act_campaigns DROP CONSTRAINT IF EXISTS chk_act_campaigns_type;
ALTER TABLE act_campaigns ADD CONSTRAINT chk_act_campaigns_type
    CHECK (type IN ('lottery', 'inflate', 'redeem', 'custom'));

-- Extend activity records for code inflation campaigns without modifying the
-- already-applied activity-center migrations.
ALTER TABLE act_participation_records
    DROP CONSTRAINT IF EXISTS chk_act_participation_records_campaign_type;
ALTER TABLE act_participation_records
    ADD CONSTRAINT chk_act_participation_records_campaign_type
    CHECK (campaign_type IN ('lottery', 'inflate', 'redeem', 'custom'));

ALTER TABLE act_participation_records
    DROP CONSTRAINT IF EXISTS chk_act_participation_records_result_status;
ALTER TABLE act_participation_records
    ADD CONSTRAINT chk_act_participation_records_result_status
    CHECK (result_status IN ('recorded', 'won', 'none', 'inflated'));

CREATE TABLE IF NOT EXISTS act_checkin_records (
    id BIGSERIAL PRIMARY KEY,
    campaign_id BIGINT NOT NULL REFERENCES act_campaigns(id),
    user_id BIGINT NOT NULL REFERENCES users(id),
    checkin_date DATE NOT NULL,
    cycle_no INTEGER NOT NULL DEFAULT 1,
    cycle_day INTEGER NOT NULL,
    streak_days INTEGER NOT NULL,
    reward_type VARCHAR(32) NOT NULL,
    reward_value VARCHAR(200) NOT NULL,
    reward_status VARCHAR(32) NOT NULL,
    reward_payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT act_checkin_records_campaign_user_date_key UNIQUE (campaign_id, user_id, checkin_date)
);

CREATE INDEX IF NOT EXISTS act_checkin_records_campaign_user_date_idx
    ON act_checkin_records (campaign_id, user_id, checkin_date DESC);

ALTER TABLE act_campaigns DROP CONSTRAINT IF EXISTS chk_act_campaigns_type;
ALTER TABLE act_campaigns ADD CONSTRAINT chk_act_campaigns_type
    CHECK (type IN ('lottery', 'inflate', 'redeem', 'custom', 'checkin'));

ALTER TABLE act_participation_records DROP CONSTRAINT IF EXISTS chk_act_participation_records_campaign_type;
ALTER TABLE act_participation_records ADD CONSTRAINT chk_act_participation_records_campaign_type
    CHECK (campaign_type IN ('lottery', 'inflate', 'redeem', 'custom', 'checkin'));


-- Daily-limit checks use the entire predicate rather than scanning a user's history.
CREATE INDEX IF NOT EXISTS idx_act_records_user_campaign_pool_created
 ON act_participation_records (user_id, campaign_id, pool_id, created_at);
CREATE INDEX IF NOT EXISTS idx_act_records_campaign_card_codes
 ON act_participation_records (campaign_id, id) WHERE reward_payload_json ? 'code';

-- A durable per-user summary keeps leaderboard reads independent of daily history size.
-- The daily ledger remains authoritative; both are written atomically.
CREATE TABLE IF NOT EXISTS act_checkin_summaries (
 campaign_id BIGINT NOT NULL REFERENCES act_campaigns(id) ON DELETE CASCADE,
 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 best_streak INTEGER NOT NULL,
 checkin_count BIGINT NOT NULL,
 last_checkin_date DATE NOT NULL,
 PRIMARY KEY (campaign_id, user_id)
);
INSERT INTO act_checkin_summaries (campaign_id, user_id, best_streak, checkin_count, last_checkin_date)
 SELECT campaign_id, user_id, MAX(streak_days), COUNT(*), MAX(checkin_date)
 FROM act_checkin_records GROUP BY campaign_id, user_id
 ON CONFLICT (campaign_id, user_id) DO NOTHING;
CREATE INDEX IF NOT EXISTS idx_act_checkin_leaderboard
 ON act_checkin_summaries (campaign_id, best_streak DESC, checkin_count DESC, last_checkin_date DESC, user_id);
