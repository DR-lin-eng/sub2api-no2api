-- Each scheme can trade TTFT against stability. The composite score becomes a
-- weighted average of the TTFT score and the error score, and those weights are
-- part of the scheme instead of being hardcoded in the scheduler.
-- Defaults keep the previous 0.4 / 0.6 behavior for existing rows.

ALTER TABLE group_routing_schemes
    ADD COLUMN IF NOT EXISTS ttft_weight INT NOT NULL DEFAULT 40;
ALTER TABLE group_routing_schemes
    ADD COLUMN IF NOT EXISTS error_weight INT NOT NULL DEFAULT 60;
