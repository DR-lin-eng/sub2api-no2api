package repository

import (
	"context"
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
