package repository

import (
	"context"
	"errors"
	"time"
)

// Change only last_used_at on the current JSON record. Security fields remain
// those committed by the latest administrative mutation, even across nodes.
func (r *settingRepository) RecordAdminAPIKeyUsage(ctx context.Context, id, digest string, usedAt time.Time) error {
	_, err := r.client.ExecContext(ctx, `
UPDATE settings SET value = jsonb_set(value::jsonb, '{keys}', (
 SELECT jsonb_agg(CASE
  WHEN record->>'id'=$2 AND record->>'key_hash'=$3 AND record->>'status'='active'
       AND COALESCE(record->>'revoked_at','')=''
  THEN jsonb_set(record, '{last_used_at}', to_jsonb($4::text), true)
  ELSE record END ORDER BY ordinal)
 FROM jsonb_array_elements(value::jsonb->'keys') WITH ORDINALITY AS entries(record,ordinal)
), true)::text, updated_at=NOW()
WHERE key=$1 AND EXISTS (
 SELECT 1 FROM jsonb_array_elements(value::jsonb->'keys') AS current
 WHERE current->>'id'=$2 AND current->>'key_hash'=$3 AND current->>'status'='active'
)`, "admin_api_keys", id, digest, usedAt.UTC().Format(time.RFC3339Nano))
	return err
}

// Claim each authenticated encapsulation once, across replicas. The envelope
// expires before its replay record can be collected.
func (r *settingRepository) ConsumeMigrationHTTP(ctx context.Context, nonce []byte, expires time.Time) error {
	res, err := r.client.ExecContext(ctx, `WITH expired AS (DELETE FROM site_migration_http_nonces WHERE expires_at<now())
 INSERT INTO site_migration_http_nonces(nonce,expires_at) SELECT $1,$2::timestamptz WHERE $2::timestamptz > now() ON CONFLICT DO NOTHING`, nonce, expires)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("migration request already consumed")
	}
	return nil
}
