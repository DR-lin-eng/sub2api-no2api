-- Keep historical activity types readable without rewriting campaigns or rewards.
-- The application still validates newly created campaigns against its current types.
-- Also align databases that already applied the original 237 successfully with
-- the checksum-pinned compatibility execution of 237 used for legacy upgrades.
ALTER TABLE act_campaigns DROP CONSTRAINT IF EXISTS chk_act_campaigns_type;
ALTER TABLE act_campaigns ADD CONSTRAINT chk_act_campaigns_type
    CHECK (type IN ('lottery', 'inflate', 'redeem', 'custom', 'checkin', 'external_link', 'announcement'));

ALTER TABLE act_participation_records DROP CONSTRAINT IF EXISTS chk_act_participation_records_campaign_type;
ALTER TABLE act_participation_records ADD CONSTRAINT chk_act_participation_records_campaign_type
    CHECK (campaign_type IN ('lottery', 'inflate', 'redeem', 'custom', 'checkin', 'external_link', 'announcement'));
