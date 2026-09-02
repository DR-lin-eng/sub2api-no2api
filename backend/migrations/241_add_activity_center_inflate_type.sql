-- Allow the activity center to distinguish code inflation from legacy redeem campaigns.
ALTER TABLE act_campaigns DROP CONSTRAINT IF EXISTS chk_act_campaigns_type;
ALTER TABLE act_campaigns ADD CONSTRAINT chk_act_campaigns_type
    CHECK (type IN ('lottery', 'inflate', 'redeem', 'custom'));
