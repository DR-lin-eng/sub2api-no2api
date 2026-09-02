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
