-- Add independently configurable total and per-user limits for redeem codes.
ALTER TABLE redeem_codes
    ADD COLUMN IF NOT EXISTS max_uses INT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS used_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS max_uses_per_user INT NOT NULL DEFAULT 1;

-- Existing one-time codes are represented by the default max_uses=1 and
-- max_uses_per_user=1, so their behavior remains unchanged.
CREATE INDEX IF NOT EXISTS idx_redeem_codes_usage_limit
    ON redeem_codes (max_uses, used_count, max_uses_per_user);

CREATE TABLE IF NOT EXISTS redeem_code_usages (
    id BIGSERIAL PRIMARY KEY,
    redeem_code_id BIGINT NOT NULL REFERENCES redeem_codes(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    value DECIMAL(20,8) NOT NULL,
    used_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_redeem_code_usages_code
    ON redeem_code_usages (redeem_code_id);
CREATE INDEX IF NOT EXISTS idx_redeem_code_usages_user
    ON redeem_code_usages (user_id, used_at DESC);
CREATE INDEX IF NOT EXISTS idx_redeem_code_usages_code_user
    ON redeem_code_usages (redeem_code_id, user_id);
