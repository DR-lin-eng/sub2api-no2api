-- Operator and progress metadata only. Peer keys never go into these tables.
CREATE TABLE IF NOT EXISTS site_migration_panel_jobs (
    id uuid PRIMARY KEY,
    admin_id bigint NOT NULL,
    direction text NOT NULL CHECK(direction IN ('push','pull')),
    peer text NOT NULL,
    profile text NOT NULL CHECK(profile IN ('all','clone','login','login-quota')),
    phase text NOT NULL,
    error_code text NOT NULL DEFAULT '',
    acknowledged integer NOT NULL DEFAULT 0,
    total integer NOT NULL DEFAULT 0,
    inserted bigint NOT NULL DEFAULT 0,
    issues integer NOT NULL DEFAULT 0,
    report_bytes bigint NOT NULL DEFAULT 0,
    attempt uuid,
    lease_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '24 hours'
);
CREATE INDEX IF NOT EXISTS site_migration_panel_admin_time ON site_migration_panel_jobs(admin_id,created_at DESC);
CREATE TABLE IF NOT EXISTS site_migration_panel_reports (
    job uuid NOT NULL REFERENCES site_migration_panel_jobs(id) ON DELETE CASCADE,
    attempt uuid NOT NULL,
    part integer NOT NULL,
    ciphertext bytea NOT NULL,
    PRIMARY KEY(job,attempt,part)
);
