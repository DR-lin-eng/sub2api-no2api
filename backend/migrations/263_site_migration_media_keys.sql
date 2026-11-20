-- Only formerly active managed Media Studio keys can be claimed by the
-- authenticated owner when they select an eligible group on the new site.
CREATE TABLE IF NOT EXISTS site_migration_studio_rebinds (
    api_key_id bigint PRIMARY KEY REFERENCES api_keys(id) ON DELETE CASCADE
);
