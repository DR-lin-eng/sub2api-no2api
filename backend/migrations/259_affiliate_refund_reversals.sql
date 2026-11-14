-- Keep the original award as the immutable amount used by rebate caps/history.
-- The remaining portion is the only amount eligible for future thaw.
ALTER TABLE user_affiliate_ledger
    ADD COLUMN IF NOT EXISTS reversed_amount DECIMAL(20,8) NOT NULL DEFAULT 0;

COMMENT ON COLUMN user_affiliate_ledger.reversed_amount IS 'Cumulative award reversed after confirmed source-order refunds';
COMMENT ON COLUMN user_affiliates.aff_quota IS 'Net affiliate quota; negative values are affiliate debt repaid by future awards';
