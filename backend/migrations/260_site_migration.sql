-- Private, resumable transfer state. Neither table belongs to a site export.
CREATE TABLE IF NOT EXISTS site_migration_jobs (
    id uuid PRIMARY KEY,
    owner text NOT NULL,
    mode text NOT NULL CHECK (mode IN ('export','import')),
    profile text NOT NULL CHECK (profile IN ('all','login','login-quota','clone')),
    local_public bytea NOT NULL,
    wrapped_private bytea NOT NULL,
    peer_public bytea,
    blocks integer NOT NULL DEFAULT 0,
    sealed boolean NOT NULL DEFAULT false,
    applied boolean NOT NULL DEFAULT false,
    inserted bigint NOT NULL DEFAULT 0,
    errors integer NOT NULL DEFAULT 0,
    expires_at timestamptz NOT NULL DEFAULT now() + interval '24 hours'
);
CREATE TABLE IF NOT EXISTS site_migration_blocks (
    job uuid NOT NULL REFERENCES site_migration_jobs(id) ON DELETE CASCADE,
    kind smallint NOT NULL CHECK (kind IN (0,1)),
    n integer NOT NULL CHECK (n >= 0),
    data bytea NOT NULL,
    PRIMARY KEY (job, kind, n)
);
CREATE TABLE IF NOT EXISTS site_migration_identity (
    singleton boolean PRIMARY KEY CHECK (singleton),
    site uuid NOT NULL DEFAULT gen_random_uuid()
);
INSERT INTO site_migration_identity(singleton) VALUES(true) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS site_migration_origins (
    site uuid NOT NULL,
    relation text NOT NULL,
    source_key bytea NOT NULL,
    PRIMARY KEY(site,relation,source_key)
);
-- A receiver-generated marker binds each independently provisioned database
-- to its original request, including recovery after CREATE DATABASE.
CREATE TABLE IF NOT EXISTS site_migration_clones (
    job uuid PRIMARY KEY,
    marker uuid NOT NULL DEFAULT gen_random_uuid(),
    created_at timestamptz NOT NULL DEFAULT now()
);
