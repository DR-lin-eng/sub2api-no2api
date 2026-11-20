-- Invalidate the previous clone format: it included the source administrators.
-- Other transfer profiles and completed, independently provisioned databases are retained.
ALTER TABLE site_migration_jobs ADD COLUMN IF NOT EXISTS ordinary_users_only boolean NOT NULL DEFAULT false;
DELETE FROM site_migration_jobs WHERE profile = 'clone' AND NOT ordinary_users_only;
ALTER TABLE site_migration_jobs DROP CONSTRAINT IF EXISTS site_migration_jobs_clone_policy;
ALTER TABLE site_migration_jobs ADD CONSTRAINT site_migration_jobs_clone_policy
    CHECK (profile <> 'clone' OR ordinary_users_only);
UPDATE site_migration_panel_jobs
SET phase='failed', error_code='checkpoint', attempt=NULL, lease_until=NULL
WHERE profile='clone' AND phase <> 'completed';

ALTER TABLE site_migration_jobs DROP CONSTRAINT IF EXISTS site_migration_jobs_profile_check;
ALTER TABLE site_migration_jobs ADD CONSTRAINT site_migration_jobs_profile_check
    CHECK (profile IN ('all','login','login-quota','login-quota-keys','clone'));
ALTER TABLE site_migration_panel_jobs DROP CONSTRAINT IF EXISTS site_migration_panel_jobs_profile_check;
ALTER TABLE site_migration_panel_jobs ADD CONSTRAINT site_migration_panel_jobs_profile_check
    CHECK (profile IN ('all','login','login-quota','login-quota-keys','clone'));
