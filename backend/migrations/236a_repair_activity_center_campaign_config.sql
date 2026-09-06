-- Repair legacy activity-center campaign tables before 237 references config_json.
-- Fresh databases do not have act_campaigns yet; 237 creates the complete table.
DO $$
BEGIN
    IF to_regclass('public.act_campaigns') IS NOT NULL THEN
        ALTER TABLE public.act_campaigns
            ADD COLUMN IF NOT EXISTS config_json TEXT NOT NULL DEFAULT '{}';
    END IF;
END $$;
