package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/domain"
)

// ClaimMigratedMediaStudioKey is called only after the studio validates the
// operator's own eligible group. An unbound or inactive key alone cannot be
// claimed: its import must have recorded that it was active before migration.
func (r *apiKeyRepository) ClaimMigratedMediaStudioKey(ctx context.Context, userID, groupID int64) (*service.APIKey, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('sub2api:studio-rebind:'||$1::text,0))", userID); err != nil {
		return nil, err
	}
	var id int64
	// Concurrent sessions for an already claimed group must get its original
	// key even if a second unbound managed key is available.
	err = scanSingleRow(ctx, tx, "SELECT id FROM api_keys WHERE user_id=$1 AND group_id=$2 AND lower(btrim(name))=lower($3) AND deleted_at IS NULL ORDER BY id LIMIT 1", []any{userID, groupID, domain.MediaStudioAPIKeyName}, &id)
	if errors.Is(err, sql.ErrNoRows) {
		err = scanSingleRow(ctx, tx, `UPDATE api_keys SET group_id=$2,group_bindings=jsonb_build_array(jsonb_build_object('group_id',$2::bigint)),status='active',updated_at=now()
         WHERE id=(SELECT k.id FROM site_migration_studio_rebinds m JOIN api_keys k ON k.id=m.api_key_id
          WHERE k.user_id=$1 AND lower(btrim(k.name))=lower($3) AND k.status='inactive'
          AND k.deleted_at IS NULL AND k.group_id IS NULL AND k.group_bindings='[]'::jsonb
          AND (k.expires_at IS NULL OR k.expires_at>now()) AND (k.quota<=0 OR k.quota_used<k.quota)
          ORDER BY k.id LIMIT 1 FOR UPDATE OF k) RETURNING id`, []any{userID, groupID, domain.MediaStudioAPIKeyName}, &id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, "DELETE FROM site_migration_studio_rebinds WHERE api_key_id=$1", id)
		}
	}
	if err != nil {
		return nil, err
	}
	key, err := tx.Client().APIKey.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return apiKeyEntityToService(key), nil
}
