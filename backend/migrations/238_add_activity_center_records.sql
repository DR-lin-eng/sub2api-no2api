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
