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
