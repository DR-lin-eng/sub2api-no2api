-- Keep the candidate version separate from the version enforced by readiness.
-- Existing installations were implicitly locked to desired_version; preserve
-- that behavior until the first new rollout explicitly unlocks the candidate.
ALTER TABLE cluster_release_state
    ADD COLUMN IF NOT EXISTS locked_version VARCHAR(64) NOT NULL DEFAULT '';

UPDATE cluster_release_state
SET locked_version = desired_version,
    updated_at = NOW()
WHERE singleton = TRUE
  AND locked_version = ''
  AND desired_version <> '';
