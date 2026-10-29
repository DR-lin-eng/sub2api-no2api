package repository

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

const authKeyBloomLockID int64 = 73622804170501

func (r *apiKeyRepository) SetAuthKeyBloomCache(cache service.APIKeyAuthBloomCache) {
	r.authBloom = cache // Set once during dependency construction, before serving.
}

func (r *apiKeyRepository) createWithAuthBloom(ctx context.Context, key *service.APIKey) error {
	client := r.client
	var tx *dbent.Tx
	if existing := dbent.TxFromContext(ctx); existing != nil {
		client = existing.Client()
	} else {
		var err error
		tx, err = r.client.Tx(ctx)
		if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
			return err
		}
		if tx != nil {
			defer func() { _ = tx.Rollback() }()
			client = tx.Client()
		}
	}
	// Rebuild takes the exclusive form of this transaction lock. This closes
	// the pre-register/commit gap even if Redis evicts or loses the bitmap.
	if _, err := client.ExecContext(ctx, "SELECT pg_advisory_xact_lock_shared($1)", authKeyBloomLockID); err != nil {
		return err
	}
	if err := r.authBloom.RegisterAuthKey(ctx, key.Key); err != nil {
		return fmt.Errorf("register API key membership before create: %w", err)
	}
	if err := r.createAPIKey(ctx, client, key); err != nil {
		return err
	}
	if tx != nil {
		return tx.Commit()
	}
	return nil
}

func (r *apiKeyRepository) WalkAuthKeyDigests(ctx context.Context, visit func([][sha256.Size]byte) error) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", authKeyBloomLockID); err != nil {
		return err
	}
	var after int64
	for {
		rows, err := tx.Client().APIKey.Query().Where(apikey.DeletedAtIsNil(), apikey.IDGT(after)).
			Order(dbent.Asc(apikey.FieldID)).Select(apikey.FieldID, apikey.FieldKey).Limit(1000).All(ctx)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return tx.Commit()
		}
		digests := make([][sha256.Size]byte, len(rows))
		for i, row := range rows {
			digests[i] = sha256.Sum256([]byte(row.Key))
			after = row.ID
		}
		if err := visit(digests); err != nil {
			return err
		}
	}
}
